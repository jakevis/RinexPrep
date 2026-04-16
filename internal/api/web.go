package api

import (
	"io/fs"
	"net/http"
	"strings"
)

// placeholderHTML is served when the embedded frontend is not available.
const placeholderHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>RinexPrep</title>
  <style>
    body { font-family: system-ui, sans-serif; display: flex; align-items: center;
           justify-content: center; min-height: 100vh; margin: 0; background: #f5f5f5; }
    .card { text-align: center; padding: 2rem; background: white; border-radius: 8px;
            box-shadow: 0 2px 8px rgba(0,0,0,0.1); }
    h1 { color: #333; } p { color: #666; }
  </style>
</head>
<body>
  <div class="card">
    <h1>RinexPrep</h1>
    <p>Frontend loading&hellip;</p>
    <p style="font-size:0.85rem;color:#999;">The React build will be embedded here.</p>
  </div>
</body>
</html>`

// serveFrontend serves the SPA frontend. All non-API paths return the
// index page so that client-side routing works correctly.
func (s *Server) serveFrontend(w http.ResponseWriter, r *http.Request) {
	if s.frontendFS == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(placeholderHTML))
		return
	}

	// Try to serve a static file from the embedded frontend.
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}

	// Check if the requested file exists in the embedded FS.
	if f, err := s.frontendFS.Open(path); err == nil {
		f.Close()
		http.FileServer(http.FS(s.frontendFS)).ServeHTTP(w, r)
		return
	}

	// SPA fallback: serve index.html for any non-file path.
	indexData, err := fs.ReadFile(s.frontendFS, "index.html")
	if err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(placeholderHTML))
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(indexData)
}
