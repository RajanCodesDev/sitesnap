package crawler

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/RajanCodesDev/sitesnap/internal/snapshot"
)

// --- Unit tests for pure logic (no network) ---

func TestResourceType(t *testing.T) {
	cases := []struct {
		ct   string
		want string
	}{
		{"text/html", "html"},
		{"text/html; charset=utf-8", "html"},
		{"TEXT/HTML", "html"},
		{"text/css", "css"},
		{"application/javascript", "script"},
		{"application/json", "script"},
		{"image/png", "image"},
		{"image/svg+xml", "image"},
		{"font/woff2", "font"},
		{"text/plain", "text"},
		{"application/pdf", "other"},
		{"", ""},
	}
	for _, c := range cases {
		if got := resourceType(c.ct); got != c.want {
			t.Errorf("resourceType(%q) = %q, want %q", c.ct, got, c.want)
		}
	}
}

func TestIsHTML(t *testing.T) {
	if !isHTML("text/html") || !isHTML("text/html; charset=utf-8") {
		t.Error("isHTML should accept text/html variants")
	}
	if isHTML("text/css") || isHTML("application/json") || isHTML("") {
		t.Error("isHTML should reject non-html content types")
	}
}

func TestCanonicalize(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"https://e.com", "https://e.com/"},
		{"https://e.com/", "https://e.com/"},
		{"https://e.com/a#frag", "https://e.com/a"},
		{"https://e.com/a?x=1#frag", "https://e.com/a?x=1"},
	}
	for _, c := range cases {
		if got := canonicalize(c.in); got != c.want {
			t.Errorf("canonicalize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// --- Integration tests using httptest ---

// newServer builds an httptest server from a route map. Each handler sets the
// given content type and writes the body. A 404 route is supported by mapping
// the path to a status via notFound.
func newServer(t *testing.T, routes map[string]string, cts map[string]string, notFound map[string]bool) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	for path, body := range routes {
		path, body := path, body
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			ct := cts[path]
			if ct == "" {
				ct = "text/html"
			}
			if notFound[path] {
				w.WriteHeader(http.StatusNotFound)
			}
			w.Header().Set("Content-Type", ct)
			w.Write([]byte(body))
		})
	}
	return httptest.NewServer(mux)
}

func crawl(t *testing.T, baseURL string, workers int) *snapshot.Snapshot {
	t.Helper()
	snap, err := Crawl(Config{BaseURL: baseURL, Workers: workers, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("crawl: %v", err)
	}
	return snap
}

func TestCrawlSinglePage(t *testing.T) {
	srv := newServer(t, map[string]string{"/": "no links here"}, nil, nil)
	defer srv.Close()

	snap := crawl(t, srv.URL, 5)
	if len(snap.Pages) != 1 {
		t.Fatalf("got %d pages, want 1: %v", len(snap.Pages), urls(snap))
	}
	if snap.Pages[0].StatusCode != 200 {
		t.Errorf("status = %d, want 200", snap.Pages[0].StatusCode)
	}
	if snap.Pages[0].ContentType != "text/html" {
		t.Errorf("content-type = %q, want text/html", snap.Pages[0].ContentType)
	}
	if snap.Pages[0].ParentURL != "" {
		t.Errorf("parent = %q, want empty for root", snap.Pages[0].ParentURL)
	}
}

func TestCrawlCircularLinks(t *testing.T) {
	routes := map[string]string{
		"/":   `<a href="/a">A</a>`,
		"/a":  `<a href="/b">B</a><a href="/a2">A2</a>`, // link the second cycle in
		"/b":  `<a href="/">Home</a>`,                   // points back to root
		"/a2": `<a href="/b2">B2</a>`,
		"/b2": `<a href="/a2">A2</a>`, // separate cycle
	}
	srv := newServer(t, routes, nil, nil)
	defer srv.Close()

	snap := crawl(t, srv.URL, 10)
	if len(snap.Pages) != 5 {
		t.Fatalf("got %d pages, want 5 (cycles must not loop forever): %v", len(snap.Pages), urls(snap))
	}
}

func TestCrawlDuplicateLinks(t *testing.T) {
	var hits int32
	mux := http.NewServeMux()
	pages := map[string]string{
		"/":  `<a href="/a">A</a><a href="/a">A2</a><a href="/b">B</a>`,
		"/a": `<a href="/">Home</a>`,
		"/b": `<a href="/a">A</a>`,
	}
	for p, body := range pages {
		p, body := p, body
		mux.HandleFunc(p, func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&hits, 1)
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(body))
		})
	}
	srv := httptest.NewServer(mux)
	defer srv.Close()

	snap := crawl(t, srv.URL, 10)
	if len(snap.Pages) != 3 {
		t.Fatalf("got %d pages, want 3: %v", len(snap.Pages), urls(snap))
	}
	if int(atomic.LoadInt32(&hits)) != 3 {
		t.Fatalf("server received %d hits, want 3 (no duplicate crawls)", hits)
	}
}

func TestCrawlBrokenLink(t *testing.T) {
	routes := map[string]string{
		"/":        `<a href="/missing">M</a>`,
		"/missing": `gone`,
	}
	srv := newServer(t, routes, nil, map[string]bool{"/missing": true})
	defer srv.Close()

	snap := crawl(t, srv.URL, 5)
	by := index(snap)
	if got := by[srv.URL+"/missing"]; got == nil || got.StatusCode != 404 {
		t.Fatalf("/missing should be 404, got %+v", got)
	}
}

func TestCrawlRedirect(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<a href="/old">Old</a><a href="/new">New</a>`))
	})
	mux.HandleFunc("/old", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/new", http.StatusFound)
	})
	mux.HandleFunc("/new", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<a href="/">Home</a>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	snap := crawl(t, srv.URL, 5)
	by := index(snap)
	// /new is linked directly, so it is crawled as its own URL and is 200.
	if got := by[srv.URL+"/new"]; got == nil || got.StatusCode != 200 {
		t.Fatalf("/new should be 200, got %+v", got)
	}
	// /old redirects to /new; the crawler follows redirects, so /old is
	// recorded with the final status 200 (the body of /new).
	if got := by[srv.URL+"/old"]; got == nil || got.StatusCode != 200 {
		t.Fatalf("/old should be 200 after following redirect, got %+v", got)
	}
}

func TestCrawlQueryParameters(t *testing.T) {
	routes := map[string]string{
		"/":  `<a href="/p?x=1">P1</a><a href="/p?x=2">P2</a>`,
		"/p": `page`,
	}
	srv := newServer(t, routes, nil, nil)
	defer srv.Close()

	snap := crawl(t, srv.URL, 5)
	// /p?x=1 and /p?x=2 are distinct URLs.
	if len(snap.Pages) != 3 {
		t.Fatalf("got %d pages, want 3 (query params are distinct): %v", len(snap.Pages), urls(snap))
	}
}

func TestCrawlFragmentsIgnored(t *testing.T) {
	routes := map[string]string{
		"/":  `<a href="/a#section">A</a><a href="/a">A2</a>`,
		"/a": `page`,
	}
	srv := newServer(t, routes, nil, nil)
	defer srv.Close()

	snap := crawl(t, srv.URL, 5)
	if len(snap.Pages) != 2 {
		t.Fatalf("got %d pages, want 2 (fragment-only links collapse to /a): %v", len(snap.Pages), urls(snap))
	}
}

func TestCrawlExternalLinksIgnored(t *testing.T) {
	routes := map[string]string{
		"/":  `<a href="https://other.example.com/x">Ext</a><a href="/a">A</a>`,
		"/a": `page`,
	}
	srv := newServer(t, routes, nil, nil)
	defer srv.Close()

	snap := crawl(t, srv.URL, 5)
	if len(snap.Pages) != 2 {
		t.Fatalf("got %d pages, want 2 (external ignored): %v", len(snap.Pages), urls(snap))
	}
}

func TestCrawlMixedContentTypes(t *testing.T) {
	routes := map[string]string{
		"/":          `<a href="/page">P</a><a href="/style.css">C</a><a href="/app.js">J</a><a href="/img.png">I</a><a href="/doc.pdf">D</a>`,
		"/page":      `html`,
		"/style.css": "C",
		"/app.js":    "J",
		"/img.png":   "I",
		"/doc.pdf":   "D",
	}
	cts := map[string]string{
		"/style.css": "text/css",
		"/app.js":    "application/javascript",
		"/img.png":   "image/png",
		"/doc.pdf":   "application/pdf",
	}
	srv := newServer(t, routes, cts, nil)
	defer srv.Close()

	snap := crawl(t, srv.URL, 5)
	by := index(snap)
	checks := map[string]string{
		srv.URL + "/page":      "text/html",
		srv.URL + "/style.css": "text/css",
		srv.URL + "/app.js":    "application/javascript",
		srv.URL + "/img.png":   "image/png",
		srv.URL + "/doc.pdf":   "application/pdf",
	}
	for u, wantCT := range checks {
		got := by[u]
		if got == nil {
			t.Errorf("missing %s", u)
			continue
		}
		if got.ContentType != wantCT {
			t.Errorf("%s content-type = %q, want %q", u, got.ContentType, wantCT)
		}
		if got.ResourceType == "" && wantCT != "" {
			t.Errorf("%s resource-type empty, want populated", u)
		}
	}
}

func TestCrawlParentURLRecorded(t *testing.T) {
	routes := map[string]string{
		"/":      `<a href="/child">C</a>`,
		"/child": `page`,
	}
	srv := newServer(t, routes, nil, nil)
	defer srv.Close()

	snap := crawl(t, srv.URL, 5)
	by := index(snap)
	child := by[srv.URL+"/child"]
	if child == nil {
		t.Fatal("missing /child")
	}
	if child.ParentURL != srv.URL+"/" {
		t.Errorf("parent = %q, want %q", child.ParentURL, srv.URL+"/")
	}
}

func TestCrawlInvalidBaseURL(t *testing.T) {
	if _, err := Crawl(Config{BaseURL: "://bad"}); err == nil {
		t.Fatal("expected error for invalid base url")
	}
	if _, err := Crawl(Config{BaseURL: "ftp://example.com"}); err == nil {
		t.Fatal("expected error for non-http(s) scheme")
	}
}
