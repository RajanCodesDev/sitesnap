package validation

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/RajanCodesDev/sitesnap/internal/dupdetect"
	"github.com/RajanCodesDev/sitesnap/internal/snapshot"
)

func vpage(url, parent string, status int, ct string) snapshot.Page {
	return snapshot.Page{URL: url, ParentURL: parent, StatusCode: status, ContentType: ct}
}

func hasKindSev(rep *Report, kind string, sev Severity) bool {
	for _, f := range rep.Findings {
		if f.Kind == kind && f.Severity == sev {
			return true
		}
	}
	return false
}

// TestBuildMergesSnapshotAndDuplicateFindings verifies structural errors and
// duplicate warnings are combined into one report.
func TestBuildMergesSnapshotAndDuplicateFindings(t *testing.T) {
	s := &snapshot.Snapshot{BaseURL: "https://e.com", Pages: []snapshot.Page{
		vpage("https://e.com/about", "https://e.com/", 200, "text/html"),
		vpage("https://e.com/about/", "https://e.com/services", 200, "text/html"),
		vpage("https://e.com/bad", "", 0, ""), // missing status + content type (errors)
	}}
	dups := dupdetect.Detect(s)
	rep := Build(s, dups)

	if !rep.HasErrors() {
		t.Fatalf("expected errors from missing status/content-type, got %+v", rep.Findings)
	}
	// The trailing-slash duplicate must be a warning, not an error.
	foundWarn := false
	for _, f := range rep.Warnings() {
		if f.Kind == "duplicate_url" {
			foundWarn = true
		}
	}
	if !foundWarn {
		t.Fatalf("expected a duplicate_url warning, got %+v", rep.Warnings())
	}
}

// TestBuildExactDuplicateIsError verifies an exact duplicate URL is reported as
// an error (SiteSnap fault), not a warning.
func TestBuildExactDuplicateIsError(t *testing.T) {
	s := &snapshot.Snapshot{BaseURL: "https://e.com", Pages: []snapshot.Page{
		vpage("https://e.com/a", "https://e.com/", 200, "text/html"),
		vpage("https://e.com/a", "https://e.com/b", 200, "text/html"),
	}}
	dups := dupdetect.Detect(s)
	rep := Build(s, dups)
	if !rep.HasErrors() {
		t.Fatalf("exact duplicate should be an error, got %+v", rep.Findings)
	}
}

// TestPromoteWarningsStrictMode verifies --strict promotes warnings to errors.
func TestPromoteWarningsStrictMode(t *testing.T) {
	s := &snapshot.Snapshot{BaseURL: "https://e.com", Pages: []snapshot.Page{
		vpage("https://e.com/about", "https://e.com/", 200, "text/html"),
		vpage("https://e.com/about/", "https://e.com/services", 200, "text/html"),
	}}
	dups := dupdetect.Detect(s)
	rep := Build(s, dups)
	if rep.HasErrors() {
		t.Fatalf("without strict, warnings must not be errors, got %+v", rep.Errors())
	}
	rep.PromoteWarnings()
	if !rep.HasErrors() {
		t.Fatalf("after PromoteWarnings, report must have errors, got %+v", rep.Findings)
	}
}

// TestPrintGroupedBySeverity verifies the report groups errors and warnings.
func TestPrintGroupedBySeverity(t *testing.T) {
	s := &snapshot.Snapshot{BaseURL: "https://e.com", Pages: []snapshot.Page{
		vpage("https://e.com/", "", 200, "text/html"),
		vpage("https://e.com/bad", "", 0, ""),
		vpage("https://e.com/about", "https://e.com/", 200, "text/html"),
		vpage("https://e.com/about/", "https://e.com/services", 200, "text/html"),
		vpage("https://e.com/c", "https://e.com/", 200, "text/html"),
	}}
	dups := dupdetect.Detect(s)
	rep := Build(s, dups)

	var buf bytes.Buffer
	rep.Print(&buf)
	out := buf.String()
	for _, want := range []string{
		"Snapshot Validation",
		"Errors (2)",
		"Warnings (1)",
		"duplicate_url",
		"✗ Snapshot validation failed.",
	} {
		if !bytes.Contains(buf.Bytes(), []byte(want)) {
			t.Errorf("report missing %q\n---\n%s", want, out)
		}
	}
}

// TestCleanSnapshotNoFindings verifies a clean snapshot yields no findings.
func TestCleanSnapshotNoFindings(t *testing.T) {
	s := &snapshot.Snapshot{BaseURL: "https://e.com", Pages: []snapshot.Page{
		vpage("https://e.com/", "", 200, "text/html"),
		vpage("https://e.com/a", "https://e.com/", 200, "text/html"),
		vpage("https://e.com/b", "https://e.com/", 200, "text/html"),
	}}
	dups := dupdetect.Detect(s)
	rep := Build(s, dups)
	if len(rep.Findings) != 0 {
		t.Fatalf("expected no findings, got %+v", rep.Findings)
	}
	if rep.HasErrors() {
		t.Fatalf("clean snapshot must not have errors, got %+v", rep.Errors())
	}
}

// TestCrawlSanityBaseURLMissing warns when the base URL is absent from pages.
func TestCrawlSanityBaseURLMissing(t *testing.T) {
	s := &snapshot.Snapshot{BaseURL: "https://e.com", Pages: []snapshot.Page{
		vpage("https://e.com/a", "", 200, "text/html"),
		vpage("https://e.com/b", "", 200, "text/html"),
	}}
	rep := Build(s, dupdetect.Detect(s))
	if !hasKindSev(rep, "base_url_missing", SeverityWarning) {
		t.Fatalf("expected base_url_missing warning, got %+v", rep.Findings)
	}
	if rep.HasErrors() {
		t.Fatalf("sanity warnings must not be errors, got %+v", rep.Errors())
	}
}

// TestCrawlSanityBaseURLNon200 warns when the base URL returns non-200.
func TestCrawlSanityBaseURLNon200(t *testing.T) {
	s := &snapshot.Snapshot{BaseURL: "https://e.com", Pages: []snapshot.Page{
		vpage("https://e.com/", "", 500, "text/html"),
		vpage("https://e.com/a", "https://e.com/", 200, "text/html"),
	}}
	rep := Build(s, dupdetect.Detect(s))
	if !hasKindSev(rep, "base_url_status", SeverityWarning) {
		t.Fatalf("expected base_url_status warning, got %+v", rep.Findings)
	}
}

// TestCrawlSanityLowPageCount warns when the crawl is suspiciously small.
func TestCrawlSanityLowPageCount(t *testing.T) {
	s := &snapshot.Snapshot{BaseURL: "https://e.com", Pages: []snapshot.Page{
		vpage("https://e.com/", "", 200, "text/html"),
	}}
	rep := Build(s, dupdetect.Detect(s))
	if !hasKindSev(rep, "low_page_count", SeverityWarning) {
		t.Fatalf("expected low_page_count warning, got %+v", rep.Findings)
	}
}

// TestCrawlSanityExcessiveUnreachable warns when most pages are unreachable.
func TestCrawlSanityExcessiveUnreachable(t *testing.T) {
	pages := []snapshot.Page{vpage("https://e.com/", "", 200, "text/html")}
	for i := 0; i < 5; i++ {
		pages = append(pages, vpage(fmt.Sprintf("https://e.com/x%d", i), "https://e.com/", 0, ""))
	}
	s := &snapshot.Snapshot{BaseURL: "https://e.com", Pages: pages}
	rep := Build(s, dupdetect.Detect(s))
	if !hasKindSev(rep, "excessive_unreachable", SeverityWarning) {
		t.Fatalf("expected excessive_unreachable warning, got %+v", rep.Findings)
	}
}

// TestCrawlSanityPageCountDrop warns when current crawl is far smaller than
// the previous snapshot.
func TestCrawlSanityPageCountDrop(t *testing.T) {
	s := &snapshot.Snapshot{BaseURL: "https://e.com", Pages: []snapshot.Page{
		vpage("https://e.com/", "", 200, "text/html"),
		vpage("https://e.com/a", "https://e.com/", 200, "text/html"),
	}}
	dups := dupdetect.Detect(s)
	dups.PreviousCount = 121 // previous had 121 pages, current has 2
	rep := Build(s, dups)
	if !hasKindSev(rep, "page_count_drop", SeverityWarning) {
		t.Fatalf("expected page_count_drop warning, got %+v", rep.Findings)
	}
}

// TestCrawlSanityNoDropWhenSimilar verifies no page_count_drop warning when
// the current crawl is comparable to the previous one.
func TestCrawlSanityNoDropWhenSimilar(t *testing.T) {
	s := &snapshot.Snapshot{BaseURL: "https://e.com", Pages: []snapshot.Page{
		vpage("https://e.com/", "", 200, "text/html"),
		vpage("https://e.com/a", "https://e.com/", 200, "text/html"),
		vpage("https://e.com/b", "https://e.com/", 200, "text/html"),
	}}
	dups := dupdetect.Detect(s)
	dups.PreviousCount = 3
	rep := Build(s, dups)
	if hasKindSev(rep, "page_count_drop", SeverityWarning) {
		t.Fatalf("did not expect page_count_drop warning, got %+v", rep.Findings)
	}
}
