package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
)

// OPUSRequest holds the parameters for an OPUS submission.
type OPUSRequest struct {
	Email       string  `json:"email"`
	AntennaType string  `json:"antenna_type"`
	Height      float64 `json:"height"`
	Mode        string  `json:"mode"` // "static" or "rapid"
}

// handleOPUSSubmit proxies the processed RINEX file to NGS OPUS.
// POST /api/v1/jobs/{id}/opus
func (s *Server) handleOPUSSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := extractJobID(r.URL.Path, "/api/v1/jobs/", "/opus")
	job, ok := s.jobStore.Get(id)
	if !ok {
		jsonError(w, "job not found", http.StatusNotFound)
		return
	}

	if job.Status != StatusReady || len(job.OutputFiles) == 0 {
		jsonError(w, "job must be processed before submitting to OPUS", http.StatusConflict)
		return
	}

	var req OPUSRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	if req.Email == "" {
		jsonError(w, "email is required", http.StatusBadRequest)
		return
	}
	if req.AntennaType == "" {
		jsonError(w, "antenna_type is required", http.StatusBadRequest)
		return
	}

	// Find the RINEX 2.11 .obs file (OPUS prefers RINEX 2)
	var rinexPath string
	for _, f := range job.OutputFiles {
		if filepath.Ext(f) == ".obs" {
			rinexPath = filepath.Join(s.jobStore.dir, f)
			break
		}
	}
	if rinexPath == "" {
		// Fall back to .rnx (RINEX 3)
		for _, f := range job.OutputFiles {
			if filepath.Ext(f) == ".rnx" {
				rinexPath = filepath.Join(s.jobStore.dir, f)
				break
			}
		}
	}
	if rinexPath == "" {
		jsonError(w, "no RINEX output file found", http.StatusInternalServerError)
		return
	}

	// Build multipart form for OPUS
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add the RINEX file
	rinexFile, err := os.Open(rinexPath)
	if err != nil {
		jsonError(w, "failed to open RINEX file", http.StatusInternalServerError)
		return
	}
	defer rinexFile.Close()

	part, err := writer.CreateFormFile("uploadfile", filepath.Base(rinexPath))
	if err != nil {
		jsonError(w, "failed to create form", http.StatusInternalServerError)
		return
	}
	if _, err := io.Copy(part, rinexFile); err != nil {
		jsonError(w, "failed to read RINEX file", http.StatusInternalServerError)
		return
	}

	// Add form fields — field names must match the OPUS upload form
	writer.WriteField("email_address", req.Email)
	writer.WriteField("ant_type", req.AntennaType)
	writer.WriteField("height", fmt.Sprintf("%.3f", req.Height))
	// Hidden fields required by OPUS form
	writer.WriteField("extend_code", "0")
	writer.WriteField("xml_code", "0")
	writer.WriteField("set_profile", "0")
	writer.WriteField("delete_profile", "0")
	writer.WriteField("share", "2")
	writer.WriteField("submit_database", "0")
	writer.WriteField("opusOption", "0")

	writer.Close()

	// Submit to the correct OPUS CGI endpoint
	var opusURL string
	if req.Mode == "rapid" {
		opusURL = "https://geodesy.noaa.gov/OPUS-cgi/OPUS/Upload/Opus-rsup.prl"
	} else {
		opusURL = "https://geodesy.noaa.gov/OPUS-cgi/OPUS/Upload/Opusup.prl"
	}
	opusReq, err := http.NewRequest("POST", opusURL, &buf)
	if err != nil {
		jsonError(w, "failed to create OPUS request", http.StatusInternalServerError)
		return
	}
	opusReq.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{}
	resp, err := client.Do(opusReq)
	if err != nil {
		log.Printf("OPUS submission failed for job %s: %v", id, err)
		jsonError(w, "OPUS submission failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	log.Printf("OPUS submission for job %s: status=%d", id, resp.StatusCode)

	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"status":        "submitted",
			"message":       "Your RINEX file has been submitted to OPUS. Results will be emailed to " + req.Email,
			"opus_response": string(body),
		})
	} else {
		jsonResponse(w, http.StatusBadGateway, map[string]interface{}{
			"status":        "failed",
			"message":       "OPUS returned an error",
			"opus_response": string(body),
		})
	}
}
