package api

import (
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
)

// Server is the RinexPrep HTTP API server.
type Server struct {
	port       int
	jobStore   *JobStore
	mux        *http.ServeMux
	frontendFS fs.FS
}

// NewServer creates a Server that listens on the given port.
// Job files are stored under a "jobs" directory inside dataDir.
func NewServer(port int, dataDir string) *Server {
	s := &Server{
		port:     port,
		jobStore: NewJobStore(dataDir),
		mux:      http.NewServeMux(),
	}
	s.SetupRoutes()
	return s
}

// SetupRoutes configures all API and frontend routes.
func (s *Server) SetupRoutes() {
	// CORS-aware API routes with request logging.
	s.mux.HandleFunc("/api/v1/upload/", s.cors(requestLogger(s.routeUpload)))
	s.mux.HandleFunc("/api/v1/upload", s.cors(requestLogger(s.handleUpload)))
	s.mux.HandleFunc("/api/v1/jobs/", s.cors(requestLogger(s.routeJobs)))

	// SPA frontend — catch-all.
	s.mux.HandleFunc("/", s.serveFrontend)
}

// routeUpload dispatches /api/v1/upload/... to chunked upload handlers.
func (s *Server) routeUpload(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	switch {
	case path == "/api/v1/upload/init":
		s.handleUploadInit(w, r)
	case hasSuffix(path, "/chunk"):
		s.handleUploadChunk(w, r)
	case hasSuffix(path, "/complete"):
		s.handleUploadComplete(w, r)
	default:
		http.NotFound(w, r)
	}
}

// routeJobs dispatches /api/v1/jobs/{id}/... to the correct handler.
func (s *Server) routeJobs(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	switch {
	case hasSuffix(path, "/status"):
		s.handleJobStatus(w, r)
	case hasSuffix(path, "/preview"):
		s.handlePreview(w, r)
	case hasSuffix(path, "/trim"):
		s.handleTrim(w, r)
	case hasSuffix(path, "/process"):
		s.handleProcess(w, r)
	case hasSuffix(path, "/download"):
		s.handleDownload(w, r)
	case hasSuffix(path, "/files"):
		s.handleListFiles(w, r)
	case hasSuffix(path, "/opus"):
		s.handleOPUSSubmit(w, r)
	default:
		// /api/v1/jobs/{id} — DELETE
		s.handleDelete(w, r)
	}
}

// cors wraps a handler with permissive CORS headers for development.
func (s *Server) cors(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Upload-Offset")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

// Start begins listening and blocks until the server stops.
func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.port)
	slog.Info("server_start", "port", s.port, "data_dir", s.jobStore.dir, "version", "0.1.0")
	return http.ListenAndServe(addr, s.mux)
}

// SetFrontendFS sets the filesystem used to serve the embedded frontend.
func (s *Server) SetFrontendFS(fsys fs.FS) {
	s.frontendFS = fsys
}

// hasSuffix is a small helper for route matching.
func hasSuffix(path, suffix string) bool {
	return len(path) > len(suffix) && path[len(path)-len(suffix):] == suffix
}
