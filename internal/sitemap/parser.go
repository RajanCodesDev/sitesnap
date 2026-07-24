package sitemap

import (
	"context"
	"encoding/xml"
	"io"
	"net/http"
)

// Parse recursively parses one or more sitemap URLs and returns every
// unique URL contained within them.
func Parse(
	ctx context.Context,
	client *http.Client,
	sitemaps []string,
) ([]string, error) {

	var urls []string

	visitedSitemaps := make(map[string]bool)
	visitedURLs := make(map[string]bool)

	for _, sm := range sitemaps {
		if err := parse(
			ctx,
			client,
			sm,
			visitedSitemaps,
			visitedURLs,
			&urls,
		); err != nil {
			return nil, err
		}
	}

	return urls, nil
}

func parse(
	ctx context.Context,
	client *http.Client,
	sitemapURL string,
	visitedSitemaps map[string]bool,
	visitedURLs map[string]bool,
	urls *[]string,
) error {

	if visitedSitemaps[sitemapURL] {
		return nil
	}
	visitedSitemaps[sitemapURL] = true

	reader, err := Fetch(ctx, client, sitemapURL)
	if err != nil {
		return err
	}
	if reader == nil {
		// Sitemap doesn't exist (404).
		return nil
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}

	// Try parsing as a normal sitemap.
	var urlset URLSet
	if err := xml.Unmarshal(data, &urlset); err == nil &&
		len(urlset.URLs) > 0 {

		for _, u := range urlset.URLs {

			if u.Loc == "" {
				continue
			}

			if visitedURLs[u.Loc] {
				continue
			}

			visitedURLs[u.Loc] = true
			*urls = append(*urls, u.Loc)
		}

		return nil
	}

	// Otherwise try parsing as a sitemap index.
	var index SitemapIndex
	if err := xml.Unmarshal(data, &index); err == nil &&
		len(index.Sitemaps) > 0 {

		for _, sm := range index.Sitemaps {

			if sm.Loc == "" {
				continue
			}

			if err := parse(
				ctx,
				client,
				sm.Loc,
				visitedSitemaps,
				visitedURLs,
				urls,
			); err != nil {
				return err
			}
		}

		return nil
	}

	// Unknown XML format.
	return nil
}
