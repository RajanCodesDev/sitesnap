package compare

import (
	"testing"
	"time"

	"github.com/RajanCodesDev/sitesnap/internal/snapshot"
)

func snap(pages ...snapshot.Page) *snapshot.Snapshot {
	return &snapshot.Snapshot{CrawledAt: time.Now(), Pages: pages}
}

func TestCompare(t *testing.T) {
	prev := snap(
		snapshot.Page{URL: "https://e.com/a", StatusCode: 200, ContentType: "text/html"},
		snapshot.Page{URL: "https://e.com/b", StatusCode: 200, ContentType: "text/html"},
		snapshot.Page{URL: "https://e.com/c", StatusCode: 200, ContentType: "text/html"},
	)
	curr := snap(
		snapshot.Page{URL: "https://e.com/a", StatusCode: 200, ContentType: "text/html"},
		snapshot.Page{URL: "https://e.com/b", StatusCode: 404, ContentType: "text/html"},
		snapshot.Page{URL: "https://e.com/c", StatusCode: 200, ContentType: "application/json"},
		snapshot.Page{URL: "https://e.com/d", StatusCode: 200, ContentType: "text/html"},
	)

	r := Compare(prev, curr)

	if len(r.Added) != 1 || r.Added[0] != "https://e.com/d" {
		t.Errorf("added = %v, want [https://e.com/d]", r.Added)
	}
	if len(r.Removed) != 0 {
		t.Errorf("removed = %v, want empty", r.Removed)
	}
	if len(r.StatusChanges) != 1 || r.StatusChanges[0].URL != "https://e.com/b" || r.StatusChanges[0].Current != "404" {
		t.Errorf("status changes = %+v", r.StatusChanges)
	}
	if len(r.ContentTypeChanges) != 1 || r.ContentTypeChanges[0].URL != "https://e.com/c" || r.ContentTypeChanges[0].Current != "application/json" {
		t.Errorf("content-type changes = %+v", r.ContentTypeChanges)
	}
}

func TestCompareRemoved(t *testing.T) {
	prev := snap(snapshot.Page{URL: "https://e.com/x", StatusCode: 200, ContentType: "text/html"})
	curr := snap()
	r := Compare(prev, curr)
	if len(r.Removed) != 1 || r.Removed[0] != "https://e.com/x" {
		t.Errorf("removed = %v, want [https://e.com/x]", r.Removed)
	}
	if len(r.Added) != 0 {
		t.Errorf("added = %v, want empty", r.Added)
	}
}
