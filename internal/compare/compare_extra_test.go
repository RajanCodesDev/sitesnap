package compare

import (
	"testing"

	"github.com/RajanCodesDev/sitesnap/internal/snapshot"
)

// TestCompareNoChanges verifies that an identical re-crawl yields an empty
// report — the core "deployment succeeded" signal.
func TestCompareNoChanges(t *testing.T) {
	prev := snap(
		snapshot.Page{URL: "https://e.com/a", StatusCode: 200, ContentType: "text/html"},
		snapshot.Page{URL: "https://e.com/b", StatusCode: 200, ContentType: "text/css"},
	)
	curr := snap(
		snapshot.Page{URL: "https://e.com/a", StatusCode: 200, ContentType: "text/html"},
		snapshot.Page{URL: "https://e.com/b", StatusCode: 200, ContentType: "text/css"},
	)
	r := Compare(prev, curr)
	if len(r.Added) != 0 || len(r.Removed) != 0 || len(r.StatusChanges) != 0 || len(r.ContentTypeChanges) != 0 {
		t.Fatalf("expected empty report, got %+v", r)
	}
}

// TestCompareMixedChanges exercises every change category in a single run and
// asserts each is reported exactly once with correct previous/current values.
func TestCompareMixedChanges(t *testing.T) {
	prev := snap(
		snapshot.Page{URL: "https://e.com/keep", StatusCode: 200, ContentType: "text/html"},
		snapshot.Page{URL: "https://e.com/status", StatusCode: 200, ContentType: "text/html"},
		snapshot.Page{URL: "https://e.com/type", StatusCode: 200, ContentType: "text/html"},
		snapshot.Page{URL: "https://e.com/gone", StatusCode: 200, ContentType: "text/html"},
		snapshot.Page{URL: "https://e.com/both", StatusCode: 200, ContentType: "text/html"},
	)
	curr := snap(
		snapshot.Page{URL: "https://e.com/keep", StatusCode: 200, ContentType: "text/html"},
		snapshot.Page{URL: "https://e.com/status", StatusCode: 500, ContentType: "text/html"},
		snapshot.Page{URL: "https://e.com/type", StatusCode: 200, ContentType: "application/json"},
		snapshot.Page{URL: "https://e.com/new", StatusCode: 200, ContentType: "text/html"},
		snapshot.Page{URL: "https://e.com/both", StatusCode: 503, ContentType: "text/plain"},
	)

	r := Compare(prev, curr)

	if len(r.Added) != 1 || r.Added[0] != "https://e.com/new" {
		t.Errorf("added = %v", r.Added)
	}
	if len(r.Removed) != 1 || r.Removed[0] != "https://e.com/gone" {
		t.Errorf("removed = %v", r.Removed)
	}
	if len(r.StatusChanges) != 2 {
		t.Fatalf("status changes = %+v", r.StatusChanges)
	}
	for _, c := range r.StatusChanges {
		switch c.URL {
		case "https://e.com/status":
			if c.Previous != "200" || c.Current != "500" {
				t.Errorf("status %q: %+v", c.URL, c)
			}
		case "https://e.com/both":
			if c.Previous != "200" || c.Current != "503" {
				t.Errorf("status %q: %+v", c.URL, c)
			}
		default:
			t.Errorf("unexpected status change %q", c.URL)
		}
	}
	if len(r.ContentTypeChanges) != 2 {
		t.Fatalf("content-type changes = %+v", r.ContentTypeChanges)
	}
	for _, c := range r.ContentTypeChanges {
		switch c.URL {
		case "https://e.com/type":
			if c.Previous != "text/html" || c.Current != "application/json" {
				t.Errorf("type %q: %+v", c.URL, c)
			}
		case "https://e.com/both":
			if c.Previous != "text/html" || c.Current != "text/plain" {
				t.Errorf("type %q: %+v", c.URL, c)
			}
		default:
			t.Errorf("unexpected content-type change %q", c.URL)
		}
	}
}
