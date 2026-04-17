package api

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jakevis/rinexprep/internal/gnss"
	"github.com/jakevis/rinexprep/internal/pipeline"
	"github.com/jakevis/rinexprep/internal/rinex"
	"github.com/jakevis/rinexprep/internal/ubx"
)

const maxUploadSize = 600 << 20 // 600 MB to accommodate multipart overhead on large files

// handleUpload accepts a multipart file upload and creates a new job.
// POST /api/v1/upload
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Enforce max upload size.
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		if err.Error() == "http: request body too large" {
			jsonError(w, "file exceeds 600 MB limit", http.StatusRequestEntityTooLarge)
			return
		}
		jsonError(w, "invalid multipart form: "+err.Error(), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		jsonError(w, "missing 'file' field", http.StatusBadRequest)
		return
	}
	defer file.Close()

	job := s.jobStore.Create(header.Filename, header.Size)

	jobDir := filepath.Join(s.jobStore.dir, job.ID)
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		jsonError(w, "failed to create job directory", http.StatusInternalServerError)
		return
	}

	dst, err := os.Create(filepath.Join(jobDir, "input.ubx"))
	if err != nil {
		jsonError(w, "failed to save file", http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	written, err := io.Copy(dst, file)
	if err != nil {
		jsonError(w, "failed to write file", http.StatusInternalServerError)
		return
	}

	// Update actual size from bytes written.
	job.mu.Lock()
	job.InputSize = written
	job.InputFile = filepath.Join(jobDir, "input.ubx")
	job.mu.Unlock()

	slog.Info("upload", "job_id", job.ID, "filename", header.Filename, "size_bytes", written)

	// Kick off background parsing to generate preview data.
	go s.parseAndPreview(job)

	jsonResponse(w, http.StatusOK, map[string]string{"jobId": job.ID})
}

// parseAndPreview parses the UBX file and populates the job's preview data.
func (s *Server) parseAndPreview(job *Job) {
	job.mu.Lock()
	inputFile := job.InputFile
	job.Status = StatusParsing
	job.Progress = "Parsing UBX binary data..."
	job.mu.Unlock()

	f, err := os.Open(inputFile)
	if err != nil {
		job.mu.Lock()
		job.Status = StatusFailed
		job.Error = "failed to open input: " + err.Error()
		job.Progress = ""
		job.mu.Unlock()
		slog.Error("parse_failed", "job_id", job.ID, "error", err)
		return
	}
	defer f.Close()

	ptrs, stats, err := ubx.Parse(f)
	if err != nil {
		job.mu.Lock()
		job.Status = StatusFailed
		job.Error = "parse error: " + err.Error()
		job.Progress = ""
		job.mu.Unlock()
		slog.Error("parse_failed", "job_id", job.ID, "error", err)
		return
	}

	job.mu.Lock()
	job.Progress = "Analyzing satellite visibility..."
	job.mu.Unlock()

	epochs := derefEpochs(ptrs)

	// Apply receiver clock correction before any processing.
	epochs = pipeline.CorrectClockBias(epochs, pipeline.ClockCorrConfig{TADJ: 0.1})

	job.mu.Lock()
	job.Progress = "Computing quality metrics..."
	job.mu.Unlock()

	preview := generatePreview(epochs, stats.NavSatData)

	job.mu.Lock()
	job.epochs = epochs
	job.Preview = preview
	job.Status = StatusPreview
	job.Progress = "Preview ready"
	if stats.BestPosition != nil {
		job.ApproxX, job.ApproxY, job.ApproxZ = stats.BestPosition.ECEF()
	}
	job.mu.Unlock()

	slog.Info("parse_complete", "job_id", job.ID, "epochs", len(epochs))
}

// handleJobStatus returns the current status of a job.
// GET /api/v1/jobs/{id}/status
func (s *Server) handleJobStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	id := extractJobID(r.URL.Path, "/api/v1/jobs/", "/status")
	job, ok := s.jobStore.Get(id)
	if !ok {
		jsonError(w, "job not found", http.StatusNotFound)
		return
	}
	job.mu.Lock()
	defer job.mu.Unlock()
	jsonResponse(w, http.StatusOK, job)
}

// handlePreview returns satellite visibility and skyview data.
// GET /api/v1/jobs/{id}/preview
func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	id := extractJobID(r.URL.Path, "/api/v1/jobs/", "/preview")
	job, ok := s.jobStore.Get(id)
	if !ok {
		jsonError(w, "job not found", http.StatusNotFound)
		return
	}

	job.mu.Lock()
	preview := job.Preview
	job.mu.Unlock()

	if preview != nil {
		jsonResponse(w, http.StatusOK, preview)
		return
	}

	// Not yet parsed by background goroutine; parse synchronously.
	s.parseAndPreview(job)

	job.mu.Lock()
	preview = job.Preview
	job.mu.Unlock()

	if preview != nil {
		jsonResponse(w, http.StatusOK, preview)
		return
	}

	jsonError(w, "failed to generate preview", http.StatusInternalServerError)
}

// handleTrim applies manual trim bounds to a job.
// POST /api/v1/jobs/{id}/trim
func (s *Server) handleTrim(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	id := extractJobID(r.URL.Path, "/api/v1/jobs/", "/trim")
	job, ok := s.jobStore.Get(id)
	if !ok {
		jsonError(w, "job not found", http.StatusNotFound)
		return
	}

	var body struct {
		StartSec float64 `json:"start_sec"`
		EndSec   float64 `json:"end_sec"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	job.TrimStart = &body.StartSec
	job.TrimEnd = &body.EndSec

	jsonResponse(w, http.StatusOK, map[string]string{"status": "trim updated"})
}

// handleProcess finalizes processing and generates RINEX output.
// POST /api/v1/jobs/{id}/process
func (s *Server) handleProcess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	id := extractJobID(r.URL.Path, "/api/v1/jobs/", "/process")
	job, ok := s.jobStore.Get(id)
	if !ok {
		jsonError(w, "job not found", http.StatusNotFound)
		return
	}

	// Always generate both RINEX 2.11 and 3.x output formats.
	job.Format = "both"

	job.mu.Lock()
	job.Status = StatusProcessing
	job.mu.Unlock()

	// Get parsed epochs (from background parse or parse now).
	job.mu.Lock()
	epochs := job.epochs
	inputFile := job.InputFile
	job.mu.Unlock()

	if epochs == nil {
		f, err := os.Open(inputFile)
		if err != nil {
			job.Status = StatusFailed
			job.Error = "failed to open input: " + err.Error()
			jsonResponse(w, http.StatusInternalServerError, job)
			return
		}
		ptrs, _, parseErr := ubx.Parse(f)
		f.Close()
		if parseErr != nil {
			job.Status = StatusFailed
			job.Error = "parse error: " + parseErr.Error()
			jsonResponse(w, http.StatusInternalServerError, job)
			return
		}
		epochs = derefEpochs(ptrs)
		job.mu.Lock()
		job.epochs = epochs
		job.mu.Unlock()
	}

	// Apply trim: user bounds override auto-trim.
	workingEpochs := epochs
	if job.TrimStart != nil && job.TrimEnd != nil && len(workingEpochs) > 0 {
		sessionStart := workingEpochs[0].Time.UnixNanos()
		startNanos := sessionStart + int64(*job.TrimStart*1e9)
		endNanos := sessionStart + int64(*job.TrimEnd*1e9)
		var trimmed []gnss.Epoch
		for _, ep := range workingEpochs {
			t := ep.Time.UnixNanos()
			if t >= startNanos && t <= endNanos {
				trimmed = append(trimmed, ep)
			}
		}
		workingEpochs = trimmed
	} else if len(workingEpochs) > 0 {
		trimmed, _ := pipeline.AutoTrim(workingEpochs, pipeline.DefaultAutoTrimConfig())
		if len(trimmed) > 0 {
			workingEpochs = trimmed
		}
	}

	// Run processing pipeline (filter + normalize).
	cfg := pipeline.DefaultConfig()
	cfg.Trim = pipeline.TrimConfig{} // trimming already applied above
	processed, _ := pipeline.Process(workingEpochs, cfg)

	// Build output metadata with placeholder values.
	meta := buildMetadata(processed, 30)
	meta.ApproxX = job.ApproxX
	meta.ApproxY = job.ApproxY
	meta.ApproxZ = job.ApproxZ

	// Generate descriptive filename with standard RINEX extension (.YYO)
	var fileBase string
	var yearSuffix string
	if len(processed) > 0 {
		y, mo, d, h, mi, _ := rinex.GNSSTimeToCalendar(processed[0].Time)
		fileBase = fmt.Sprintf("rinexprep_%04d%02d%02d_%02d%02d", y, mo, d, h, mi)
		yearSuffix = fmt.Sprintf("%02dO", y%100)
	} else {
		now := time.Now().UTC()
		fileBase = fmt.Sprintf("rinexprep_%s", now.Format("20060102_1504"))
		yearSuffix = fmt.Sprintf("%02dO", now.Year()%100)
	}

	// Write output files.
	jobDir := filepath.Join(s.jobStore.dir, job.ID)
	var outputFiles []string

	if job.Format == "rinex2" || job.Format == "both" {
		obsName := fileBase + "_v2." + yearSuffix
		outPath := filepath.Join(jobDir, obsName)
		if err := writeRinex2File(outPath, meta, processed); err != nil {
			job.Status = StatusFailed
			job.Error = "rinex2 write error: " + err.Error()
			jsonResponse(w, http.StatusInternalServerError, job)
			return
		}
		outputFiles = append(outputFiles, filepath.Join(job.ID, obsName))
	}

	if job.Format == "rinex3" || job.Format == "both" {
		rnxName := fileBase + "_v3." + yearSuffix
		outPath := filepath.Join(jobDir, rnxName)
		if err := writeRinex3File(outPath, meta, processed); err != nil {
			job.Status = StatusFailed
			job.Error = "rinex3 write error: " + err.Error()
			jsonResponse(w, http.StatusInternalServerError, job)
			return
		}
		outputFiles = append(outputFiles, filepath.Join(job.ID, rnxName))
	}

	now := time.Now().UTC()
	job.mu.Lock()
	job.Status = StatusReady
	job.CompletedAt = &now
	job.OutputFiles = outputFiles
	job.mu.Unlock()

	slog.Info("process_complete", "job_id", job.ID, "format", "both", "epochs_out", len(processed))

	job.mu.Lock()
	defer job.mu.Unlock()
	jsonResponse(w, http.StatusOK, job)
}

// handleDownload serves the processed RINEX file(s).
// GET /api/v1/jobs/{id}/download?format=rinex2|rinex3  (or no param for zip)
func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	id := extractJobID(r.URL.Path, "/api/v1/jobs/", "/download")
	job, ok := s.jobStore.Get(id)
	if !ok {
		jsonError(w, "job not found", http.StatusNotFound)
		return
	}

	if job.Status != StatusReady || len(job.OutputFiles) == 0 {
		jsonError(w, "job is not ready for download", http.StatusConflict)
		return
	}

	format := r.URL.Query().Get("format")

	switch format {
	case "rinex2", "rinex3":
		tag := "_v2."
		if format == "rinex3" {
			tag = "_v3."
		}
		var filePath string
		for _, f := range job.OutputFiles {
			if strings.Contains(f, tag) {
				filePath = filepath.Join(s.jobStore.dir, f)
				break
			}
		}
		if filePath == "" {
			jsonError(w, format+" file not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, filepath.Base(filePath)))
		w.Header().Set("Content-Type", "application/octet-stream")
		http.ServeFile(w, r, filePath)

	default:
		// Zip all output files — explicitly close before return.
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="rinex_output.zip"`)

		zw := zip.NewWriter(w)

		for _, relPath := range job.OutputFiles {
			absPath := filepath.Join(s.jobStore.dir, relPath)
			f, err := os.Open(absPath)
			if err != nil {
				slog.Error("zip_open_failed", "path", absPath, "error", err)
				continue
			}
			fw, err := zw.Create(filepath.Base(absPath))
			if err != nil {
				f.Close()
				slog.Error("zip_create_failed", "error", err)
				continue
			}
			io.Copy(fw, f)
			f.Close()
		}
		if err := zw.Close(); err != nil {
			slog.Error("zip_finalize_failed", "error", err)
		}
	}
}

// handleListFiles returns the list of available output files for a job.
// GET /api/v1/jobs/{id}/files
func (s *Server) handleListFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	id := extractJobID(r.URL.Path, "/api/v1/jobs/", "/files")
	job, ok := s.jobStore.Get(id)
	if !ok {
		jsonError(w, "job not found", http.StatusNotFound)
		return
	}

	type FileInfo struct {
		Name   string `json:"name"`
		Format string `json:"format"`
		Size   int64  `json:"size"`
		Label  string `json:"label"`
	}

	var files []FileInfo

	for _, relPath := range job.OutputFiles {
		absPath := filepath.Join(s.jobStore.dir, relPath)
		info, err := os.Stat(absPath)
		if err != nil {
			continue
		}
		name := filepath.Base(relPath)
		format := "rinex3"
		label := "RINEX 3.03 (." + filepath.Ext(name)[1:] + ")"
		if strings.Contains(name, "_v2.") {
			format = "rinex2"
			label = "RINEX 2.11 (." + filepath.Ext(name)[1:] + ")"
		}
		files = append(files, FileInfo{
			Name: name, Format: format, Size: info.Size(), Label: label,
		})
	}

	if files == nil {
		files = []FileInfo{}
	}

	jsonResponse(w, http.StatusOK, map[string]any{"files": files})
}

// handleDelete removes a job and its associated files.
// DELETE /api/v1/jobs/{id}
func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	id := extractJobID(r.URL.Path, "/api/v1/jobs/", "")
	if !s.jobStore.Delete(id) {
		jsonError(w, "job not found", http.StatusNotFound)
		return
	}

	// Best-effort cleanup of job files.
	jobDir := filepath.Join(s.jobStore.dir, id)
	os.RemoveAll(jobDir)

	jsonResponse(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// --- preview helpers ---

// generatePreview builds PreviewData from parsed epochs.
func generatePreview(epochs []gnss.Epoch, navSatData []*ubx.NavSatEpoch) *PreviewData {
	if len(epochs) == 0 {
		return &PreviewData{
			Epochs:  []EpochSummary{},
			Skyview: []SatPosition{},
			QC:      QCSummary{Warnings: []string{"no valid UBX epochs found in file"}},
		}
	}

	startNanos := epochs[0].Time.UnixNanos()
	endNanos := epochs[len(epochs)-1].Time.UnixNanos()
	totalSecs := float64(endNanos-startNanos) / 1e9

	// Build epoch summaries and accumulate QC stats.
	summaries := make([]EpochSummary, len(epochs))
	var totalGPSSats float64
	var l2Count int
	for i, ep := range epochs {
		gpsSats := ep.GPSSatCount()
		summaries[i] = EpochSummary{
			TimeSec:   float64(ep.Time.UnixNanos()-startNanos) / 1e9,
			GPSSats:   gpsSats,
			TotalSats: len(ep.Satellites),
			AvgSNR:    computeAvgSNR(ep),
		}
		totalGPSSats += float64(gpsSats)
		if hasL2Signal(ep) {
			l2Count++
		}
	}

	// Auto-trim result for suggested bounds.
	_, autoResult := pipeline.AutoTrim(epochs, pipeline.DefaultAutoTrimConfig())
	trimBounds := TrimBounds{
		StartSec: autoResult.StartTrimmedSec,
		EndSec:   totalSecs - autoResult.EndTrimmedSec,
	}

	// QC metrics.
	n := float64(len(epochs))
	gpsMean := totalGPSSats / n
	l2Pct := float64(l2Count) / n * 100
	durHours := totalSecs / 3600

	var warnings []string
	var failures []string

	if durHours < 2 {
		failures = append(failures, "Session shorter than 2 hours (OPUS minimum)")
	} else if durHours < 4 {
		warnings = append(warnings, "Session shorter than 4 hours (recommended for best results)")
	}

	if gpsMean < 4 {
		failures = append(failures, fmt.Sprintf("Average GPS satellites %.1f < 4 minimum", gpsMean))
	} else if gpsMean < 6 {
		warnings = append(warnings, fmt.Sprintf("Average GPS satellites %.1f is marginal", gpsMean))
	}

	if l2Pct < 10 {
		failures = append(failures, "No L2 signal coverage (dual-frequency required by OPUS)")
	} else if l2Pct < 50 {
		warnings = append(warnings, fmt.Sprintf("L2 coverage %.0f%% is low (>80%% recommended)", l2Pct))
	} else if l2Pct < 80 {
		warnings = append(warnings, fmt.Sprintf("L2 coverage %.0f%% is acceptable but >80%% recommended", l2Pct))
	}

	qc := QCSummary{
		OPUSReady:     len(failures) == 0,
		Score:         computeScore(durHours, gpsMean, l2Pct),
		DurationHours: durHours,
		GPSSatsMean:   gpsMean,
		L2CoveragePct: l2Pct,
		Warnings:      warnings,
		Failures:      failures,
	}

	// Compute actual UTC start/end times from the first and last epochs.
	var startTimeUTC, endTimeUTC string
	if len(epochs) > 0 {
		sNanos := epochs[0].Time.UnixNanos()
		eNanos := epochs[len(epochs)-1].Time.UnixNanos()
		if epochs[0].Time.LeapValid {
			sNanos -= int64(epochs[0].Time.LeapSeconds) * 1e9
		}
		if epochs[len(epochs)-1].Time.LeapValid {
			eNanos -= int64(epochs[len(epochs)-1].Time.LeapSeconds) * 1e9
		}
		startTimeUTC = time.Unix(0, sNanos).UTC().Format(time.RFC3339)
		endTimeUTC = time.Unix(0, eNanos).UTC().Format(time.RFC3339)
	}

	// Build per-satellite frequency coverage map from RAWX observation data.
	type freqInfo struct {
		hasL1, hasL2, hasL5 bool
	}
	freqAccum := make(map[string]*freqInfo)
	for _, ep := range epochs {
		for _, sat := range ep.Satellites {
			if sat.Constellation != gnss.ConsGPS {
				continue
			}
			key := fmt.Sprintf("%s%d", sat.Constellation.String(), sat.PRN)
			fi, exists := freqAccum[key]
			if !exists {
				fi = &freqInfo{}
				freqAccum[key] = fi
			}
			for _, sig := range sat.Signals {
				switch sig.FreqBand {
				case 0:
					fi.hasL1 = true
				case 1:
					fi.hasL2 = true
				case 2:
					fi.hasL5 = true
				}
			}
		}
	}
	freqMap := make(map[string]string)
	for key, fi := range freqAccum {
		switch {
		case fi.hasL1 && fi.hasL2:
			freqMap[key] = "L1+L2"
		case fi.hasL1 && fi.hasL5:
			freqMap[key] = "L1+L5"
		case fi.hasL1:
			freqMap[key] = "L1"
		case fi.hasL2:
			freqMap[key] = "L2"
		default:
			freqMap[key] = "none"
		}
	}

	// Build skyview arcs from NAV-SAT data (all epochs, subsampled).
	startTOWNanos := epochs[0].Time.TOWNanos
	var skyview []SatPosition
	if len(navSatData) > 0 {
		step := 1
		if len(navSatData) > 100 {
			step = len(navSatData) / 100
		}
		const weekNanos = int64(7 * 24 * 3600) * int64(1e9)
		for i := 0; i < len(navSatData); i += step {
			nav := navSatData[i]
			navTOWNanos := int64(nav.ITOW) * int64(1e6)
			diffNanos := navTOWNanos - startTOWNanos
			// Handle GPS week rollover
			if diffNanos < -weekNanos/2 {
				diffNanos += weekNanos
			}
			timeSec := float64(diffNanos) / 1e9
			for _, sat := range nav.Satellites {
				if sat.Elevation <= 0 {
					continue
				}
				if gnss.Constellation(sat.GnssID) != gnss.ConsGPS {
					continue
				}
				satKey := fmt.Sprintf("G%d", sat.SvID)
				freqs := freqMap[satKey]
				if freqs == "" {
					freqs = "L1"
				}
				skyview = append(skyview, SatPosition{
					System:    gnss.Constellation(sat.GnssID).String(),
					PRN:       int(sat.SvID),
					Azimuth:   float64(sat.Azimuth),
					Elevation: float64(sat.Elevation),
					SNR:       float64(sat.CNO),
					TimeSec:   timeSec,
					Freqs:     freqs,
				})
			}
		}
	}
	// Generate sky tracks from RAWX observation data.
	// Each satellite gets a pseudo-arc based on its PRN (orbital slot) and visibility window.
	if len(skyview) == 0 && len(epochs) > 0 {
		type epochEntry struct {
			snr    float64
			hasL1  bool
			hasL2  bool
			hasL5  bool
			locked bool // true if all signals have lock time > 0
		}
		type satTrack struct {
			system     string
			prn        int
			firstEpoch int
			lastEpoch  int
			epochData  []epochEntry
		}

		tracks := make(map[string]*satTrack)

		for i, ep := range epochs {
			for _, sat := range ep.Satellites {
				if sat.Constellation != gnss.ConsGPS {
					continue
				}
				key := fmt.Sprintf("%s%d", sat.Constellation.String(), sat.PRN)
				bestSNR := 0.0
				hasL1, hasL2, hasL5 := false, false, false
				allLocked := true

				for _, sig := range sat.Signals {
					if sig.SNR > bestSNR {
						bestSNR = sig.SNR
					}
					switch sig.FreqBand {
					case 0:
						hasL1 = true
					case 1:
						hasL2 = true
					case 2:
						hasL5 = true
					}
					if sig.LockTimeSec <= 0 {
						allLocked = false
					}
				}

				t, exists := tracks[key]
				if !exists {
					t = &satTrack{
						system:     sat.Constellation.String(),
						prn:        int(sat.PRN),
						firstEpoch: i,
					}
					tracks[key] = t
				}
				t.lastEpoch = i
				t.epochData = append(t.epochData, epochEntry{
					snr:    bestSNR,
					hasL1:  hasL1,
					hasL2:  hasL2,
					hasL5:  hasL5,
					locked: allLocked,
				})
			}
		}

		// Generate arc positions for each satellite.
		totalEpochs := len(epochs)

		for _, t := range tracks {
			// Determine arc parameters from PRN.
			basePRN := t.prn
			if t.system == "R" {
				basePRN += 32
			}
			if t.system == "E" {
				basePRN += 56
			}
			if t.system == "C" {
				basePRN += 92
			}

			// Base azimuth: golden angle distribution for even spread.
			baseAz := math.Mod(float64(basePRN)*137.508, 360)

			// Arc sweep: satellites cross ~120-180 degrees of azimuth during a pass.
			arcSweep := 120.0 + math.Mod(float64(basePRN)*23.7, 60.0)
			startAz := baseAz - arcSweep/2

			// Peak elevation: use average SNR as a rough proxy.
			avgSNR := 0.0
			for _, ed := range t.epochData {
				avgSNR += ed.snr
			}
			avgSNR /= float64(len(t.epochData))
			peakEl := 20.0 + (avgSNR-20.0)/30.0*60.0
			if peakEl < 15 {
				peakEl = 15
			}
			if peakEl > 85 {
				peakEl = 85
			}

			// Generate ~20 points per arc (or fewer if satellite was briefly visible).
			numPoints := 20
			epochRange := t.lastEpoch - t.firstEpoch
			if epochRange < numPoints {
				numPoints = epochRange + 1
			}
			if numPoints < 2 {
				numPoints = 2
			}

			for j := 0; j < numPoints; j++ {
				frac := float64(j) / float64(numPoints-1)

				az := math.Mod(startAz+frac*arcSweep+360, 360)

				// Parabolic elevation curve peaking at frac=0.5.
				el := peakEl * (1 - math.Pow(2*frac-1, 2))
				if el < 5 {
					el = 5
				}

				epochIdx := t.firstEpoch + int(frac*float64(epochRange))
				if epochIdx >= totalEpochs {
					epochIdx = totalEpochs - 1
				}
				timeSec := float64(epochs[epochIdx].Time.UnixNanos()-startNanos) / 1e9

				// Map arc point to the nearest per-epoch data entry.
				dataIdx := int(frac * float64(len(t.epochData)-1))
				if dataIdx >= len(t.epochData) {
					dataIdx = len(t.epochData) - 1
				}
				ed := t.epochData[dataIdx]

				// Determine frequency string for this specific point.
				freqs := "none"
				if !ed.locked {
					freqs = "no_lock"
				} else if ed.hasL1 && ed.hasL2 {
					freqs = "L1+L2"
				} else if ed.hasL1 && ed.hasL5 {
					freqs = "L1+L5"
				} else if ed.hasL1 {
					freqs = "L1"
				} else if ed.hasL2 {
					freqs = "L2"
				}

				skyview = append(skyview, SatPosition{
					System:    t.system,
					PRN:       t.prn,
					Azimuth:   az,
					Elevation: el,
					SNR:       ed.snr,
					TimeSec:   timeSec,
					Freqs:     freqs,
				})
			}
		}
	}
	if skyview == nil {
		skyview = []SatPosition{}
	}

	// Count unique satellite passes and frequency stats.
	type satKey struct{ sys string; prn int }
	uniqueSats := make(map[satKey]bool)
	l1Sats := make(map[satKey]bool)
	l2Sats := make(map[satKey]bool)
	l5Sats := make(map[satKey]bool)
	var l1SNRTotal, l2SNRTotal float64
	var l1SNRCount, l2SNRCount int

	for _, ep := range epochs {
		for _, sat := range ep.Satellites {
			sk := satKey{sat.Constellation.String(), int(sat.PRN)}
			uniqueSats[sk] = true
			for _, sig := range sat.Signals {
				switch sig.FreqBand {
				case 0:
					l1Sats[sk] = true
					if sig.SNR > 0 {
						l1SNRTotal += sig.SNR
						l1SNRCount++
					}
				case 1:
					l2Sats[sk] = true
					if sig.SNR > 0 {
						l2SNRTotal += sig.SNR
						l2SNRCount++
					}
				case 2:
					l5Sats[sk] = true
				}
			}
		}
	}

	// Count dual-frequency satellites.
	dualFreq := 0
	for sk := range l1Sats {
		if l2Sats[sk] {
			dualFreq++
		}
	}

	// Max gap between epochs.
	maxGap := 0.0
	for i := 1; i < len(epochs); i++ {
		gap := float64(epochs[i].Time.UnixNanos()-epochs[i-1].Time.UnixNanos()) / 1e9
		if gap > maxGap {
			maxGap = gap
		}
	}

	preview := &PreviewData{
		Epochs:        downsampleEpochs(summaries, 1000),
		Skyview:       skyview,
		AutoTrim:      trimBounds,
		QC:            qc,
		TotalSecs:     totalSecs,
		StartTimeUTC:  startTimeUTC,
		EndTimeUTC:    endTimeUTC,
		SatPasses:     len(uniqueSats),
		L1Count:       len(l1Sats),
		L2Count:       len(l2Sats),
		L5Count:       len(l5Sats),
		DualFreqCount: dualFreq,
		MaxGap:        maxGap,
	}
	if l1SNRCount > 0 {
		preview.MeanSNRL1 = l1SNRTotal / float64(l1SNRCount)
	}
	if l2SNRCount > 0 {
		preview.MeanSNRL2 = l2SNRTotal / float64(l2SNRCount)
	}

	return preview
}

// downsampleEpochs reduces epoch summaries to at most maxPoints for chart display.
// Uses simple decimation — picks evenly spaced points.
func downsampleEpochs(summaries []EpochSummary, maxPoints int) []EpochSummary {
	if len(summaries) <= maxPoints {
		return summaries
	}
	step := float64(len(summaries)) / float64(maxPoints)
	result := make([]EpochSummary, 0, maxPoints)
	for i := 0; i < maxPoints; i++ {
		idx := int(float64(i) * step)
		if idx >= len(summaries) {
			idx = len(summaries) - 1
		}
		result = append(result, summaries[idx])
	}
	// Always include the last point
	if len(result) > 0 {
		result[len(result)-1] = summaries[len(summaries)-1]
	}
	return result
}

func computeAvgSNR(ep gnss.Epoch) float64 {
	var total float64
	var count int
	for _, sat := range ep.Satellites {
		for _, sig := range sat.Signals {
			if sig.SNR > 0 {
				total += sig.SNR
				count++
			}
		}
	}
	if count == 0 {
		return 0
	}
	return total / float64(count)
}

func hasL2Signal(ep gnss.Epoch) bool {
	for _, sat := range ep.Satellites {
		if sat.Constellation == gnss.ConsGPS {
			for _, sig := range sat.Signals {
				// GPS L2 signals: sigId 3 (L2 CL) or 4 (L2 CM)
				if sig.FreqBand == 1 || sig.SigID == 3 || sig.SigID == 4 {
					return true
				}
			}
		}
	}
	return false
}

func computeScore(durHours, gpsMean, l2Pct float64) float64 {
	score := 0.0
	// Duration: 40% weight, full marks at 4+ hours
	if durHours >= 4 {
		score += 40
	} else if durHours >= 2 {
		score += 20 + 20*(durHours-2)/2
	} else {
		score += 20 * (durHours / 2)
	}
	// GPS satellites: 30% weight, full marks at 8+ mean
	if gpsMean >= 8 {
		score += 30
	} else if gpsMean >= 4 {
		score += 15 + 15*(gpsMean-4)/4
	} else {
		score += 15 * (gpsMean / 4)
	}
	// L2 coverage: 30% weight, full marks at 90%+
	if l2Pct >= 90 {
		score += 30
	} else if l2Pct >= 50 {
		score += 15 + 15*(l2Pct-50)/40
	} else {
		score += 15 * (l2Pct / 50)
	}
	return score
}

// --- processing helpers ---

func derefEpochs(ptrs []*gnss.Epoch) []gnss.Epoch {
	if ptrs == nil {
		return nil
	}
	result := make([]gnss.Epoch, len(ptrs))
	for i, p := range ptrs {
		result[i] = *p
	}
	return result
}

func buildMetadata(epochs []gnss.Epoch, intervalSec float64) gnss.Metadata {
	meta := gnss.Metadata{
		MarkerName:   "UNKNOWN",
		MarkerNumber: "UNKNOWN",
		ReceiverType: "UNKNOWN",
		AntennaType:  "UNKNOWN NONE",
		Observer:     "UNKNOWN",
		Agency:       "UNKNOWN",
		Interval:     intervalSec,
	}
	if len(epochs) > 0 {
		meta.FirstEpoch = epochs[0].Time
		meta.LastEpoch = epochs[len(epochs)-1].Time
	}
	return meta
}

func writeRinex2File(path string, meta gnss.Metadata, epochs []gnss.Epoch) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return rinex.WriteRinex2(f, meta, epochs)
}

func writeRinex3File(path string, meta gnss.Metadata, epochs []gnss.Epoch) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return rinex.WriteRinex3(f, meta, epochs)
}

// --- routing helpers ---

// extractJobID pulls the job ID from a URL path between the given prefix and suffix.
func extractJobID(path, prefix, suffix string) string {
	s := strings.TrimPrefix(path, prefix)
	if suffix != "" {
		s = strings.TrimSuffix(s, suffix)
	}
	return s
}

func jsonResponse(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
