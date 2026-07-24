// Package cli implements the SiteSnap command-line interface and orchestrates
// the crawl -> load -> compare -> replace workflow.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"
	"sitesnap/internal/compare"
	"sitesnap/internal/crawler"
	"sitesnap/internal/dupdetect"
	"sitesnap/internal/exporter"
	"sitesnap/internal/report"
	"sitesnap/internal/snapshot"
	"sitesnap/internal/validation"
	"time"
	"path/filepath"
)

// Run executes the SiteSnap CLI with the given arguments.
func Run(args []string) error {
	// Centralized panic recovery. Registered first so it catches any unexpected
	// panic raised anywhere in the workflow (crawl, compare, replace). On a
	// panic it removes the temporary snapshot, releases the lock, preserves the
	// existing snapshot, prints a concise user-friendly message, and exits
	// non-zero. SITESNAP_DEBUG=1 additionally prints the panic value and the
	// full Go stack trace. Cleanup always runs before any diagnostic output.
	var (
		tmpPath string
		release func() error
	)
	defer recoverPanic(&tmpPath, &release)

	fs := flag.NewFlagSet("sitesnap", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: sitesnap [options] <base-url>\n\n")
		fmt.Fprintf(fs.Output(), "Crawl a website and detect deployment regressions by comparing\n")
		fmt.Fprintf(fs.Output(), "against the latest stored snapshot.\n\n")
		fs.PrintDefaults()
	}

	var (
		workers   = fs.Int("workers", 50, "number of concurrent crawl workers")
		timeout   = fs.Duration("timeout", 30*time.Second, "per-request timeout")
		snapPath  = fs.String("snapshot", "snapshot.json", "path to the stored snapshot file")
		jsonOut   = fs.Bool("json", false, "emit the raw comparison report as JSON")
		noReplace = fs.Bool("no-replace", false, "do not replace the stored snapshot after comparing")
		quiet     = fs.Bool("quiet", false, "suppress the progress bar")
		strict    = fs.Bool("strict", false, "treat warnings (e.g. URL duplicates) as errors; fails the crawl if any exist")
		csvOut    = fs.String(
			"csv",
			"",
			"export comparison report as CSV files",
		)
	)

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return fmt.Errorf("exactly one base-url argument is required")
	}
	baseURL := fs.Arg(0)

	cfg := crawler.Config{
		BaseURL:   baseURL,
		Workers:   *workers,
		Timeout:   *timeout,
		UserAgent: "SiteSnap/0.1",
	}
	if !*quiet {
		cfg.Progress = progressBar
	}

	fmt.Fprintf(os.Stderr, "Crawling %s with %d workers...\n", baseURL, *workers)
	start := time.Now()
	curr, err := crawler.Crawl(cfg)
	if err != nil {
		return fmt.Errorf("crawl failed: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Crawled %d URLs in %s\n\n", len(curr.Pages), time.Since(start).Round(time.Millisecond))

	// Ensure only one SiteSnap process operates on this snapshot at a time.
	// The lock is released via defer so it is freed even on error or panic.
	release, err = snapshot.Lock(*snapPath)
	if err != nil {
		var held *snapshot.LockHeldError
		if errors.As(err, &held) {
			return fmt.Errorf("sitesnap is already running for %s (lock held at %s); exiting", *snapPath, held.Path)
		}
		return fmt.Errorf("failed to acquire lock: %w", err)
	}

	// Temporary snapshot path for this execution; used by the panic recovery to
	// clean up on unexpected failure. The lock itself is released by the
	// deferred recoverPanic at the top of Run.
	tmpPath = snapshot.TempFile(*snapPath)

	// Remove any stale temporary snapshot left by a previous crashed run. This
	// runs only after we hold the lock, so it is safe and never reuses a stale
	// temp. It is reported as an informational message, not an error.
	if removedPath, removed, cerr := snapshot.CleanupTemp(*snapPath); cerr != nil {
		return fmt.Errorf("failed to clean up stale temp file: %w", cerr)
	} else if removed {
		fmt.Fprintln(os.Stderr, "Found stale temporary snapshot from a previous execution.")
		fmt.Fprintf(os.Stderr, "Removing %s...\n", removedPath)
	}

	// Load the previous snapshot (if any) so crawl-quality sanity checks can
	// compare against it. The previous snapshot is never modified here.
	var prev *snapshot.Snapshot
	if snapshot.Exists(*snapPath) {
		prev, err = snapshot.Load(*snapPath)
		if err != nil {
			return fmt.Errorf("failed to load previous snapshot: %w", err)
		}
	}

	// Validate the freshly crawled snapshot. Structural problems (empty/invalid
	// URLs, missing status or content type, invalid resource type) are errors
	// that fail the crawl. Duplicate URLs and website-quality issues are
	// warnings that are reported but never stop normal operation unless
	// --strict is set, in which case warnings are promoted to errors.
	dups := dupdetect.Detect(curr)
	if prev != nil {
		dups.PreviousCount = len(prev.Pages)
	}
	vrep := validation.Build(curr, dups)
	if *strict {
		vrep.PromoteWarnings()
	}
	vrep.Print(os.Stderr)

	if vrep.HasErrors() {
		return fmt.Errorf("snapshot validation failed: %d error(s)", len(vrep.Errors()))
	}

	// Write the new snapshot to a temporary file, then durably promote it.
	if err := snapshot.Save(tmpPath, curr); err != nil {
		return fmt.Errorf("failed to write snapshot: %w", err)
	}
	if *csvOut != "" {
		if err := exporter.ExportSnapshotCSV(
			curr,
			filepath.Join(*csvOut, "snapshot.csv"),
		); err != nil {
			return fmt.Errorf("failed to export snapshot csv: %w", err)
		}
	}

	if prev == nil {
		if err := snapshot.AtomicReplace(tmpPath, *snapPath); err != nil {
			return fmt.Errorf("failed to store baseline snapshot: %w", err)
		}

		// Export crawl snapshot as CSV if requested.


		fmt.Printf("Baseline snapshot saved to %s (%d URLs).\n", *snapPath, len(curr.Pages))
		return nil
	}

	rep := compare.Compare(prev, curr)

	if *csvOut != "" {
		if err := exporter.ExportCSV(rep, *csvOut); err != nil {
			return fmt.Errorf("failed to export csv: %w", err)
		}
	}

	if *jsonOut {
		data, err := report.ToJSON(rep)
		if err != nil {
			return fmt.Errorf("failed to marshal report: %w", err)
		}
		fmt.Println(string(data))
	} else {
		report.Print(os.Stdout, prev, curr, rep)
	}

	if !*noReplace {
		if err := snapshot.AtomicReplace(tmpPath, *snapPath); err != nil {
			return fmt.Errorf("failed to replace stored snapshot: %w", err)
		}
		fmt.Fprintf(os.Stderr, "\nStored snapshot replaced at %s.\n", *snapPath)
	} else {
		if err := os.Remove(tmpPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove temporary snapshot: %w", err)
		}
	}

	return nil
}

// exitProcess terminates the process. It is a variable (rather than a direct
// os.Exit call) so tests, and future debug/CLI options, can intercept the exit
// without duplicating the recovery logic.
var exitProcess = os.Exit

// recoverPanic is the single, centralized panic recovery point for SiteSnap. It
// is deferred at the very top of Run so it catches any unexpected panic raised
// anywhere in the crawl/compare/replace workflow.
//
// On a normal return (or expected error) it releases the snapshot lock if it
// was acquired. On an unexpected panic it additionally guarantees, in order:
//   - the temporary snapshot created by this execution is removed,
//   - the existing snapshot is preserved (never modified here),
//   - a concise, user-friendly message is printed (no panic/stack by default),
//   - and the process exits non-zero.
//
// If SITESNAP_DEBUG=1 is set, the original panic value and the complete Go
// stack trace are printed as well, after cleanup. Cleanup always runs first.
func recoverPanic(tmpPath *string, release *func() error) {
	r := recover()

	// Release the snapshot lock if we acquired it (normal return or panic).
	if release != nil && *release != nil {
		if rerr := (*release)(); rerr != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to release lock: %v\n", rerr)
		}
	}

	if r == nil {
		return
	}

	// 1. Remove the temporary snapshot created by this execution (if any).
	if tmpPath != nil && *tmpPath != "" {
		if _, statErr := os.Stat(*tmpPath); statErr == nil {
			if rmErr := os.Remove(*tmpPath); rmErr != nil && !os.IsNotExist(rmErr) {
				fmt.Fprintf(os.Stderr, "warning: failed to remove temporary snapshot: %v\n", rmErr)
			}
		}
	}

	// 2. The existing snapshot is preserved: we never touch snapshot.json here.

	// 3 & 4. Print a concise message, or the full diagnostic in debug mode.
	if os.Getenv("SITESNAP_DEBUG") == "1" {
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "SiteSnap encountered an unexpected internal error.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Cleaning up temporary files...")
		fmt.Fprintln(os.Stderr, "Previous snapshot preserved.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintf(os.Stderr, "panic: %v\n\n", r)
		buf := make([]byte, 1<<20)
		n := runtime.Stack(buf, true)
		fmt.Fprintf(os.Stderr, "%s", buf[:n])
	} else {
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "SiteSnap encountered an unexpected internal error.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "This indicates a bug in SiteSnap.")
		fmt.Fprintln(os.Stderr, "The previous snapshot has been preserved.")
		fmt.Fprintln(os.Stderr, "Temporary files have been cleaned up.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "To display the full panic and stack trace, run:")
		fmt.Fprintf(os.Stderr, "\n    SITESNAP_DEBUG=1 sitesnap crawl <url>\n\n")
		fmt.Fprintln(os.Stderr, "Please report this issue if it is reproducible.")
	}

	// Exit non-zero. Cleanup (temp removal + lock release) has already run.
	exitProcess(1)
}

// progressBar renders a simple one-line progress indicator to stderr.
func progressBar(crawled, pending int) {
	fmt.Fprintf(os.Stderr, "\r  crawled: %d  pending: %d", crawled, pending)
	if pending == 0 {
		fmt.Fprint(os.Stderr, "\n")
	}
}
