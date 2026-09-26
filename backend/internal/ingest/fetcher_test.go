package ingest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPFetcher_LimitReader(t *testing.T) {
	// Serve a response larger than max body size limit (e.g. 15MB)
	largeBody := strings.Repeat("A", 15*1024*1024)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(largeBody))
	}))
	defer ts.Close()

	fetcher := NewHTTPFetcher(5 * time.Second)
	// Enable private IP for local httptest server
	fetcher.AllowLocalhost = true

	data, err := fetcher.FetchHTML(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("unexpected error fetching HTML: %v", err)
	}

	if len(data) > MaxHTMLBytes {
		t.Errorf("expected HTML payload <= %d bytes, got %d", MaxHTMLBytes, len(data))
	}
}

func TestHTTPFetcher_SSRFProtection(t *testing.T) {
	fetcher := NewHTTPFetcher(2 * time.Second)
	fetcher.AllowLocalhost = false // default production behavior

	blockedURLs := []string{
		"http://127.0.0.1:8080/secret",
		"http://localhost:8080/secret",
		"http://169.254.169.254/latest/meta-data/",
		"http://10.0.0.1/admin",
		"http://192.168.1.1/router",
		"http://172.16.0.1/internal",
	}

	for _, u := range blockedURLs {
		_, err := fetcher.FetchHTML(context.Background(), u)
		if err == nil {
			t.Errorf("expected SSRF block for %s, but request succeeded", u)
		} else if !strings.Contains(err.Error(), "blocked") && !strings.Contains(err.Error(), "private") && !strings.Contains(err.Error(), "denied") {
			t.Logf("URL %s failed as expected with: %v", u, err)
		}
	}
}

func TestHTTPFetcher_ContentTypeValidation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write([]byte("PK\x03\x04fakezip"))
	}))
	defer ts.Close()

	fetcher := NewHTTPFetcher(5 * time.Second)
	fetcher.AllowLocalhost = true

	_, err := fetcher.FetchHTML(context.Background(), ts.URL)
	if err == nil {
		t.Fatalf("expected error fetching non-HTML content type, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported content-type") {
		t.Errorf("expected 'unsupported content-type' error, got %v", err)
	}
}

func TestHTTPFetcher_BrowserHeaders(t *testing.T) {
	var capturedUserAgent string
	var capturedAccept string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUserAgent = r.Header.Get("User-Agent")
		capturedAccept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html><body>Hello</body></html>"))
	}))
	defer ts.Close()

	fetcher := NewHTTPFetcher(5 * time.Second)
	fetcher.AllowLocalhost = true

	_, err := fetcher.FetchHTML(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("unexpected fetch error: %v", err)
	}

	if !strings.Contains(capturedUserAgent, "Mozilla/5.0") {
		t.Errorf("expected realistic browser User-Agent containing Mozilla/5.0, got %q", capturedUserAgent)
	}
	if !strings.Contains(capturedAccept, "text/html") {
		t.Errorf("expected Accept header with text/html, got %q", capturedAccept)
	}
}
