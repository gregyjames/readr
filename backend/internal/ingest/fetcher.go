package ingest

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

type HTTPFetcher struct {
	client *http.Client
}

func NewHTTPFetcher(timeout time.Duration) *HTTPFetcher {
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &HTTPFetcher{
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

func (f *HTTPFetcher) FetchHTML(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}
	req.Header.Set("User-Agent", "Readr/1.0 (Article Extractor; +https://github.com/gregyjames/readr)")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch page failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch page returned status: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body failed: %w", err)
	}

	return data, nil
}

func (f *HTTPFetcher) FetchImage(ctx context.Context, imgURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imgURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create image request failed: %w", err)
	}
	req.Header.Set("User-Agent", "Readr/1.0 (Image Extractor; +https://github.com/gregyjames/readr)")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch image failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch image returned status: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read image data failed: %w", err)
	}

	return data, nil
}
