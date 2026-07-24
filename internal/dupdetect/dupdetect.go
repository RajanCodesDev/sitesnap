// Package dupdetect finds URLs in a snapshot that represent the same resource
// but were recorded as distinct pages (e.g. trailing-slash variants, fragment
// differences, or inconsistent internal linking). It produces a report that
// groups such variants under a single canonical URL so the source of the
// duplication can be traced via each variant's parent URL.
package dupdetect

import (
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"

	"github.com/RajanCodesDev/sitesnap/internal/snapshot"
)

// Severity classifies a duplicate group. Exact duplicates (the same URL
// string recorded more than once) are errors because they indicate a bug in
// SiteSnap or the crawl; variant duplicates (trailing slash, fragment, query,
// or canonicalization differences) are warnings because they reflect the
// target website's URL hygiene, not a SiteSnap fault.
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

// Variant is one recorded occurrence of a duplicated resource.
type Variant struct {
	URL         string `json:"url"`
	ParentURL   string `json:"parent_url"`
	StatusCode  int    `json:"status_code"`
	ContentType string `json:"content_type"`
}

// Group is the set of variant URLs that all resolve to the same resource.
type Group struct {
	Canonical string    `json:"canonical"`
	Variants  []Variant `json:"variants"`
	// Severity is ERROR for exact duplicates, WARNING for variant duplicates.
	Severity Severity `json:"severity"`
	// Reason explains why the variants are considered the same resource
	// (e.g. "Trailing slash variant", "URLs differing only by fragments").
	Reason string `json:"reason"`
}

// Report lists every duplicate group found in a snapshot.
type Report struct {
	Groups []Group `json:"groups"`
	// PreviousCount is the page count of the previously stored snapshot, used
	// for crawl-quality sanity checks. It is 0 when there is no previous
	// snapshot (first crawl).
	PreviousCount int `json:"previous_count"`
}

// Detect inspects a snapshot and returns a Report containing every group of
// pages that resolve to the same resource. A URL is considered a duplicate of
// another when they share a normalized key: lower-cased scheme and host, a
// path with any trailing slash removed (the root "/" is kept), and an
// identical query string. Fragment differences are ignored because they point
// at the same resource.
func Detect(s *snapshot.Snapshot) *Report {
	byKey := make(map[string][]Variant)
	order := make([]string, 0)
	for _, p := range s.Pages {
		k := keyOf(p.URL)
		if _, ok := byKey[k]; !ok {
			order = append(order, k)
		}
		byKey[k] = append(byKey[k], Variant{
			URL:         p.URL,
			ParentURL:   p.ParentURL,
			StatusCode:  p.StatusCode,
			ContentType: p.ContentType,
		})
	}

	rep := &Report{}
	for _, k := range order {
		vs := byKey[k]
		if len(vs) < 2 {
			continue
		}
		sev, reason := classify(vs)
		rep.Groups = append(rep.Groups, Group{
			Canonical: pickCanonical(vs, k),
			Variants:  vs,
			Severity:  sev,
			Reason:    reason,
		})
	}
	sort.Slice(rep.Groups, func(i, j int) bool {
		return rep.Groups[i].Canonical < rep.Groups[j].Canonical
	})
	return rep
}

// classify decides whether a duplicate group is an ERROR (exact duplicate URL
// strings) or a WARNING (variants that differ only by trailing slash,
// fragment, query string, or other canonicalization). It also returns a
// human-readable reason.
func classify(vs []Variant) (Severity, string) {
	exact := true
	for i := 1; i < len(vs); i++ {
		if vs[i].URL != vs[0].URL {
			exact = false
			break
		}
	}
	if exact {
		return SeverityError, "Exact duplicate URL entries"
	}

	// Determine the dominant reason among the variants.
	reasons := make(map[string]int)
	for _, v := range vs {
		reasons[reasonFor(vs[0].URL, v.URL)]++
	}
	best := ""
	bestN := 0
	for r, n := range reasons {
		if n > bestN {
			best, bestN = r, n
		}
	}
	return SeverityWarning, best
}

// reasonFor explains how variant differs from the canonical reference URL.
func reasonFor(ref, variant string) string {
	ru, err1 := url.Parse(ref)
	vu, err2 := url.Parse(variant)
	if err1 != nil || err2 != nil {
		return "Canonicalization duplicate"
	}
	if ru.Path != "/" && strings.TrimSuffix(ru.Path, "/") == strings.TrimSuffix(vu.Path, "/") {
		return "Trailing slash variant"
	}
	if ru.Fragment == "" && vu.Fragment != "" {
		return "URLs differing only by fragments"
	}
	if ru.RawQuery != vu.RawQuery {
		return "Query-string duplicate"
	}
	return "Canonicalization duplicate"
}

// pickCanonical prefers the variant whose own URL already has the normalized
// (no trailing slash) form; otherwise it falls back to the first variant.
func pickCanonical(vs []Variant, key string) string {
	for _, v := range vs {
		if keyOf(v.URL) != key {
			continue
		}
		if u, err := url.Parse(v.URL); err == nil && (u.Path == "/" || !strings.HasSuffix(u.Path, "/")) {
			return v.URL
		}
	}
	return vs[0].URL
}

// keyOf returns the normalization key used for duplicate grouping.
func keyOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Host)
	path := u.Path
	if path != "/" && strings.HasSuffix(path, "/") {
		path = path[:len(path)-1]
	}
	out := scheme + "://" + host + path
	if u.RawQuery != "" {
		out += "?" + u.RawQuery
	}
	return out
}

// Summary returns the total number of duplicate groups and the total number of
// duplicate URLs across all groups.
func (r *Report) Summary() (groups, urls int) {
	for _, g := range r.Groups {
		groups++
		urls += len(g.Variants)
	}
	return
}

// Print writes the duplicate report grouped by severity, in the documented
// format. Each group shows its canonical URL, the reason for duplication, the
// parent URLs where each variant was discovered, and per-variant status and
// content type so the bad link can be traced to its origin.
func Print(w io.Writer, r *Report) {
	errors := r.GroupsBySeverity(SeverityError)
	warnings := r.GroupsBySeverity(SeverityWarning)

	fmt.Fprintln(w, "Duplicate URL Report")
	fmt.Fprintln(w)

	fmt.Fprintf(w, "Errors (%d)\n", len(errors))
	if len(errors) == 0 {
		fmt.Fprintln(w, "No errors found.")
	}
	for _, g := range errors {
		printGroup(w, g)
	}
	fmt.Fprintln(w)

	fmt.Fprintf(w, "Warnings (%d)\n", len(warnings))
	if len(warnings) == 0 {
		fmt.Fprintln(w, "No warnings found.")
	}
	for _, g := range warnings {
		printGroup(w, g)
	}
	fmt.Fprintln(w)

	eg, eu := count(errors)
	wg, wu := count(warnings)
	fmt.Fprintf(w, "Summary\n")
	fmt.Fprintf(w, "  Errors: %d (groups), %d (urls)\n", eg, eu)
	fmt.Fprintf(w, "  Warnings: %d (groups), %d (urls)\n", wg, wu)
}

func printGroup(w io.Writer, g Group) {
	fmt.Fprintf(w, "[%s] %s\n", g.Severity.String(), g.Canonical)
	fmt.Fprintln(w, "Canonical:")
	fmt.Fprintf(w, "%s\n", g.Canonical)
	fmt.Fprintln(w, "Reason:")
	fmt.Fprintf(w, "%s\n", g.Reason)
	fmt.Fprintln(w, "Variants:")
	for _, v := range g.Variants {
		fmt.Fprintf(w, "  • %s\n", v.URL)
		fmt.Fprintf(w, "    Parent: %s\n", parentPath(v.ParentURL))
		if v.StatusCode != 0 {
			fmt.Fprintf(w, "    Status: %d\n", v.StatusCode)
		}
		if v.ContentType != "" {
			fmt.Fprintf(w, "    Content-Type: %s\n", v.ContentType)
		}
	}
	fmt.Fprintln(w)
}

// GroupsBySeverity returns the groups with the given severity, preserving the
// canonical-URL sort order.
func (r *Report) GroupsBySeverity(sev Severity) []Group {
	var out []Group
	for _, g := range r.Groups {
		if g.Severity == sev {
			out = append(out, g)
		}
	}
	return out
}

func count(groups []Group) (g, u int) {
	for _, gr := range groups {
		g++
		u += len(gr.Variants)
	}
	return
}

// parentPath returns the path component of a parent URL, defaulting to "/"
// when the parent is empty or unparseable.
func parentPath(raw string) string {
	if raw == "" {
		return "/"
	}
	u, err := url.Parse(raw)
	if err != nil || u.Path == "" {
		return raw
	}
	return u.Path
}
