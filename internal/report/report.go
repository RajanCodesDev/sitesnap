package report

// Package report renders a deployment comparison as a human-readable text
// report with summary statistics, and can also emit the raw report as JSON.

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/RajanCodesDev/sitesnap/internal/compare"
	"github.com/RajanCodesDev/sitesnap/internal/snapshot"
)

// Stats is the summary statistics for a comparison.
type Stats struct {
	TotalPrevious      int `json:"total_previous"`
	TotalCurrent       int `json:"total_current"`
	Added              int `json:"added"`
	Removed            int `json:"removed"`
	StatusChanges      int `json:"status_changes"`
	ContentTypeChanges int `json:"content_type_changes"`
}

// Statistics computes summary counts from a comparison.
func Statistics(prev, curr *snapshot.Snapshot, r *compare.Report) Stats {
	return Stats{
		TotalPrevious:      len(prev.Pages),
		TotalCurrent:       len(curr.Pages),
		Added:              len(r.Added),
		Removed:            len(r.Removed),
		StatusChanges:      len(r.StatusChanges),
		ContentTypeChanges: len(r.ContentTypeChanges),
	}
}

// Print writes a formatted deployment report to w.
func Print(w io.Writer, prev, curr *snapshot.Snapshot, r *compare.Report) {
	stats := Statistics(prev, curr, r)

	fmt.Fprintln(w, "=== SiteSnap Deployment Report ===")
	fmt.Fprintf(w, "Base URL:   %s\n", curr.BaseURL)
	if prev != nil {
		fmt.Fprintf(w, "Previous:   %s\n", prev.CrawledAt.Format(time.RFC3339))
	}
	fmt.Fprintf(w, "Current:    %s\n", curr.CrawledAt.Format(time.RFC3339))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Statistics:")
	fmt.Fprintf(w, "  URLs (previous):       %d\n", stats.TotalPrevious)
	fmt.Fprintf(w, "  URLs (current):        %d\n", stats.TotalCurrent)
	fmt.Fprintf(w, "  Added:                 %d\n", stats.Added)
	fmt.Fprintf(w, "  Removed:               %d\n", stats.Removed)
	fmt.Fprintf(w, "  Status changes:        %d\n", stats.StatusChanges)
	fmt.Fprintf(w, "  Content-Type changes:  %d\n", stats.ContentTypeChanges)
	fmt.Fprintln(w)

	printSection(w, "Added URLs", len(r.Added), func() {
		for _, u := range r.Added {
			fmt.Fprintf(w, "  + %s\n", u)
		}
	})
	printSection(w, "Removed URLs", len(r.Removed), func() {
		for _, u := range r.Removed {
			fmt.Fprintf(w, "  - %s\n", u)
		}
	})
	printSection(w, "Status Code Changes", len(r.StatusChanges), func() {
		for _, c := range r.StatusChanges {
			fmt.Fprintf(w, "  ~ %s  %s -> %s\n", c.URL, c.Previous, c.Current)
		}
	})
	printSection(w, "Content-Type Changes", len(r.ContentTypeChanges), func() {
		for _, c := range r.ContentTypeChanges {
			fmt.Fprintf(w, "  ~ %s  %s -> %s\n", c.URL, c.Previous, c.Current)
		}
	})
}

func printSection(w io.Writer, title string, n int, body func()) {
	fmt.Fprintf(w, "%s (%d):\n", title, n)
	if n == 0 {
		fmt.Fprintln(w, "  (none)")
	} else {
		body()
	}
	fmt.Fprintln(w)
}

// ToJSON marshals the report as indented JSON.
func ToJSON(r *compare.Report) ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}
