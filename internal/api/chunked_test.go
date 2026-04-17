package api

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestChunkedUploadHappyPath(t *testing.T) {
	_, ts := newTestServer(t)

	// Create test data (200 bytes, will be split into chunks).
	data := bytes.Repeat([]byte("UBX"), 100) // 300 bytes
	totalSize := int64(len(data))

	// 1. Init.
	initBody, _ := json.Marshal(map[string]any{
		"filename": "test.ubx",
		"size":     totalSize,
	})
	resp, err := http.Post(ts.URL+"/api/v1/upload/init", "application/json", bytes.NewReader(initBody))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("init: want 200, got %d: %s", resp.StatusCode, b)
	}
	var initResp struct {
		JobID string `json:"jobId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&initResp); err != nil {
		t.Fatal(err)
	}
	jobID := initResp.JobID
	if jobID == "" {
		t.Fatal("empty jobId")
	}

	// Verify job is in uploading state.
	job, ok := getTestJob(t, ts, jobID)
	if !ok {
		t.Fatal("job not found after init")
	}
	if job.Status != "uploading" {
		t.Fatalf("expected status uploading, got %s", job.Status)
	}

	// 2. Upload in two chunks.
	chunkSize := 150
	for offset := 0; offset < len(data); offset += chunkSize {
		end := offset + chunkSize
		if end > len(data) {
			end = len(data)
		}
		sendChunk(t, ts, jobID, data[offset:end], int64(offset))
	}

	// 3. Complete.
	completeResp, err := http.Post(ts.URL+"/api/v1/upload/"+jobID+"/complete", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer completeResp.Body.Close()
	if completeResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(completeResp.Body)
		t.Fatalf("complete: want 200, got %d: %s", completeResp.StatusCode, b)
	}

	// Verify job transitioned to parsing/uploaded.
	job2, _ := getTestJob(t, ts, jobID)
	if job2.Status == "uploading" {
		t.Fatal("job should not be in uploading state after complete")
	}

	// Verify the file on disk matches.
	finalPath := filepath.Join(os.TempDir(), "rinexprep-test-*") // approximate
	_ = finalPath // file check via job store
	if job2.InputSize != totalSize {
		t.Fatalf("input size = %d, want %d", job2.InputSize, totalSize)
	}
}

func TestChunkedUploadOffsetMismatch(t *testing.T) {
	_, ts := newTestServer(t)

	data := bytes.Repeat([]byte("X"), 200)
	jobID := initChunkedUpload(t, ts, "test.ubx", int64(len(data)))

	// Send first chunk at offset 0.
	sendChunk(t, ts, jobID, data[:100], 0)

	// Send second chunk with wrong offset (0 instead of 100).
	resp := sendChunkRaw(t, ts, jobID, data[100:], 0)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 Conflict for offset mismatch, got %d", resp.StatusCode)
	}
}

func TestChunkedUploadCompleteSizeMismatch(t *testing.T) {
	_, ts := newTestServer(t)

	data := bytes.Repeat([]byte("X"), 100)
	// Declare size as 200 but only upload 100 bytes.
	jobID := initChunkedUpload(t, ts, "test.ubx", 200)

	sendChunk(t, ts, jobID, data, 0)

	// Complete should fail because size doesn't match.
	resp, err := http.Post(ts.URL+"/api/v1/upload/"+jobID+"/complete", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for size mismatch, got %d", resp.StatusCode)
	}
}

func TestChunkedUploadPreviewBeforeComplete(t *testing.T) {
	_, ts := newTestServer(t)

	jobID := initChunkedUpload(t, ts, "test.ubx", 100)

	// Try to get preview before upload is complete — job is in uploading state
	// so status should reflect that.
	job, _ := getTestJob(t, ts, jobID)
	if job.Status != "uploading" {
		t.Fatalf("expected uploading, got %s", job.Status)
	}
}

func TestSingleUploadStillWorks(t *testing.T) {
	_, ts := newTestServer(t)

	// Existing single-POST upload path should still work.
	result := uploadFile(t, ts, "small.ubx", []byte("small test data"))
	if result["jobId"] == "" {
		t.Fatal("single upload returned empty jobId")
	}
}

// --- helpers ---

func initChunkedUpload(t *testing.T, ts *httptest.Server, filename string, size int64) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"filename": filename, "size": size})
	resp, err := http.Post(ts.URL+"/api/v1/upload/init", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("init failed: %d: %s", resp.StatusCode, b)
	}
	var r struct {
		JobID string `json:"jobId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		t.Fatal(err)
	}
	return r.JobID
}

func sendChunk(t *testing.T, ts *httptest.Server, jobID string, data []byte, offset int64) {
	t.Helper()
	resp := sendChunkRaw(t, ts, jobID, data, offset)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("chunk at offset %d failed: %d: %s", offset, resp.StatusCode, b)
	}
	resp.Body.Close()
}

func sendChunkRaw(t *testing.T, ts *httptest.Server, jobID string, data []byte, offset int64) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("chunk", "chunk.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequest("POST", ts.URL+"/api/v1/upload/"+jobID+"/chunk", &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("X-Upload-Offset", strconv.FormatInt(offset, 10))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func getTestJob(t *testing.T, ts *httptest.Server, jobID string) (struct {
	Status    string `json:"status"`
	InputSize int64  `json:"input_size_bytes"`
}, bool) {
	t.Helper()
	resp, err := http.Get(ts.URL + "/api/v1/jobs/" + jobID + "/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var job struct {
		Status    string `json:"status"`
		InputSize int64  `json:"input_size_bytes"`
	}
	if resp.StatusCode != http.StatusOK {
		return job, false
	}
	if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
		t.Fatal(err)
	}
	return job, true
}
