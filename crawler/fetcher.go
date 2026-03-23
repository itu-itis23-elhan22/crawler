package crawler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Fetcher handles HTTP requests to download web pages.
type Fetcher struct {
	client *http.Client
}

// NewFetcher creates a Fetcher with a configured timeout.
// Timeout prevents the system from hanging on slow/unresponsive servers.
func NewFetcher(timeout time.Duration) *Fetcher {
	return &Fetcher{
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

// Fetch downloads the content of a URL and returns it as a string.
// It only accepts HTML pages — skips binary content like images or PDFs.
func (f *Fetcher) Fetch(ctx context.Context, rawURL string) (string, error) {
	// Create a request with context so it can be cancelled
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating request for %s: %w", rawURL, err)
	}

	// Set a User-Agent so servers know we're a crawler
	req.Header.Set("User-Agent", "SimpleCrawler/1.0")

	// Send the HTTP request
	resp, err := f.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	// Check if the response status is OK (200)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d for %s", resp.StatusCode, rawURL)
	}

	// Only process HTML content — skip images, PDFs, etc.
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		return "", fmt.Errorf("not HTML (content-type: %s) for %s", contentType, rawURL)
	}

	// Read the page body with a size limit (10MB max to prevent memory issues)
	const maxSize = 10 * 1024 * 1024
	limitedReader := io.LimitReader(resp.Body, maxSize)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return "", fmt.Errorf("reading body of %s: %w", rawURL, err)
	}

	return string(body), nil
}
