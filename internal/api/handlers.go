package api

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"log"
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
		log.Printf("parse error for job %s: %v", job.ID, err)
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
		log.Printf("parse error for job %s: %v", job.ID, err)
		return
	}

	job.mu.Lock()
	job.Progress = "Analyzing satellite visibility..."
	job.mu.Unlock()

	epochs := derefEpochs(ptrs)

	job.mu.Lock()
	job.Progress = "Computing quality metrics..."
	job.mu.Unlock()

	preview := generatePreview(epochs, stats.NavSatData)

	job.mu.Lock()
	job.epochs = epochs
	job.Preview = preview
	job.Status = StatusPreview
	job.Progress = "Preview ready"
	job.mu.Unlock()
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

	var body struct {
		Format string `json:"format"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	switch body.Format {
	case "rinex2", "rinex3", "both":
		job.Format = body.Format
	default:
		jsonError(w, `format must be "rinex2", "rinex3", or "both"`, http.StatusBadRequest)
		return
	}

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

	// Write output files.
	jobDir := filepath.Join(s.jobStore.dir, job.ID)
	var outputFiles []string

	if job.Format == "rinex2" || job.Format == "both" {
		outPath := filepath.Join(jobDir, "output.obs")
		if err := writeRinex2File(outPath, meta, processed); err != nil {
			job.Status = StatusFailed
			job.Error = "rinex2 write error: " + err.Error()
			jsonResponse(w, http.StatusInternalServerError, job)
			return
		}
		outputFiles = append(outputFiles, filepath.Join(job.ID, "output.obs"))
	}

	if job.Format == "rinex3" || job.Format == "both" {
		outPath := filepath.Join(jobDir, "output.rnx")
		if err := writeRinex3File(outPath, meta, processed); err != nil {
			job.Status = StatusFailed
			job.Error = "rinex3 write error: " + err.Error()
			jsonResponse(w, http.StatusInternalServerError, job)
			return
		}
		outputFiles = append(outputFiles, filepath.Join(job.ID, "output.rnx"))
	}

	now := time.Now().UTC()
	job.mu.Lock()
	job.Status = StatusReady
	job.CompletedAt = &now
	job.OutputFiles = outputFiles
	job.mu.Unlock()

	job.mu.Lock()
	defer job.mu.Unlock()
	jsonResponse(w, http.StatusOK, job)
}

// handleDownload serves the processed RINEX file(s).
// GET /api/v1/jobs/{id}/download
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

	// Single output file: serve directly.
	if len(job.OutputFiles) == 1 {
		outPath := filepath.Join(s.jobStore.dir, job.OutputFiles[0])
		if _, err := os.Stat(outPath); os.IsNotExist(err) {
			jsonError(w, "output file not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(outPath)))
		http.ServeFile(w, r, outPath)
		return
	}

	// Multiple output files (format "both"): create a zip.
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="rinex_output.zip"`)

	zw := zip.NewWriter(w)
	defer zw.Close()

	for _, relPath := range job.OutputFiles {
		absPath := filepath.Join(s.jobStore.dir, relPath)
		f, err := os.Open(absPath)
		if err != nil {
			log.Printf("zip: failed to open %s: %v", absPath, err)
			continue
		}
		fw, err := zw.Create(filepath.Base(absPath))
		if err != nil {
			f.Close()
			log.Printf("zip: failed to create entry: %v", err)
			continue
		}
		io.Copy(fw, f)
		f.Close()
	}
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
				skyview = append(skyview, SatPosition{
					System:    gnss.Constellation(sat.GnssID).String(),
					PRN:       int(sat.SvID),
					Azimuth:   float64(sat.Azimuth),
					Elevation: float64(sat.Elevation),
					SNR:       float64(sat.CNO),
					TimeSec:   timeSec,
				})
			}
		}
	}
	// Generate sky tracks from RAWX observation data.
	// Each satellite gets a pseudo-arc based on its PRN (orbital slot) and visibility window.
	if len(skyview) == 0 && len(epochs) > 0 {
		type satTrack struct {
			system     string
			prn        int
			firstEpoch int
			lastEpoch  int
			snrSamples []float64
		}

		tracks := make(map[string]*satTrack)

		for i, ep := range epochs {
			for _, sat := range ep.Satellites {
				key := fmt.Sprintf("%s%d", sat.Constellation.String(), sat.PRN)
				bestSNR := 0.0
				for _, sig := range sat.Signals {
					if sig.SNR > bestSNR {
						bestSNR = sig.SNR
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
				t.snrSamples = append(t.snrSamples, bestSNR)
			}
		}

		// Generate arc positions for each satellite.
		totalEpochs := len(epochs)

		for _, t := range tracks {
			// Determine arc parameters from PRN.
			// Use PRN with offsets per constellation to spread satellites around the sky.
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
			for _, s := range t.snrSamples {
				avgSNR += s
			}
			avgSNR /= float64(len(t.snrSamples))
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

				snrIdx := int(frac * float64(len(t.snrSamples)-1))
				if snrIdx >= len(t.snrSamples) {
					snrIdx = len(t.snrSamples) - 1
				}

				skyview = append(skyview, SatPosition{
					System:    t.system,
					PRN:       t.prn,
					Azimuth:   az,
					Elevation: el,
					SNR:       t.snrSamples[snrIdx],
					TimeSec:   timeSec,
				})
			}
		}
	}
	if skyview == nil {
		skyview = []SatPosition{}
	}

	return &PreviewData{
		Epochs:       downsampleEpochs(summaries, 1000),
		Skyview:      skyview,
		AutoTrim:     trimBounds,
		QC:           qc,
		TotalSecs:    totalSecs,
		StartTimeUTC: startTimeUTC,
		EndTimeUTC:   endTimeUTC,
	}
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
