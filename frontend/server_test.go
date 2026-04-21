package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSPAHandler(t *testing.T) {
	distDir := "testdata/dist"
	handler := NewSPAHandler(distDir)

	tests := []struct {
		name           string
		path           string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "Existing file",
			path:           "/file.txt",
			expectedStatus: http.StatusOK,
			expectedBody:   "some file",
		},
		{
			name:           "Directory with index.html",
			path:           "/subdir/",
			expectedStatus: http.StatusOK,
			expectedBody:   "subdir index",
		},
		{
			name:           "SPA fallback for non-existent path",
			path:           "/non-existent",
			expectedStatus: http.StatusOK,
			expectedBody:   "root index",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", tt.path, nil)
			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expectedStatus {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, tt.expectedStatus)
			}

			if body := strings.TrimSpace(rr.Body.String()); body != tt.expectedBody {
				t.Errorf("handler returned unexpected body: got %v want %v",
					body, tt.expectedBody)
			}
		})
	}
}
