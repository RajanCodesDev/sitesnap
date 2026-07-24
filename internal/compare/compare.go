package compare

// Package compare produces a deployment regression report by diffing two
// snapshots. It reports only what the product defines: added URLs, removed
// URLs, status code changes, and content-type changes.

import (
	"sort"
	"strconv"

	"github.com/RajanCodesDev/sitesnap/internal/snapshot"
)

// Change describes a single differing attribute for a URL present in both
// snapshots.
type Change struct {
	URL      string `json:"url"`
	Previous string `json:"previous"`
	Current  string `json:"current"`
}

// Report is the result of comparing a previous and a current snapshot.
type Report struct {
	Added              []string `json:"added"`
	Removed            []string `json:"removed"`
	StatusChanges      []Change `json:"status_changes"`
	ContentTypeChanges []Change `json:"content_type_changes"`
}

// Compare diffs curr against prev and returns a Report. URLs present in curr
// but not prev are Added; those in prev but not curr are Removed; matching URLs
// with differing status codes or content types produce Changes.
func Compare(prev, curr *snapshot.Snapshot) *Report {
	prevMap := make(map[string]snapshot.Page, len(prev.Pages))
	for _, p := range prev.Pages {
		prevMap[p.URL] = p
	}
	currMap := make(map[string]snapshot.Page, len(curr.Pages))
	for _, p := range curr.Pages {
		currMap[p.URL] = p
	}

	r := &Report{}
	for url, cp := range currMap {
		pp, ok := prevMap[url]
		if !ok {
			r.Added = append(r.Added, url)
			continue
		}
		if pp.StatusCode != cp.StatusCode {
			r.StatusChanges = append(r.StatusChanges, Change{
				URL:      url,
				Previous: statusText(pp.StatusCode),
				Current:  statusText(cp.StatusCode),
			})
		}
		if pp.ContentType != cp.ContentType {
			r.ContentTypeChanges = append(r.ContentTypeChanges, Change{
				URL:      url,
				Previous: pp.ContentType,
				Current:  cp.ContentType,
			})
		}
	}
	for url := range prevMap {
		if _, ok := currMap[url]; !ok {
			r.Removed = append(r.Removed, url)
		}
	}

	sort.Strings(r.Added)
	sort.Strings(r.Removed)
	sort.Slice(r.StatusChanges, func(i, j int) bool { return r.StatusChanges[i].URL < r.StatusChanges[j].URL })
	sort.Slice(r.ContentTypeChanges, func(i, j int) bool { return r.ContentTypeChanges[i].URL < r.ContentTypeChanges[j].URL })
	return r
}

func statusText(code int) string {
	if code == 0 {
		return "unreachable"
	}
	return strconv.Itoa(code)
}
