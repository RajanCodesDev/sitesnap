package crawler

// Package crawler performs a concurrent breadth-first crawl of a website.
//
// It uses a fixed pool of worker goroutines. A single coordinator goroutine
// owns the work queue and the set of discovered URLs, feeding an unbuffered
// jobs channel and draining a results channel in a select. Because the
// coordinator never blocks on a send while workers wait on it (it can always
// receive a result in the same select), the design is deadlock-free and scales
// to hundreds of concurrent workers.

import (
	"context"
	"fmt"
	"github.com/RajanCodesDev/sitesnap/internal/parser"
	"github.com/RajanCodesDev/sitesnap/internal/sitemap"
	"github.com/RajanCodesDev/sitesnap/internal/snapshot"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

// Config controls a crawl.
type Config struct {
	BaseURL   string
	Workers   int
	Timeout   time.Duration
	UserAgent string
	// Progress, if set, is called after each page is processed with the number
	// of pages crawled so far and the number still pending.
	Progress func(crawled, pending int)
}

type job struct {
	url       string
	parentURL string
}

type result struct {
	page  snapshot.Page
	links []string
}

// Crawl fetches and walks the website rooted at cfg.BaseURL, returning a
// Snapshot of every discovered URL.
func Crawl(cfg Config) (*snapshot.Snapshot, error) {
	base, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base url: %w", err)
	}
	if base.Scheme != "http" && base.Scheme != "https" {
		return nil, fmt.Errorf("base url must use http or https")
	}
	if base.Host == "" {
		return nil, fmt.Errorf("base url must include a host")
	}

	workers := cfg.Workers
	if workers < 1 {
		workers = 1
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Follow redirects but never leave the base host.
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			if !strings.EqualFold(req.URL.Host, base.Host) {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
	// Discover sitemap URLs.
	var sitemapURLs []string

	sitemaps, err := sitemap.DiscoverSitemaps(ctx, client, cfg.BaseURL)
	if err == nil {
		sitemapURLs, err = sitemap.Parse(ctx, client, sitemaps)
		fmt.Println("Discovered sitemap files:", len(sitemaps))
		fmt.Println("URLs from sitemap:", len(sitemapURLs))
		if err != nil {
			sitemapURLs = nil
		}
	}

	jobs := make(chan job)
	results := make(chan result)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			worker(ctx, client, base, jobs, results)
		}()
	}

	visited := make(map[string]bool)
	var pages []snapshot.Page

	seed := canonicalize(cfg.BaseURL)

	visited[seed] = true

	toSend := []job{
		{
			url: seed,
		},
	}

	pending := 1

	for _, u := range sitemapURLs {
		u = canonicalize(u)

		if visited[u] {
			continue
		}

		visited[u] = true

		toSend = append(toSend, job{
			url: u,
		})

		pending++
	}

	crawled := 0

	for pending > 0 {
		if len(toSend) > 0 {
			select {
			case jobs <- toSend[0]:
				toSend = toSend[1:]
			case res := <-results:
				pages = append(pages, res.page)
				pending, crawled, toSend = handleResult(res, pending, crawled, visited, toSend)
				if cfg.Progress != nil {
					cfg.Progress(crawled, pending)
				}
			}
		} else {
			res := <-results
			pages = append(pages, res.page)
			pending, crawled, toSend = handleResult(res, pending, crawled, visited, toSend)
			if cfg.Progress != nil {
				cfg.Progress(crawled, pending)
			}
		}
	}

	close(jobs)
	wg.Wait()

	sort.Slice(pages, func(i, j int) bool { return pages[i].URL < pages[j].URL })

	fmt.Println("Pages crawled:", len(pages))

	return &snapshot.Snapshot{
		BaseURL:   cfg.BaseURL,
		CrawledAt: time.Now().UTC(),
		Pages:     pages,
	}, nil
}

// handleResult enqueues any newly discovered internal links from a result.
// It returns the updated pending count, crawled count, and send queue. The
// crawled page itself is appended to pages by the caller.
func handleResult(res result, pending, crawled int, visited map[string]bool, toSend []job) (int, int, []job) {
	pending--
	crawled++
	for _, l := range res.links {
		l = canonicalize(l)
		if visited[l] {
			continue
		}
		visited[l] = true
		pending++
		toSend = append(toSend, job{url: l, parentURL: res.page.URL})
	}
	return pending, crawled, toSend
}

func worker(ctx context.Context, client *http.Client, base *url.URL, jobs <-chan job, results chan<- result) {
	for j := range jobs {
		select {
		case <-ctx.Done():
			return
		case results <- fetch(ctx, client, base, j):
		}
	}
}

func fetch(ctx context.Context, client *http.Client, base *url.URL, j job) result {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, j.url, nil)
	if err != nil {
		return result{page: snapshot.Page{URL: j.url, ParentURL: j.parentURL, StatusCode: 0}}
	}
	req.Header.Set("User-Agent", "SiteSnap/0.1")

	resp, err := client.Do(req)
	if err != nil {
		// Unreachable / network error: record with status 0.
		return result{page: snapshot.Page{URL: j.url, ParentURL: j.parentURL, StatusCode: 0}}
	}
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	page := snapshot.Page{
		URL:          j.url,
		ParentURL:    j.parentURL,
		StatusCode:   resp.StatusCode,
		ContentType:  ct,
		ResourceType: resourceType(ct),
	}

	var links []string
	if isHTML(ct) {
		links, _ = parser.ExtractLinks(base, resp.Body)
	}
	return result{page: page, links: links}
}

func isHTML(ct string) bool {
	return strings.HasPrefix(strings.ToLower(ct), "text/html")
}

// resourceType derives an optional coarse classification from a content type.
func resourceType(ct string) string {
	ct = strings.ToLower(strings.TrimSpace(ct))
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = ct[:i]
	}
	ct = strings.TrimSpace(ct)
	switch {
	case ct == "":
		return ""
	case ct == "text/html":
		return "html"
	case ct == "text/css":
		return "css"
	case strings.Contains(ct, "javascript") || ct == "application/json":
		return "script"
	case strings.HasPrefix(ct, "image/"):
		return "image"
	case strings.HasPrefix(ct, "font/"):
		return "font"
	case strings.HasPrefix(ct, "text/"):
		return "text"
	default:
		return "other"
	}
}

// canonicalize normalizes a URL string so that equivalent forms (e.g. a host
// with and without a trailing slash) compare equal.
func canonicalize(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if u.Path == "" {
		u.Path = "/"
	}
	u.Fragment = ""
	return u.String()
}
