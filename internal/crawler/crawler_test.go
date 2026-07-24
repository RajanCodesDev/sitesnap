package crawler

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/RajanCodesDev/sitesnap/internal/snapshot"
)

// testServer returns an httptest server with a small linked site.
func testServer(hits *int32) *httptest.Server {
	mux := http.NewServeMux()
	pages := map[string]string{
		"/":        `<a href="/a">A</a><a href="/b">B</a>`,
		"/a":       `<a href="/">Home</a><a href="/c">C</a>`,
		"/b":       `<a href="/a">A</a>`,
		"/c":       `<a href="/missing">M</a>`,
		"/missing": `not found`,
	}
	for path, body := range pages {
		body := body
		p := path
		mux.HandleFunc(p, func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(hits, 1)
			if p == "/missing" {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte(body))
				return
			}
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(body))
		})
	}
	return httptest.NewServer(mux)
}

func TestCrawlDiscoversAllPages(t *testing.T) {
	var hits int32
	srv := testServer(&hits)
	defer srv.Close()

	snap, err := Crawl(Config{BaseURL: srv.URL, Workers: 10, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("crawl: %v", err)
	}

	// /, /a, /b, /c, /missing = 5 unique URLs.
	if len(snap.Pages) != 5 {
		t.Fatalf("got %d pages, want 5: %v", len(snap.Pages), urls(snap))
	}

	byURL := index(snap)
	if byURL[srv.URL+"/"] == nil || byURL[srv.URL+"/a"] == nil ||
		byURL[srv.URL+"/b"] == nil || byURL[srv.URL+"/c"] == nil {
		t.Fatalf("missing expected pages: %v", urls(snap))
	}
	if got := byURL[srv.URL+"/missing"]; got == nil || got.StatusCode != 404 {
		t.Fatalf("/missing should be 404, got %+v", got)
	}

	// No URL should be crawled more than once (visited set dedupes).
	if int(hits) != 5 {
		t.Fatalf("server received %d hits, want 5 (no duplicate crawls)", hits)
	}
}

func TestCrawlHighConcurrency(t *testing.T) {
	var hits int32
	srv := testServer(&hits)
	defer srv.Close()

	snap, err := Crawl(Config{BaseURL: srv.URL, Workers: 200, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("crawl: %v", err)
	}
	if len(snap.Pages) != 5 {
		t.Fatalf("got %d pages, want 5 under high concurrency", len(snap.Pages))
	}
}

func urls(s *snapshot.Snapshot) []string {
	out := make([]string, 0, len(s.Pages))
	for _, p := range s.Pages {
		out = append(out, p.URL)
	}
	return out
}

func index(s *snapshot.Snapshot) map[string]*snapshot.Page {
	m := make(map[string]*snapshot.Page, len(s.Pages))
	for i := range s.Pages {
		m[s.Pages[i].URL] = &s.Pages[i]
	}
	return m
}
