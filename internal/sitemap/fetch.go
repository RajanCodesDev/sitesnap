package sitemap

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Fetch downloads a sitemap and returns a reader for its XML.
//
// It automatically decompresses .xml.gz sitemaps.
func Fetch(
	ctx context.Context,
	client *http.Client,
	url string,
) (io.ReadCloser, error) {

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		url,
		nil,
	)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "SiteSnap/0.1")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf(
			"unexpected status %d fetching sitemap %q",
			resp.StatusCode,
			url,
		)
	}

	// Handle compressed sitemap files.
	if strings.HasSuffix(strings.ToLower(url), ".gz") ||
		strings.Contains(
			strings.ToLower(resp.Header.Get("Content-Encoding")),
			"gzip",
		) {

		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			resp.Body.Close()
			return nil, err
		}

		return &gzipReadCloser{
			Reader: gz,
			body:   resp.Body,
		}, nil
	}

	return resp.Body, nil
}

// gzipReadCloser closes both the gzip reader and
// the underlying HTTP response body.
type gzipReadCloser struct {
	*gzip.Reader
	body io.Closer
}

func (g *gzipReadCloser) Close() error {
	_ = g.Reader.Close()
	return g.body.Close()
}
