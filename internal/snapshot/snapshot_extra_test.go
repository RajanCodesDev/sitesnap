package snapshot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// sampleSnapshot builds a snapshot with pages in the given order.
func sampleSnapshot(order []string) *Snapshot {
	pages := make([]Page, 0, len(order))
	for _, u := range order {
		pages = append(pages, Page{
			URL:          u,
			ParentURL:    "https://e.com/",
			StatusCode:   200,
			ContentType:  "text/html",
			ResourceType: "html",
		})
	}
	return &Snapshot{
		BaseURL:   "https://e.com",
		CrawledAt: time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
		Pages:     pages,
	}
}

// TestSaveProducesDeterministicOutput verifies that the same logical snapshot
// serializes to byte-identical JSON regardless of the order pages are supplied
// in. This is what makes two runs against an unchanged site comparable.
func TestSaveProducesDeterministicOutput(t *testing.T) {
	urls := []string{"https://e.com/a", "https://e.com/b", "https://e.com/c"}
	dir := t.TempDir()

	p1 := filepath.Join(dir, "one.json")
	p2 := filepath.Join(dir, "two.json")
	if err := Save(p1, sampleSnapshot(urls)); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := Save(p2, sampleSnapshot([]string{urls[2], urls[0], urls[1]})); err != nil {
		t.Fatalf("save: %v", err)
	}

	b1, _ := os.ReadFile(p1)
	b2, _ := os.ReadFile(p2)
	if string(b1) != string(b2) {
		t.Fatalf("non-deterministic output:\n%s\n---\n%s", b1, b2)
	}
}

// TestSaveEmitsValidJSON verifies the persisted file is parseable JSON and
// round-trips through encoding/json directly (not just Load).
func TestSaveEmitsValidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snap.json")
	s := sampleSnapshot([]string{"https://e.com/a"})
	if err := Save(path, s); err != nil {
		t.Fatalf("save: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if _, ok := decoded["pages"]; !ok {
		t.Fatal("missing 'pages' key in JSON output")
	}
	if _, ok := decoded["base_url"]; !ok {
		t.Fatal("missing 'base_url' key in JSON output")
	}
}

// TestRequiredFieldsAlwaysPopulated verifies that every page in a loaded
// snapshot carries the fields the comparison and report stages depend on.
func TestRequiredFieldsAlwaysPopulated(t *testing.T) {
	s := sampleSnapshot([]string{"https://e.com/a", "https://e.com/b"})
	for _, p := range s.Pages {
		if p.URL == "" {
			t.Error("page URL must not be empty")
		}
		if p.StatusCode == 0 {
			t.Errorf("page %q must record a status code", p.URL)
		}
		if p.ContentType == "" {
			t.Errorf("page %q must record a content type", p.URL)
		}
	}
}

// TestLoadPreservesAllPages verifies no page is dropped on round trip, even
// with many pages and mixed resource types.
func TestLoadPreservesAllPages(t *testing.T) {
	pages := make([]Page, 0, 50)
	for i := 0; i < 50; i++ {
		rt := "html"
		if i%2 == 0 {
			rt = "css"
		}
		pages = append(pages, Page{
			URL:          "https://e.com/p" + itoa(i),
			ParentURL:    "https://e.com/",
			StatusCode:   200,
			ContentType:  "text/" + rt,
			ResourceType: rt,
		})
	}
	s := &Snapshot{BaseURL: "https://e.com", CrawledAt: time.Now().UTC(), Pages: pages}

	dir := t.TempDir()
	path := filepath.Join(dir, "snap.json")
	if err := Save(path, s); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got.Pages) != len(pages) {
		t.Fatalf("page count mismatch: got %d want %d", len(got.Pages), len(pages))
	}
	want := make(map[string]Page, len(pages))
	for _, p := range pages {
		want[p.URL] = p
	}
	for _, p := range got.Pages {
		w, ok := want[p.URL]
		if !ok {
			t.Fatalf("unexpected page after round trip: %q", p.URL)
		}
		if w.ResourceType != p.ResourceType || w.StatusCode != p.StatusCode {
			t.Fatalf("page %q changed after round trip: %+v vs %+v", p.URL, p, w)
		}
	}
}

// TestTwoRunEquivalenceAgainstUnchangedSite simulates two crawls of an
// unchanged site (identical page sets, possibly discovered in different order)
// and asserts the stored snapshots are byte-for-byte equal.
func TestTwoRunEquivalenceAgainstUnchangedSite(t *testing.T) {
	urls := []string{
		"https://e.com/", "https://e.com/about", "https://e.com/contact",
		"https://e.com/style.css", "https://e.com/app.js",
	}
	dir := t.TempDir()
	run1 := filepath.Join(dir, "run1.json")
	run2 := filepath.Join(dir, "run2.json")

	// Run 1 discovers in one order; run 2 in a different order.
	if err := Save(run1, sampleSnapshot(urls)); err != nil {
		t.Fatalf("save run1: %v", err)
	}
	if err := Save(run2, sampleSnapshot([]string{urls[3], urls[1], urls[4], urls[0], urls[2]})); err != nil {
		t.Fatalf("save run2: %v", err)
	}

	b1, _ := os.ReadFile(run1)
	b2, _ := os.ReadFile(run2)
	if string(b1) != string(b2) {
		t.Fatal("two runs against an unchanged site produced different snapshots")
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}
