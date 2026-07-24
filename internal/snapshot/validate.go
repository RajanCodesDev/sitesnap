package snapshot

import (
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"
)

// Severity classifies a validation issue. ERROR means SiteSnap produced an
// invalid snapshot or hit an internal inconsistency and the crawl must fail.
// WARNING means the target website has a quality issue (e.g. trailing-slash
// variants) that should be reported but must never fail the crawl.
type Severity int

const (
	SeverityWarning Severity = iota
	SeverityError
)

func (s Severity) String() string {
	if s == SeverityError {
		return "error"
	}
	return "warning"
}

// ValidationIssue describes a single problem found while validating a snapshot.
type ValidationIssue struct {
	URL      string   `json:"url"`
	Kind     string   `json:"kind"`
	Severity Severity `json:"severity"`
	Detail   string   `json:"detail"`
	// Parents lists the URLs where the issue was discovered (when relevant).
	Parents []string `json:"parents,omitempty"`
}

// ValidationReport is the result of validating a snapshot.
type ValidationReport struct {
	Issues []ValidationIssue `json:"issues"`
}

// knownResourceTypes is the set of ResourceType values the crawler may emit.
var knownResourceTypes = map[string]bool{
	"":       true,
	"html":   true,
	"css":    true,
	"script": true,
	"image":  true,
	"font":   true,
	"text":   true,
	"other":  true,
}

// Validate checks a snapshot for the conditions that make a stored baseline
// unreliable for deployment diffing. Issues are classified by severity:
//
//   - ERROR: empty URL, invalid URL, missing status code, missing content
//     type, invalid resource type, or a corrupted/empty structure. These
//     indicate a SiteSnap fault or internal inconsistency.
//   - WARNING: website-quality issues such as template placeholders
//     (e.g. /{item.link}) or suspicious URLs. Duplicate URLs are reported by
//     the dupdetect package, not here.
//
// Duplicate detection (trailing-slash, fragment, query, and canonicalization
// variants) lives in the dupdetect package and is merged into the unified
// validation report by the cli layer.
func Validate(s *Snapshot) *ValidationReport {
	rep := &ValidationReport{}

	if s == nil || len(s.Pages) == 0 {
		rep.Issues = append(rep.Issues, ValidationIssue{
			URL:      "",
			Kind:     "empty_snapshot",
			Severity: SeverityError,
			Detail:   "snapshot contains no pages (corrupted or empty structure)",
		})
		return rep
	}
	if s.BaseURL == "" {
		rep.Issues = append(rep.Issues, ValidationIssue{
			URL:      "",
			Kind:     "missing_base_url",
			Severity: SeverityError,
			Detail:   "snapshot is missing its base URL",
		})
	}

	for _, p := range s.Pages {
		if p.URL == "" {
			rep.Issues = append(rep.Issues, ValidationIssue{
				URL:      p.URL,
				Kind:     "empty_url",
				Severity: SeverityError,
				Detail:   "page has an empty URL",
			})
			continue
		}

		u, err := url.Parse(p.URL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			rep.Issues = append(rep.Issues, ValidationIssue{
				URL:      p.URL,
				Kind:     "invalid_url",
				Severity: SeverityError,
				Detail:   "URL is not a valid absolute http(s) URL",
			})
		} else if u.Scheme != "http" && u.Scheme != "https" {
			rep.Issues = append(rep.Issues, ValidationIssue{
				URL:      p.URL,
				Kind:     "invalid_url",
				Severity: SeverityError,
				Detail:   "URL scheme must be http or https",
			})
		}

		if p.StatusCode == 0 {
			rep.Issues = append(rep.Issues, ValidationIssue{
				URL:      p.URL,
				Kind:     "missing_status",
				Severity: SeverityError,
				Detail:   "page has no status code recorded",
			})
		}

		if strings.TrimSpace(p.ContentType) == "" {
			rep.Issues = append(rep.Issues, ValidationIssue{
				URL:      p.URL,
				Kind:     "missing_content_type",
				Severity: SeverityError,
				Detail:   "page has no content type recorded",
			})
		}

		if !knownResourceTypes[p.ResourceType] {
			rep.Issues = append(rep.Issues, ValidationIssue{
				URL:      p.URL,
				Kind:     "invalid_resource_type",
				Severity: SeverityError,
				Detail:   fmt.Sprintf("unknown resource type %q", p.ResourceType),
			})
		}

		// Website-quality warnings (do not fail the crawl).
		if strings.Contains(p.URL, "{") || strings.Contains(p.URL, "}") {
			rep.Issues = append(rep.Issues, ValidationIssue{
				URL:      p.URL,
				Kind:     "template_placeholder",
				Severity: SeverityWarning,
				Detail:   "URL contains a template placeholder (e.g. /{item.link})",
				Parents:  []string{p.ParentURL},
			})
		}
	}

	sort.Slice(rep.Issues, func(i, j int) bool {
		if rep.Issues[i].Severity != rep.Issues[j].Severity {
			return rep.Issues[i].Severity > rep.Issues[j].Severity // errors first
		}
		if rep.Issues[i].URL != rep.Issues[j].URL {
			return rep.Issues[i].URL < rep.Issues[j].URL
		}
		return rep.Issues[i].Kind < rep.Issues[j].Kind
	})
	return rep
}

// HasErrors reports whether any issue has ERROR severity.
func (r *ValidationReport) HasErrors() bool {
	for _, i := range r.Issues {
		if i.Severity == SeverityError {
			return true
		}
	}
	return false
}

// Warnings returns only the WARNING-severity issues.
func (r *ValidationReport) Warnings() []ValidationIssue {
	var out []ValidationIssue
	for _, i := range r.Issues {
		if i.Severity == SeverityWarning {
			out = append(out, i)
		}
	}
	return out
}

// Errors returns only the ERROR-severity issues.
func (r *ValidationReport) Errors() []ValidationIssue {
	var out []ValidationIssue
	for _, i := range r.Issues {
		if i.Severity == SeverityError {
			out = append(out, i)
		}
	}
	return out
}

// Valid reports whether the snapshot passed validation with no issues.
func (r *ValidationReport) Valid() bool {
	return len(r.Issues) == 0
}

// Print writes the validation report grouped by severity to w.
func (r *ValidationReport) Print(w io.Writer) {
	errors := r.Errors()
	warnings := r.Warnings()

	fmt.Fprintln(w, "Snapshot Validation")
	fmt.Fprintln(w)

	fmt.Fprintf(w, "Errors (%d)\n", len(errors))
	if len(errors) == 0 {
		fmt.Fprintln(w, "No errors found.")
	}
	for _, iss := range errors {
		printIssue(w, iss)
	}
	fmt.Fprintln(w)

	fmt.Fprintf(w, "Warnings (%d)\n", len(warnings))
	if len(warnings) == 0 {
		fmt.Fprintln(w, "No warnings found.")
	}
	for _, iss := range warnings {
		printIssue(w, iss)
	}
	fmt.Fprintln(w)

	fmt.Fprintf(w, "Summary\n")
	fmt.Fprintf(w, "  Warnings: %d\n", len(warnings))
	fmt.Fprintf(w, "  Errors: %d\n", len(errors))
	if len(errors) == 0 {
		fmt.Fprintln(w, "✓ Snapshot validation passed.")
	} else {
		fmt.Fprintln(w, "✗ Snapshot validation failed.")
	}
}

func printIssue(w io.Writer, iss ValidationIssue) {
	fmt.Fprintf(w, "[%s] %s\n", iss.Kind, iss.URL)
	fmt.Fprintf(w, "  %s\n", iss.Detail)
	if len(iss.Parents) > 0 {
		fmt.Fprintf(w, "  Found from: %s\n", strings.Join(iss.Parents, ", "))
	}
}
