package snapshot

import (
	"bytes"
	"testing"
)

func vpage(url, parent string, status int, ct string) Page {
	return Page{URL: url, ParentURL: parent, StatusCode: status, ContentType: ct}
}

func vpageRT(url, parent string, status int, ct, rt string) Page {
	return Page{URL: url, ParentURL: parent, StatusCode: status, ContentType: ct, ResourceType: rt}
}

func kinds(rep *ValidationReport) map[string]bool {
	m := make(map[string]bool)
	for _, i := range rep.Issues {
		m[i.Kind] = true
	}
	return m
}

func hasKindSev(rep *ValidationReport, kind string, sev Severity) bool {
	for _, i := range rep.Issues {
		if i.Kind == kind && i.Severity == sev {
			return true
		}
	}
	return false
}

// TestValidateCleanSnapshot: a well-formed snapshot has no issues.
func TestValidateCleanSnapshot(t *testing.T) {
	s := &Snapshot{BaseURL: "https://e.com", Pages: []Page{
		vpage("https://e.com/", "", 200, "text/html"),
		vpage("https://e.com/a", "https://e.com/", 200, "text/html"),
	}}
	rep := Validate(s)
	if !rep.Valid() {
		t.Fatalf("expected valid, got %+v", rep.Issues)
	}
}

// TestValidateEmptyURL: a page with an empty URL is an ERROR.
func TestValidateEmptyURL(t *testing.T) {
	s := &Snapshot{Pages: []Page{vpage("", "", 200, "text/html")}}
	rep := Validate(s)
	if !hasKindSev(rep, "empty_url", SeverityError) {
		t.Fatalf("expected empty_url error, got %+v", rep.Issues)
	}
}

// TestValidateInvalidURL: a non-absolute / bad URL is an ERROR.
func TestValidateInvalidURL(t *testing.T) {
	s := &Snapshot{Pages: []Page{
		vpage("not-a-url", "", 200, "text/html"),
		vpage("ftp://e.com/x", "", 200, "text/html"),
	}}
	rep := Validate(s)
	if !hasKindSev(rep, "invalid_url", SeverityError) {
		t.Fatalf("expected invalid_url error, got %+v", rep.Issues)
	}
}

// TestValidateMissingStatus: a page with StatusCode 0 is an ERROR.
func TestValidateMissingStatus(t *testing.T) {
	s := &Snapshot{Pages: []Page{vpage("https://e.com/a", "", 0, "text/html")}}
	rep := Validate(s)
	if !hasKindSev(rep, "missing_status", SeverityError) {
		t.Fatalf("expected missing_status error, got %+v", rep.Issues)
	}
}

// TestValidateMissingContentType: a page with empty content type is an ERROR.
func TestValidateMissingContentType(t *testing.T) {
	s := &Snapshot{Pages: []Page{vpage("https://e.com/a", "", 200, "")}}
	rep := Validate(s)
	if !hasKindSev(rep, "missing_content_type", SeverityError) {
		t.Fatalf("expected missing_content_type error, got %+v", rep.Issues)
	}
}

// TestValidateInvalidResourceType: an unknown resource type is an ERROR.
func TestValidateInvalidResourceType(t *testing.T) {
	s := &Snapshot{Pages: []Page{vpageRT("https://e.com/a", "", 200, "text/html", "bogus")}}
	rep := Validate(s)
	if !hasKindSev(rep, "invalid_resource_type", SeverityError) {
		t.Fatalf("expected invalid_resource_type error, got %+v", rep.Issues)
	}
}

// TestValidateTemplatePlaceholder: a URL with a template placeholder is a
// WARNING (website quality issue, must not fail the crawl).
func TestValidateTemplatePlaceholder(t *testing.T) {
	s := &Snapshot{BaseURL: "https://e.com", Pages: []Page{vpage("https://e.com/{item.link}", "https://e.com/", 200, "text/html")}}
	rep := Validate(s)
	if !hasKindSev(rep, "template_placeholder", SeverityWarning) {
		t.Fatalf("expected template_placeholder warning, got %+v", rep.Issues)
	}
	if rep.HasErrors() {
		t.Fatalf("template placeholder must not be an error, got %+v", rep.Errors())
	}
}

// TestValidateEmptySnapshot: a snapshot with no pages is an ERROR.
func TestValidateEmptySnapshot(t *testing.T) {
	rep := Validate(&Snapshot{})
	if !hasKindSev(rep, "empty_snapshot", SeverityError) {
		t.Fatalf("expected empty_snapshot error, got %+v", rep.Issues)
	}
}

// TestValidateMultipleIssueKinds: a single bad page can raise several errors.
func TestValidateMultipleIssueKinds(t *testing.T) {
	s := &Snapshot{Pages: []Page{vpage("https://e.com/a", "", 0, "")}}
	rep := Validate(s)
	if !hasKindSev(rep, "missing_status", SeverityError) || !hasKindSev(rep, "missing_content_type", SeverityError) {
		t.Fatalf("expected missing_status and missing_content_type errors, got %+v", rep.Issues)
	}
}

// TestValidationPrintFormat verifies the report renders grouped by severity.
func TestValidationPrintFormat(t *testing.T) {
	s := &Snapshot{BaseURL: "https://e.com", Pages: []Page{
		vpage("https://e.com/a", "", 0, ""),
		vpage("https://e.com/{x}", "https://e.com/", 200, "text/html"),
	}}
	var buf bytes.Buffer
	rep := Validate(s)
	rep.Print(&buf)
	out := buf.String()
	for _, want := range []string{"Snapshot Validation", "Errors (2)", "Warnings (1)", "missing_status", "template_placeholder", "✗ Snapshot validation failed."} {
		if !bytes.Contains(buf.Bytes(), []byte(want)) {
			t.Errorf("report missing %q\n---\n%s", want, out)
		}
	}
}
