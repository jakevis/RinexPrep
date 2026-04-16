package api

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/jakevis/rinexprep/internal/gnss"
)

// JobStatus represents the current state of a processing job.
type JobStatus string

const (
	StatusUploaded   JobStatus = "uploaded"
	StatusParsing    JobStatus = "parsing"
	StatusPreview    JobStatus = "preview"
	StatusProcessing JobStatus = "processing"
	StatusReady      JobStatus = "ready"
	StatusFailed     JobStatus = "failed"
)

// Job represents a single UBX→RINEX processing job.
type Job struct {
	mu     sync.Mutex   // protects epochs and Preview
	epochs []gnss.Epoch // parsed epochs, not serialized

	ID          string     `json:"id"`
	Status      JobStatus  `json:"status"`
	Progress    string     `json:"progress,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	InputFile   string     `json:"input_file"`
	InputSize   int64      `json:"input_size_bytes"`
	OutputFiles []string   `json:"output_files,omitempty"`
	Error       string     `json:"error,omitempty"`
	Preview     *PreviewData `json:"preview,omitempty"`
	TrimStart   *float64   `json:"trim_start_sec,omitempty"`
	TrimEnd     *float64   `json:"trim_end_sec,omitempty"`
	Format      string     `json:"format"`
}

// PreviewData contains parsed observation summary for the frontend.
type PreviewData struct {
	Epochs       []EpochSummary `json:"epochs"`
	Skyview      []SatPosition  `json:"skyview"`
	AutoTrim     TrimBounds     `json:"auto_trim"`
	QC           QCSummary      `json:"qc"`
	TotalSecs    float64        `json:"total_duration_sec"`
	StartTimeUTC string         `json:"start_time_utc"` // ISO 8601 format
	EndTimeUTC   string         `json:"end_time_utc"`   // ISO 8601 format
}

// EpochSummary is a reduced view of one observation epoch.
type EpochSummary struct {
	TimeSec   float64 `json:"time_sec"`
	GPSSats   int     `json:"gps_sats"`
	TotalSats int     `json:"total_sats"`
	AvgSNR    float64 `json:"avg_snr"`
}

// SatPosition describes a satellite's position for sky-view rendering.
type SatPosition struct {
	System    string  `json:"system"`
	PRN       int     `json:"prn"`
	Azimuth   float64 `json:"azimuth"`
	Elevation float64 `json:"elevation"`
	SNR       float64 `json:"snr"`
	TimeSec   float64 `json:"time_sec"`  // seconds from session start
	Freqs     string  `json:"freqs"`     // "L1", "L2", "L1+L2", "L1+L5", etc.
}

// TrimBounds defines start/end trim points in seconds from session start.
type TrimBounds struct {
	StartSec float64 `json:"start_sec"`
	EndSec   float64 `json:"end_sec"`
}

// QCSummary holds quality-control metrics for OPUS readiness.
type QCSummary struct {
	OPUSReady     bool     `json:"opus_ready"`
	Score         float64  `json:"score"`
	DurationHours float64  `json:"duration_hours"`
	GPSSatsMean   float64  `json:"gps_sats_mean"`
	L2CoveragePct float64  `json:"l2_coverage_pct"`
	Warnings      []string `json:"warnings,omitempty"`
	Failures      []string `json:"failures,omitempty"`
}

// JobStore is a thread-safe in-memory store for processing jobs.
type JobStore struct {
	mu   sync.RWMutex
	jobs map[string]*Job
	dir  string
}

// NewJobStore creates a JobStore that uses dir as the base for job files.
func NewJobStore(dir string) *JobStore {
	return &JobStore{
		jobs: make(map[string]*Job),
		dir:  dir,
	}
}

// Create adds a new job in "uploaded" status and returns it.
func (js *JobStore) Create(inputFile string, inputSize int64) *Job {
	id := generateID()
	now := time.Now().UTC()
	job := &Job{
		ID:        id,
		Status:    StatusUploaded,
		CreatedAt: now,
		InputFile: inputFile,
		InputSize: inputSize,
	}
	js.mu.Lock()
	js.jobs[id] = job
	js.mu.Unlock()
	return job
}

// Get retrieves a job by ID.
func (js *JobStore) Get(id string) (*Job, bool) {
	js.mu.RLock()
	defer js.mu.RUnlock()
	job, ok := js.jobs[id]
	return job, ok
}

// Delete removes a job by ID, returning true if it existed.
func (js *JobStore) Delete(id string) bool {
	js.mu.Lock()
	defer js.mu.Unlock()
	if _, ok := js.jobs[id]; !ok {
		return false
	}
	delete(js.jobs, id)
	return true
}

// List returns all jobs (snapshot).
func (js *JobStore) List() []*Job {
	js.mu.RLock()
	defer js.mu.RUnlock()
	result := make([]*Job, 0, len(js.jobs))
	for _, j := range js.jobs {
		result = append(result, j)
	}
	return result
}

// generateID produces a 16-byte random hex string (32 chars).
func generateID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}
