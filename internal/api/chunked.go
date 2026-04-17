package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const maxChunkSize = 95 << 20 // 95 MB — safely under Cloudflare's 100 MB limit

// handleUploadInit creates a new job for a chunked upload.
// POST /api/v1/upload/init
func (s *Server) handleUploadInit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Filename string `json:"filename"`
		Size     int64  `json:"size"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.Filename == "" || req.Size <= 0 {
		jsonError(w, "filename and size are required", http.StatusBadRequest)
		return
	}
	if req.Size > maxUploadSize {
		jsonError(w, "file exceeds 600 MB limit", http.StatusRequestEntityTooLarge)
		return
	}

	now := time.Now().UTC()
	job := s.jobStore.Create(req.Filename, 0)
	job.mu.Lock()
	job.Status = StatusUploading
	job.ExpectedSize = req.Size
	job.LastActivity = now
	job.mu.Unlock()

	jobDir := filepath.Join(s.jobStore.dir, job.ID)
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		jsonError(w, "failed to create job directory", http.StatusInternalServerError)
		return
	}

	// Create empty .part file.
	partPath := filepath.Join(jobDir, "input.ubx.part")
	f, err := os.Create(partPath)
	if err != nil {
		jsonError(w, "failed to create upload file", http.StatusInternalServerError)
		return
	}
	f.Close()

	slog.Info("upload_init", "job_id", job.ID, "filename", req.Filename, "expected_size", req.Size)

	jsonResponse(w, http.StatusOK, map[string]any{
		"jobId":        job.ID,
		"expectedSize": req.Size,
	})
}

// handleUploadChunk receives a single chunk and appends it to the .part file.
// POST /api/v1/upload/{id}/chunk
// Header: X-Upload-Offset — expected byte offset (must match current .part size)
func (s *Server) handleUploadChunk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	jobID := extractChunkJobID(r.URL.Path)
	if jobID == "" {
		jsonError(w, "missing job ID", http.StatusBadRequest)
		return
	}

	job, ok := s.jobStore.Get(jobID)
	if !ok {
		jsonError(w, "job not found", http.StatusNotFound)
		return
	}

	job.mu.Lock()
	if job.Status != StatusUploading {
		job.mu.Unlock()
		jsonError(w, "job is not in uploading state", http.StatusConflict)
		return
	}
	job.mu.Unlock()

	// Parse expected offset.
	offsetStr := r.Header.Get("X-Upload-Offset")
	if offsetStr == "" {
		jsonError(w, "X-Upload-Offset header required", http.StatusBadRequest)
		return
	}
	expectedOffset, err := strconv.ParseInt(offsetStr, 10, 64)
	if err != nil {
		jsonError(w, "invalid X-Upload-Offset", http.StatusBadRequest)
		return
	}

	// Limit chunk body size.
	r.Body = http.MaxBytesReader(w, r.Body, maxChunkSize+1024) // small buffer for multipart framing

	if err := r.ParseMultipartForm(maxChunkSize); err != nil {
		jsonError(w, "invalid multipart form: "+err.Error(), http.StatusBadRequest)
		return
	}

	chunk, _, err := r.FormFile("chunk")
	if err != nil {
		jsonError(w, "missing 'chunk' field", http.StatusBadRequest)
		return
	}
	defer chunk.Close()

	// Lock the job for writing to prevent concurrent chunk uploads.
	job.mu.Lock()
	defer job.mu.Unlock()

	partPath := filepath.Join(s.jobStore.dir, jobID, "input.ubx.part")

	// Validate offset matches current file size.
	info, err := os.Stat(partPath)
	if err != nil {
		jsonError(w, "upload file not found", http.StatusInternalServerError)
		return
	}
	currentSize := info.Size()
	if currentSize != expectedOffset {
		jsonError(w, "offset mismatch: expected "+strconv.FormatInt(currentSize, 10)+", got "+offsetStr, http.StatusConflict)
		return
	}

	// Append chunk data.
	f, err := os.OpenFile(partPath, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		jsonError(w, "failed to open upload file", http.StatusInternalServerError)
		return
	}

	written, err := io.Copy(f, chunk)
	if closeErr := f.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		// Truncate back to the offset on failure to keep the file consistent.
		if truncErr := os.Truncate(partPath, currentSize); truncErr != nil {
			slog.Error("truncate_failed", "job_id", jobID, "error", truncErr)
		}
		jsonError(w, "failed to write chunk", http.StatusInternalServerError)
		return
	}

	newSize := currentSize + written
	job.InputSize = newSize
	job.LastActivity = time.Now().UTC()

	// Check if we've exceeded the declared size.
	if newSize > job.ExpectedSize {
		jsonError(w, "upload exceeds declared size", http.StatusBadRequest)
		return
	}

	slog.Info("upload_chunk", "job_id", jobID, "offset", currentSize, "written", written, "total", newSize)

	jsonResponse(w, http.StatusOK, map[string]any{
		"received": newSize,
	})
}

// handleUploadComplete finalizes a chunked upload.
// POST /api/v1/upload/{id}/complete
func (s *Server) handleUploadComplete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	jobID := extractCompleteJobID(r.URL.Path)
	if jobID == "" {
		jsonError(w, "missing job ID", http.StatusBadRequest)
		return
	}

	job, ok := s.jobStore.Get(jobID)
	if !ok {
		jsonError(w, "job not found", http.StatusNotFound)
		return
	}

	job.mu.Lock()
	if job.Status != StatusUploading {
		job.mu.Unlock()
		jsonError(w, "job is not in uploading state", http.StatusConflict)
		return
	}

	jobDir := filepath.Join(s.jobStore.dir, jobID)
	partPath := filepath.Join(jobDir, "input.ubx.part")
	finalPath := filepath.Join(jobDir, "input.ubx")

	// Verify file size matches declared size.
	info, err := os.Stat(partPath)
	if err != nil {
		job.mu.Unlock()
		jsonError(w, "upload file not found", http.StatusInternalServerError)
		return
	}
	if info.Size() != job.ExpectedSize {
		job.mu.Unlock()
		jsonError(w, "size mismatch: got "+strconv.FormatInt(info.Size(), 10)+
			", expected "+strconv.FormatInt(job.ExpectedSize, 10), http.StatusBadRequest)
		return
	}

	// Rename .part → input.ubx.
	if err := os.Rename(partPath, finalPath); err != nil {
		job.mu.Unlock()
		jsonError(w, "failed to finalize upload", http.StatusInternalServerError)
		return
	}

	job.InputFile = finalPath
	job.InputSize = info.Size()
	job.Status = StatusUploaded
	job.LastActivity = time.Now().UTC()
	job.mu.Unlock()

	slog.Info("upload_complete", "job_id", jobID, "size_bytes", info.Size())

	// Kick off background parsing.
	go s.parseAndPreview(job)

	jsonResponse(w, http.StatusOK, map[string]any{
		"jobId":  jobID,
		"status": string(StatusParsing),
	})
}

// extractChunkJobID extracts the job ID from /api/v1/upload/{id}/chunk.
func extractChunkJobID(path string) string {
	const prefix = "/api/v1/upload/"
	const suffix = "/chunk"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return ""
	}
	return path[len(prefix) : len(path)-len(suffix)]
}

// extractCompleteJobID extracts the job ID from /api/v1/upload/{id}/complete.
func extractCompleteJobID(path string) string {
	const prefix = "/api/v1/upload/"
	const suffix = "/complete"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return ""
	}
	return path[len(prefix) : len(path)-len(suffix)]
}
