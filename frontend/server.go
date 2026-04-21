package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
)

func NewSPAHandler(distDir string) http.HandlerFunc {
	fileServer := http.FileServer(http.Dir(distDir))

	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Check if file exists
		fullPath := filepath.Join(distDir, path)
		info, err := os.Stat(fullPath)
		if err == nil && !info.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}

		// Check for index.html in directory
		if err == nil && info.IsDir() {
			indexPath := filepath.Join(fullPath, "index.html")
			if _, err := os.Stat(indexPath); err == nil {
				http.ServeFile(w, r, indexPath)
				return
			}
		}

		// For SPA routing, serve index.html for all other routes
		indexPath := filepath.Join(distDir, "index.html")
		if _, err := os.Stat(indexPath); err == nil {
			http.ServeFile(w, r, indexPath)
			return
		}

		http.NotFound(w, r)
	}
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	distDir := os.Getenv("DIST_DIR")
	if distDir == "" {
		distDir = "dist"
	}

	http.HandleFunc("/", NewSPAHandler(distDir))

	log.Printf("Server running on http://0.0.0.0:%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
