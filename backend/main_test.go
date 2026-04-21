package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

func BenchmarkDownloadImagesSequential(b *testing.B) {
	// Setup a mock server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("fake image data"))
	}))
	defer ts.Close()

	images := make([]string, 10)
	for i := 0; i < 10; i++ {
		images[i] = fmt.Sprintf("%s/image%d.png", ts.URL, i)
	}

	tempDir, _ := os.MkdirTemp("", "bench")
	defer os.RemoveAll(tempDir)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		markdownContent := "test content"
		filenameID := int64(12345)

		for _, imgURL := range images {
			imgResp, err := http.Get(imgURL)
			if err != nil || imgResp.StatusCode != 200 {
				continue
			}
			// Simulate the current bug of defer in loop for baseline
			// Actually, to make it fair and not leak, I'll close it inside the loop in the baseline if I were to fix it,
			// but I should replicate the current code as closely as possible.
			// However, b.N can be large, so defer in loop will definitely cause issues.

			func() {
				defer imgResp.Body.Close()
				parts := strings.Split(imgURL, "/")
				filename := parts[len(parts)-1]
				savePath := tempDir + "/" + filename

				out, _ := os.Create(savePath)
				io.Copy(out, imgResp.Body)
				out.Close()

				markdownContent = strings.ReplaceAll(markdownContent, imgURL, fmt.Sprintf("/images/%d/", filenameID)+filename)
			}()
		}
		_ = markdownContent
	}
}

func BenchmarkDownloadImagesParallel(b *testing.B) {
	// Setup a mock server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("fake image data"))
	}))
	defer ts.Close()

	images := make([]string, 10)
	for i := 0; i < 10; i++ {
		images[i] = fmt.Sprintf("%s/image%d.png", ts.URL, i)
	}

	tempDir, _ := os.MkdirTemp("", "bench_parallel")
	defer os.RemoveAll(tempDir)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		markdownContent := "test content"
		filenameID := int64(12345)

		var wg sync.WaitGroup
		var mu sync.Mutex
		replacements := make([]string, 0, len(images)*2)

		for _, imgURL := range images {
			wg.Add(1)
			go func(url string) {
				defer wg.Done()
				imgResp, err := http.Get(url)
				if err != nil || imgResp.StatusCode != 200 {
					return
				}
				defer imgResp.Body.Close()

				parts := strings.Split(url, "/")
				filename := parts[len(parts)-1]
				savePath := tempDir + "/" + filename

				out, err := os.Create(savePath)
				if err != nil {
					return
				}
				io.Copy(out, imgResp.Body)
				out.Close()

				mu.Lock()
				replacements = append(replacements, url, fmt.Sprintf("/images/%d/", filenameID)+filename)
				mu.Unlock()
			}(imgURL)
		}
		wg.Wait()

		replacer := strings.NewReplacer(replacements...)
		markdownContent = replacer.Replace(markdownContent)
		_ = markdownContent
	}
}
