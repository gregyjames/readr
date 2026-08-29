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

func (f *HTTPFetcher) fetch(ctx context.Context, url string, userAgent string, errMsg string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create %s request failed: %w", errMsg, err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s failed: %w", errMsg, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s returned status: %d", errMsg, resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s response body failed: %w", errMsg, err)
	}

	return data, nil
}

func (f *HTTPFetcher) FetchHTML(ctx context.Context, rawURL string) ([]byte, error) {
	return f.fetch(ctx, rawURL, "Readr/1.0 (Article Extractor; +https://github.com/gregyjames/readr)", "page")
}

func (f *HTTPFetcher) FetchImage(ctx context.Context, imgURL string) ([]byte, error) {
	return f.fetch(ctx, imgURL, "Readr/1.0 (Image Extractor; +https://github.com/gregyjames/readr)", "image")
}
