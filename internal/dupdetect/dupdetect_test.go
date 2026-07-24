package dupdetect

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/RajanCodesDev/sitesnap/internal/crawler"
	"github.com/RajanCodesDev/sitesnap/internal/snapshot"
)

func page(url, parent string, status int, ct string) snapshot.Page {
	return snapshot.Page{URL: url, ParentURL: parent, StatusCode: status, ContentType: ct}
}

// TestDetectExactDuplicates: the same URL recorded twice (e.g. two internal
// links pointing at the identical href) must form one group and be an ERROR.
func TestDetectExactDuplicates(t *testing.T) {
	s := &snapshot.Snapshot{Pages: []snapshot.Page{
		page("https://e.com/a", "https://e.com/", 200, "text/html"),
		page("https://e.com/a", "https://e.com/b", 200, "text/html"),
	}}
	r := Detect(s)
	if len(r.Groups) != 1 {
		t.Fatalf("want 1 group, got %d", len(r.Groups))
	}
	g := r.Groups[0]
	if g.Canonical != "https://e.com/a" {
		t.Errorf("canonical = %q", g.Canonical)
	}
	if g.Severity != SeverityError {
		t.Errorf("exact duplicate should be ERROR, got %s", g.Severity)
	}
	if g.Reason != "Exact duplicate URL entries" {
		t.Errorf("reason = %q", g.Reason)
	}
	if len(g.Variants) != 2 {
		t.Fatalf("want 2 variants, got %d", len(g.Variants))
	}
	// Both parents must be present so the origin of the dup is traceable.
	parents := map[string]bool{}
	for _, v := range g.Variants {
		parents[v.ParentURL] = true
	}
	if !parents["https://e.com/"] || !parents["https://e.com/b"] {
		t.Errorf("expected both parents, got %v", parents)
	}
}

// TestDetectTrailingSlash: /about and /about/ are the same resource and are a
// WARNING (website URL-hygiene issue, not a SiteSnap fault).
func TestDetectTrailingSlash(t *testing.T) {
	s := &snapshot.Snapshot{Pages: []snapshot.Page{
		page("https://e.com/about", "https://e.com/", 200, "text/html"),
		page("https://e.com/about/", "https://e.com/services", 200, "text/html"),
	}}
	r := Detect(s)
	if len(r.Groups) != 1 {
		t.Fatalf("want 1 group, got %d", len(r.Groups))
	}
	g := r.Groups[0]
	if g.Canonical != "https://e.com/about" {
		t.Errorf("canonical = %q, want https://e.com/about", g.Canonical)
	}
	if g.Severity != SeverityWarning {
		t.Errorf("trailing-slash duplicate should be WARNING, got %s", g.Severity)
	}
	if g.Reason != "Trailing slash variant" {
		t.Errorf("reason = %q", g.Reason)
	}
	if len(g.Variants) != 2 {
		t.Fatalf("want 2 variants, got %d", len(g.Variants))
	}
}

// TestDetectFragmentIgnored: /blog#top and /blog are the same resource.
func TestDetectFragmentIgnored(t *testing.T) {
	s := &snapshot.Snapshot{Pages: []snapshot.Page{
		page("https://e.com/blog", "https://e.com/", 200, "text/html"),
		page("https://e.com/blog#top", "https://e.com/", 200, "text/html"),
	}}
	r := Detect(s)
	if len(r.Groups) != 1 {
		t.Fatalf("want 1 group, got %d", len(r.Groups))
	}
}

// TestDetectMultiplePagesLinkingSameURL: three pages link to /x; one group.
func TestDetectMultiplePagesLinkingSameURL(t *testing.T) {
	s := &snapshot.Snapshot{Pages: []snapshot.Page{
		page("https://e.com/x", "https://e.com/p1", 200, "text/html"),
		page("https://e.com/x", "https://e.com/p2", 200, "text/html"),
		page("https://e.com/x", "https://e.com/p3", 200, "text/html"),
		page("https://e.com/y", "https://e.com/", 200, "text/html"),
	}}
	r := Detect(s)
	if len(r.Groups) != 1 {
		t.Fatalf("want 1 group, got %d", len(r.Groups))
	}
	if len(r.Groups[0].Variants) != 3 {
		t.Fatalf("want 3 variants, got %d", len(r.Groups[0].Variants))
	}
}

// TestDetectNoFalsePositives: distinct resources stay separate.
func TestDetectNoFalsePositives(t *testing.T) {
	s := &snapshot.Snapshot{Pages: []snapshot.Page{
		page("https://e.com/a", "https://e.com/", 200, "text/html"),
		page("https://e.com/b", "https://e.com/", 200, "text/html"),
		page("https://e.com/a?x=1", "https://e.com/", 200, "text/html"),
	}}
	r := Detect(s)
	if len(r.Groups) != 0 {
		t.Fatalf("want 0 groups, got %d", len(r.Groups))
	}
}

// TestDetectUnderConcurrency: crawl a live server that links the same page
// with and without a trailing slash from multiple parents; duplicates must be
// detected regardless of worker count.
func TestDetectUnderConcurrency(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<a href="/about">A</a><a href="/about/">B</a>`))
	})
	mux.HandleFunc("/about", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<a href="/">Home</a>`))
	})
	mux.HandleFunc("/about/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<a href="/">Home</a>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	for _, workers := range []int{1, 5, 50, 200} {
		snap, err := crawler.Crawl(crawler.Config{BaseURL: srv.URL, Workers: workers, Timeout: 2 * time.Second})
		if err != nil {
			t.Fatalf("workers=%d crawl: %v", workers, err)
		}
		r := Detect(snap)
		if len(r.Groups) != 1 {
			t.Fatalf("workers=%d: want 1 duplicate group, got %d", workers, len(r.Groups))
		}
		if len(r.Groups[0].Variants) < 2 {
			t.Fatalf("workers=%d: want >=2 variants, got %d", workers, len(r.Groups[0].Variants))
		}
	}
}

// TestReportSummary verifies the group/URL counts reported.
func TestReportSummary(t *testing.T) {
	s := &snapshot.Snapshot{Pages: []snapshot.Page{
		page("https://e.com/about", "https://e.com/", 200, "text/html"),
		page("https://e.com/about/", "https://e.com/s", 200, "text/html"),
		page("https://e.com/blog", "https://e.com/", 200, "text/html"),
		page("https://e.com/blog/", "https://e.com/a", 200, "text/html"),
	}}
	r := Detect(s)
	groups, urls := r.Summary()
	if groups != 2 {
		t.Errorf("groups = %d, want 2", groups)
	}
	if urls != 4 {
		t.Errorf("urls = %d, want 4", urls)
	}
}

// TestPrintFormat verifies the report renders the documented structure.
func TestPrintFormat(t *testing.T) {
	s := &snapshot.Snapshot{Pages: []snapshot.Page{
		page("https://e.com/about", "https://e.com/", 200, "text/html"),
		page("https://e.com/about/", "https://e.com/services", 200, "text/html"),
	}}
	var buf bytes.Buffer
	Print(&buf, Detect(s))
	out := buf.String()
	for _, want := range []string{
		"Duplicate URL Report",
		"Warnings (1)",
		"[warning] https://e.com/about",
		"Canonical:",
		"https://e.com/about",
		"Reason:",
		"Trailing slash variant",
		"Variants:",
		"https://e.com/about/",
		"Parent: /services",
		"Status: 200",
		"Warnings: 1 (groups), 2 (urls)",
	} {
		if !bytes.Contains(buf.Bytes(), []byte(want)) {
			t.Errorf("report missing %q\n---\n%s", want, out)
		}
	}
}
