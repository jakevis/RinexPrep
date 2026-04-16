package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// newTestServer creates a server backed by a temporary directory.
func newTestServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	dir := t.TempDir()
	s := &Server{
		port:     0,
		jobStore: NewJobStore(dir),
		mux:      http.NewServeMux(),
	}
	s.SetupRoutes()
	ts := httptest.NewServer(s.mux)
	t.Cleanup(ts.Close)
	return s, ts
}

// uploadFile performs a multipart upload and returns the decoded response body.
func uploadFile(t *testing.T, ts *httptest.Server, filename string, content []byte) map[string]string {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	w.Close()

	resp, err := http.Post(ts.URL+"/api/v1/upload", w.FormDataContentType(), &buf)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("upload returned %d: %s", resp.StatusCode, body)
	}

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestUpload(t *testing.T) {
	_, ts := newTestServer(t)
	result := uploadFile(t, ts, "test.ubx", []byte("fake ubx data"))

	id, ok := result["jobId"]
	if !ok || id == "" {
		t.Fatal("upload response missing 'id'")
	}
}

func TestUploadCreatesFile(t *testing.T) {
	s, ts := newTestServer(t)
	result := uploadFile(t, ts, "test.ubx", []byte("fake ubx data"))
	id := result["jobId"]

	path := filepath.Join(s.jobStore.dir, id, "input.ubx")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected input file at %s: %v", path, err)
	}
	if string(data) != "fake ubx data" {
		t.Fatalf("unexpected file content: %q", data)
	}
}

func TestStatusAfterUpload(t *testing.T) {
	_, ts := newTestServer(t)
	result := uploadFile(t, ts, "test.ubx", []byte("data"))
	id := result["jobId"]

	resp, err := http.Get(ts.URL + "/api/v1/jobs/" + id + "/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var job Job
	if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
		t.Fatal(err)
	}
	// After upload, the background goroutine may have already progressed
	// the status beyond "uploaded" (parsing, preview, or failed for invalid data).
	validStatuses := map[JobStatus]bool{
		StatusUploaded: true,
		StatusParsing:  true,
		StatusPreview:  true,
		StatusFailed:   true,
	}
	if !validStatuses[job.Status] {
		t.Fatalf("expected a valid post-upload status, got %q", job.Status)
	}
	if job.ID != id {
		t.Fatalf("expected id %q, got %q", id, job.ID)
	}
}

func TestStatusNotFound(t *testing.T) {
	_, ts := newTestServer(t)
	resp, err := http.Get(ts.URL + "/api/v1/jobs/nonexistent/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestDelete(t *testing.T) {
	_, ts := newTestServer(t)
	result := uploadFile(t, ts, "test.ubx", []byte("data"))
	id := result["jobId"]

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/jobs/"+id, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete expected 200, got %d", resp.StatusCode)
	}

	// Verify the job is gone.
	resp2, err := http.Get(ts.URL + "/api/v1/jobs/" + id + "/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", resp2.StatusCode)
	}
}

func TestDeleteNotFound(t *testing.T) {
	_, ts := newTestServer(t)
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/jobs/nonexistent", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestUploadFileSizeLimit(t *testing.T) {
	_, ts := newTestServer(t)

	// Create a body that claims to be larger than 600 MB.
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", "huge.ubx")
	if err != nil {
		t.Fatal(err)
	}

	// Write slightly over 600 MB. We write in 1 MB chunks to avoid allocating
	// a single huge slice.
	chunk := make([]byte, 1<<20) // 1 MB
	for i := 0; i < 601; i++ {
		if _, err := part.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}
	w.Close()

	resp, err := http.Post(ts.URL+"/api/v1/upload", w.FormDataContentType(), &buf)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 413, got %d: %s", resp.StatusCode, body)
	}
}

func TestCORSHeaders(t *testing.T) {
	_, ts := newTestServer(t)
	req, _ := http.NewRequest(http.MethodOptions, ts.URL+"/api/v1/upload", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("missing CORS header Access-Control-Allow-Origin")
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("OPTIONS expected 204, got %d", resp.StatusCode)
	}
}

func TestFrontendPlaceholder(t *testing.T) {
	_, ts := newTestServer(t)
	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if ct != "text/html; charset=utf-8" {
		t.Fatalf("expected text/html, got %q", ct)
	}

	body, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(body, []byte("RinexPrep")) {
		t.Fatal("placeholder page does not contain 'RinexPrep'")
	}
}

func TestPreview(t *testing.T) {
	_, ts := newTestServer(t)
	result := uploadFile(t, ts, "test.ubx", []byte("data"))
	id := result["jobId"]

	resp, err := http.Get(ts.URL + "/api/v1/jobs/" + id + "/preview")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("preview expected 200, got %d: %s", resp.StatusCode, body)
	}

	var preview PreviewData
	if err := json.NewDecoder(resp.Body).Decode(&preview); err != nil {
		t.Fatal(err)
	}
	// With non-UBX input data, epochs may be empty; verify the response is valid JSON.
	if preview.Epochs == nil {
		t.Fatal("expected epochs field in preview (may be empty)")
	}
}

func TestTrim(t *testing.T) {
	_, ts := newTestServer(t)
	result := uploadFile(t, ts, "test.ubx", []byte("data"))
	id := result["jobId"]

	body := `{"start_sec": 5.0, "end_sec": 55.0}`
	resp, err := http.Post(ts.URL+"/api/v1/jobs/"+id+"/trim", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("trim expected 200, got %d: %s", resp.StatusCode, b)
	}

	// Verify trim was stored on the job.
	job, err := ts.Client().Get(ts.URL + "/api/v1/jobs/" + id + "/status")
	if err != nil {
		t.Fatal(err)
	}
	defer job.Body.Close()
	var j Job
	json.NewDecoder(job.Body).Decode(&j)
	if j.TrimStart == nil || *j.TrimStart != 5.0 {
		t.Fatalf("expected trim start 5.0, got %v", j.TrimStart)
	}
}

func TestProcess(t *testing.T) {
	_, ts := newTestServer(t)
	result := uploadFile(t, ts, "test.ubx", []byte("data"))
	id := result["jobId"]

	body := `{"format": "rinex3"}`
	resp, err := http.Post(ts.URL+"/api/v1/jobs/"+id+"/process", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("process expected 200, got %d: %s", resp.StatusCode, b)
	}

	var j Job
	json.NewDecoder(resp.Body).Decode(&j)
	if j.Status != StatusReady {
		t.Fatalf("expected status %q, got %q", StatusReady, j.Status)
	}
	if j.Format != "rinex3" {
		t.Fatalf("expected format rinex3, got %q", j.Format)
	}
}

func TestProcessInvalidFormat(t *testing.T) {
	_, ts := newTestServer(t)
	result := uploadFile(t, ts, "test.ubx", []byte("data"))
	id := result["jobId"]

	body := `{"format": "invalid"}`
	resp, err := http.Post(ts.URL+"/api/v1/jobs/"+id+"/process", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

// Silence the "imported and not used" error for fmt if needed.
var _ = fmt.Sprintf
