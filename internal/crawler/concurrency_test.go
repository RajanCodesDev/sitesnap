package crawler

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"github.com/RajanCodesDev/sitesnap/internal/snapshot"
)

// buildSite returns an httptest server with a moderately linked site so that
// concurrency actually matters: a root linking to N leaf pages, each linking
// back to root and to two siblings.
func buildSite(t *testing.T, leaves int) (*httptest.Server, *int32) {
	t.Helper()
	var hits int32
	mux := http.NewServeMux()
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})

	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "text/html")
		body := ""
		for i := 0; i < leaves; i++ {
			body += `<a href="/p` + itoa(i) + `">P` + itoa(i) + `</a>`
		}
		w.Write([]byte(body))
	})
	for i := 0; i < leaves; i++ {
		i := i
		mux.HandleFunc("/p"+itoa(i), func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&hits, 1)
			w.Header().Set("Content-Type", "text/html")
			body := `<a href="/">Home</a>`
			if i+1 < leaves {
				body += `<a href="/p` + itoa(i+1) + `">Next</a>`
			}
			if i > 0 {
				body += `<a href="/p` + itoa(i-1) + `">Prev</a>`
			}
			w.Write([]byte(body))
		})
	}
	return httptest.NewServer(mux), &hits
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// rel strips the scheme and host from an absolute URL so snapshots from
// different httptest servers (different ports) can be compared by path.
func rel(u string) string {
	for i := 0; i < len(u); i++ {
		if u[i] == ':' && i+2 < len(u) && u[i+1] == '/' && u[i+2] == '/' {
			rest := u[i+3:]
			if slash := indexByte(rest, '/'); slash >= 0 {
				return rest[slash:]
			}
			return "/"
		}
	}
	return u
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

// urlSet returns the sorted list of page paths for deterministic comparison.
func urlSet(s *snapshot.Snapshot) []string {
	out := make([]string, 0, len(s.Pages))
	for _, p := range s.Pages {
		out = append(out, rel(p.URL))
	}
	sort.Strings(out)
	return out
}

// TestCrawlConcurrencyIdentical verifies that the resulting snapshot is
// identical regardless of worker count. Only execution time should differ.
func TestCrawlConcurrencyIdentical(t *testing.T) {
	const leaves = 12
	srv, hits := buildSite(t, leaves)
	defer srv.Close()

	want := urlSet(crawl(t, srv.URL, 1))
	// Total URLs: root + leaves.
	if len(want) != leaves+1 {
		t.Fatalf("unexpected page count baseline: got %d want %d", len(want), leaves+1)
	}

	for _, workers := range []int{1, 5, 10, 50, 100, 200} {
		// Reset hit counter by recreating the server (handlers capture &hits).
		srv2, _ := buildSite(t, leaves)
		_ = hits
		snap := crawl(t, srv2.URL, workers)
		got := urlSet(snap)
		if len(got) != len(want) {
			t.Fatalf("workers=%d: got %d pages, want %d", workers, len(got), len(want))
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("workers=%d: snapshot differs at index %d: %q vs %q", workers, i, got[i], want[i])
			}
		}
		srv2.Close()
	}
}

// TestCrawlNoDuplicateCrawlsUnderConcurrency verifies the visited set prevents
// a URL from being fetched more than once, even with many workers racing.
func TestCrawlNoDuplicateCrawlsUnderConcurrency(t *testing.T) {
	const leaves = 20
	srv, hits := buildSite(t, leaves)
	defer srv.Close()

	// Warm the server once so handlers exist; reset counter via fresh server.
	srv2, hits2 := buildSite(t, leaves)
	defer srv2.Close()
	_ = srv
	_ = hits

	snap := crawl(t, srv2.URL, 200)
	if len(snap.Pages) != leaves+1 {
		t.Fatalf("got %d pages, want %d", len(snap.Pages), leaves+1)
	}
	// Every unique URL must be fetched exactly once.
	if got := atomic.LoadInt32(hits2); int(got) != leaves+1 {
		t.Fatalf("server received %d hits, want %d (no duplicate crawls under concurrency)", got, leaves+1)
	}
}

// TestCrawlTimeoutUnreachable ensures a slow/unreachable page is recorded with
// status 0 rather than hanging the crawl.
func TestCrawlTimeoutUnreachable(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})

	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<a href="/slow">S</a>`))
	})
	// /slow never responds within the timeout.
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	start := time.Now()
	snap, err := Crawl(Config{BaseURL: srv.URL, Workers: 5, Timeout: 200 * time.Millisecond})
	if err != nil {
		t.Fatalf("crawl: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed > 5*time.Second {
		t.Fatalf("crawl took %s, expected to respect the timeout", elapsed)
	}
	by := index(snap)
	if got := by[srv.URL+"/slow"]; got == nil || got.StatusCode != 0 {
		t.Fatalf("/slow should be recorded with status 0 (timeout), got %+v", got)
	}
}
