package sitemap

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// DiscoverSitemaps reads robots.txt and returns every Sitemap: entry.
//
// If no Sitemap entries are found, it falls back to /sitemap.xml.
func DiscoverSitemaps(
	ctx context.Context,
	client *http.Client,
	baseURL string,
) ([]string, error) {

	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}

	robotsURL := *u
	robotsURL.Path = "/robots.txt"
	robotsURL.RawQuery = ""
	robotsURL.Fragment = ""

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		robotsURL.String(),
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
	defer resp.Body.Close()

	var sitemaps []string

	switch resp.StatusCode {

	case http.StatusOK:

		scanner := bufio.NewScanner(resp.Body)

		for scanner.Scan() {

			line := strings.TrimSpace(scanner.Text())

			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}

			lower := strings.ToLower(line)

			if strings.HasPrefix(lower, "sitemap:") {

				loc := strings.TrimSpace(line[len("Sitemap:"):])

				if loc != "" {
					sitemaps = append(sitemaps, loc)
				}
			}
		}

		if err := scanner.Err(); err != nil {
			return nil, err
		}

	case http.StatusNotFound:
		// robots.txt is optional.

	default:
		return nil, fmt.Errorf(
			"unexpected status %d fetching robots.txt",
			resp.StatusCode,
		)
	}

	// Fallback to the conventional sitemap location.
	if len(sitemaps) == 0 {

		sitemapURL := *u
		sitemapURL.Path = "/sitemap.xml"
		sitemapURL.RawQuery = ""
		sitemapURL.Fragment = ""

		sitemaps = append(sitemaps, sitemapURL.String())
	}

	return sitemaps, nil
}
