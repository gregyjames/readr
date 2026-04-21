package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func BenchmarkSPAHandler(b *testing.B) {
	distDir := "testdata/dist"
	handler := NewSPAHandler(distDir)

	// We benchmark a directory request as it triggers the redundant os.Stat calls
	req, _ := http.NewRequest("GET", "/subdir/", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}
}

func BenchmarkSPAHandler_File(b *testing.B) {
	distDir := "testdata/dist"
	handler := NewSPAHandler(distDir)

	req, _ := http.NewRequest("GET", "/file.txt", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}
}

func BenchmarkSPAHandler_Fallback(b *testing.B) {
	distDir := "testdata/dist"
	handler := NewSPAHandler(distDir)

	req, _ := http.NewRequest("GET", "/non-existent", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}
}
