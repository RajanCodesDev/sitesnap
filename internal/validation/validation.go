package validation

// Package validation produces a single, severity-grouped validation report
// for a crawled snapshot. It combines the structural checks from the snapshot
// package (empty/invalid URLs, missing status or content type, invalid
// resource type) with the duplicate-URL findings from the dupdetect package
// (exact duplicates as errors, trailing-slash/fragment/query/canonicalization
// variants as warnings).
//
// Severity model:
//   - ERROR: a SiteSnap fault or internal inconsistency. Fails the crawl.
//   - WARNING: a quality issue with the target website. Never fails the crawl
//     unless --strict is enabled, in which case warnings are promoted to errors.

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/RajanCodesDev/sitesnap/internal/dupdetect"
	"github.com/RajanCodesDev/sitesnap/internal/snapshot"
)

// Severity mirrors the severity used by the underlying packages.
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

// Finding is one validation result, normalized across packages.
type Finding struct {
	URL      string   `json:"url"`
	Kind     string   `json:"kind"`
	Severity Severity `json:"severity"`
	Detail   string   `json:"detail"`
	Parents  []string `json:"parents,omitempty"`
}

// Report is the unified, severity-grouped validation result.
type Report struct {
	Findings []Finding `json:"findings"`
}

// Build constructs a unified report from a snapshot and its duplicate groups.
// prev may be nil (first crawl); when provided, crawl-quality sanity checks
// compare the current crawl against the previous snapshot.
func Build(s *snapshot.Snapshot, dups *dupdetect.Report) *Report {
	rep := &Report{}

	for _, iss := range snapshot.Validate(s).Issues {
		rep.Findings = append(rep.Findings, Finding{
			URL:      iss.URL,
			Kind:     iss.Kind,
			Severity: sev(iss.Severity),
			Detail:   iss.Detail,
			Parents:  iss.Parents,
		})
	}

	for _, g := range dups.Groups {
		parents := make([]string, 0, len(g.Variants))
		for _, v := range g.Variants {
			parents = append(parents, v.ParentURL)
		}
		rep.Findings = append(rep.Findings, Finding{
			URL:      g.Canonical,
			Kind:     "duplicate_url",
			Severity: dupSev(g.Severity),
			Detail:   fmt.Sprintf("%s — %d variant(s): %s", g.Reason, len(g.Variants), variantList(g.Variants)),
			Parents:  parents,
		})
	}

	addCrawlSanity(rep, s, dups)
	return rep
}

// addCrawlSanity appends crawl-quality warnings. These are website/operational
// quality signals (not SiteSnap faults) and are therefore WARNINGs by default.
func addCrawlSanity(rep *Report, s *snapshot.Snapshot, dups *dupdetect.Report) {
	// Base URL must be present and have returned HTTP 200. Compare against
	// both the raw base URL and its trailing-slash canonical form so that
	// "https://e.com" matches a recorded "https://e.com/".
	baseCandidates := map[string]bool{s.BaseURL: true}
	if strings.HasSuffix(s.BaseURL, "/") {
		baseCandidates[strings.TrimRight(s.BaseURL, "/")] = true
	} else {
		baseCandidates[s.BaseURL+"/"] = true
	}
	baseOK := false
	for _, p := range s.Pages {
		if !baseCandidates[p.URL] {
			continue
		}
		baseOK = true
		if p.StatusCode != http.StatusOK {
			rep.Findings = append(rep.Findings, Finding{
				URL:      p.URL,
				Kind:     "base_url_status",
				Severity: SeverityWarning,
				Detail:   fmt.Sprintf("base URL returned HTTP %d (expected 200)", p.StatusCode),
			})
		}
		break
	}
	if !baseOK {
		rep.Findings = append(rep.Findings, Finding{
			URL:      s.BaseURL,
			Kind:     "base_url_missing",
			Severity: SeverityWarning,
			Detail:   "base URL was not found among the crawled pages",
		})
	}

	// Suspiciously low page count.
	if len(s.Pages) < 3 {
		rep.Findings = append(rep.Findings, Finding{
			URL:      s.BaseURL,
			Kind:     "low_page_count",
			Severity: SeverityWarning,
			Detail:   fmt.Sprintf("crawl produced only %d pages; possible incomplete crawl", len(s.Pages)),
		})
	}

	// Excessive unreachable pages (status 0 or >= 400).
	unreachable := 0
	for _, p := range s.Pages {
		if p.StatusCode == 0 || p.StatusCode >= 400 {
			unreachable++
		}
	}
	if len(s.Pages) > 0 && float64(unreachable)/float64(len(s.Pages)) > 0.5 {
		rep.Findings = append(rep.Findings, Finding{
			URL:      s.BaseURL,
			Kind:     "excessive_unreachable",
			Severity: SeverityWarning,
			Detail:   fmt.Sprintf("%d of %d pages were unreachable (status 0 or >=400)", unreachable, len(s.Pages)),
		})
	}

	// Compare page count with the previous snapshot.
	if dups != nil && dups.PreviousCount > 0 {
		ratio := float64(len(s.Pages)) / float64(dups.PreviousCount)
		if ratio < 0.5 {
			rep.Findings = append(rep.Findings, Finding{
				URL:      s.BaseURL,
				Kind:     "page_count_drop",
				Severity: SeverityWarning,
				Detail:   fmt.Sprintf("current crawl contains significantly fewer pages than the previous snapshot (previous: %d, current: %d); possible incomplete crawl", dups.PreviousCount, len(s.Pages)),
			})
		}
	}
}

func variantList(vs []dupdetect.Variant) string {
	parts := make([]string, 0, len(vs))
	for _, v := range vs {
		parts = append(parts, v.URL)
	}
	return strings.Join(parts, ", ")
}

func sev(s snapshot.Severity) Severity {
	if s == snapshot.SeverityError {
		return SeverityError
	}
	return SeverityWarning
}

func dupSev(s dupdetect.Severity) Severity {
	if s == dupdetect.SeverityError {
		return SeverityError
	}
	return SeverityWarning
}

// PromoteWarnings upgrades every WARNING finding to ERROR. Used by --strict
// mode so that any website-quality issue fails the crawl (CI/CD use case).
func (r *Report) PromoteWarnings() {
	for i := range r.Findings {
		if r.Findings[i].Severity == SeverityWarning {
			r.Findings[i].Severity = SeverityError
		}
	}
}

// HasErrors reports whether any finding is an ERROR.
func (r *Report) HasErrors() bool {
	for _, f := range r.Findings {
		if f.Severity == SeverityError {
			return true
		}
	}
	return false
}

// Errors returns only ERROR findings.
func (r *Report) Errors() []Finding {
	var out []Finding
	for _, f := range r.Findings {
		if f.Severity == SeverityError {
			out = append(out, f)
		}
	}
	return out
}

// Warnings returns only WARNING findings.
func (r *Report) Warnings() []Finding {
	var out []Finding
	for _, f := range r.Findings {
		if f.Severity == SeverityWarning {
			out = append(out, f)
		}
	}
	return out
}

// Print writes the report grouped by severity, matching the documented format.
func (r *Report) Print(w io.Writer) {
	errors := r.Errors()
	warnings := r.Warnings()

	fmt.Fprintln(w, "Snapshot Validation")
	fmt.Fprintln(w)

	fmt.Fprintf(w, "Errors (%d)\n", len(errors))
	if len(errors) == 0 {
		fmt.Fprintln(w, "No errors found.")
	}
	for _, f := range errors {
		printFinding(w, f)
	}
	fmt.Fprintln(w)

	fmt.Fprintf(w, "Warnings (%d)\n", len(warnings))
	if len(warnings) == 0 {
		fmt.Fprintln(w, "No warnings found.")
	}
	for _, f := range warnings {
		printFinding(w, f)
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

func printFinding(w io.Writer, f Finding) {
	fmt.Fprintf(w, "[%s] %s\n", f.Kind, f.URL)
	fmt.Fprintf(w, "  %s\n", f.Detail)
	if len(f.Parents) > 0 {
		fmt.Fprintf(w, "  Found from: %s\n", strings.Join(f.Parents, ", "))
	}
}
