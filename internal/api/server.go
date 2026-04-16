package api

import (
	"fmt"
	"io/fs"
	"log"
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
	// CORS-aware API routes.
	s.mux.HandleFunc("/api/v1/upload", s.cors(s.handleUpload))
	s.mux.HandleFunc("/api/v1/jobs/", s.cors(s.routeJobs))

	// SPA frontend — catch-all.
	s.mux.HandleFunc("/", s.serveFrontend)
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
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

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
	log.Printf("RinexPrep API server listening on %s", addr)
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
