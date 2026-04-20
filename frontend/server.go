package main

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	distDir := os.Getenv("DIST_DIR")
	if distDir == "" {
		distDir = "dist"
	}

	backendURLStr := os.Getenv("BACKEND_URL")
	if backendURLStr == "" {
		backendURLStr = "http://backend:3000"
	}

	backendURL, err := url.Parse(backendURLStr)
	if err != nil {
		log.Fatalf("Invalid backend URL: %v", err)
	}

	proxy := httputil.NewSingleHostReverseProxy(backendURL)

	// Create file system from dist directory
	fileSystem := http.Dir(distDir)
	fileServer := http.FileServer(fileSystem)

	// Custom handler for SPA routing
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Forward requests to backend APIs and assets
		if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/images/") || strings.HasPrefix(path, "/articles/") {
			proxy.ServeHTTP(w, r)
			return
		}

		// Check if file exists
		fullPath := filepath.Join(distDir, path)
		if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}

		// Check for index.html in directory
		if info, err := os.Stat(fullPath); err == nil && info.IsDir() {
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
	})

	log.Printf("Server running on http://0.0.0.0:%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

