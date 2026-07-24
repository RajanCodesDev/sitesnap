package cli

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sitesnap/internal/snapshot"
)

// serveSite returns a tiny site: "/" links to /a and /b; /b returns 500 so the
// crawl still succeeds structurally (status recorded, not an error).
func serveSite() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><body><a href="/a">a</a><a href="/b">b</a></body></html>`)
	})
	mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><body><a href="/">home</a></body></html>`)
	})
	mux.HandleFunc("/b", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		fmt.Fprint(w, `<html><body>boom</body></html>`)
	})
	return httptest.NewServer(mux)
}

func run(t *testing.T, args ...string) error {
	t.Helper()
	return Run(args)
}

// TestRunFirstCrawlCreatesBaseline verifies a first crawl writes a baseline
// snapshot and removes the lock afterwards.
func TestRunFirstCrawlCreatesBaseline(t *testing.T) {
	srv := serveSite()
	defer srv.Close()
	dir := t.TempDir()
	snap := filepath.Join(dir, "snapshot.json")

	if err := run(t, "-quiet", "-snapshot", snap, srv.URL); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !snapshot.Exists(snap) {
		t.Fatal("baseline snapshot should exist")
	}
	if _, err := os.Stat(snapshot.LockFile(snap)); !os.IsNotExist(err) {
		t.Fatal("lock file should be released after run")
	}
}

// TestRunLockExcludesConcurrentRuns verifies a second invocation while the
// first holds the lock fails with a clear "already running" error. We simulate
// this by acquiring the lock ourselves first, then calling Run.
func TestRunLockExcludesConcurrentRuns(t *testing.T) {
	srv := serveSite()
	defer srv.Close()
	dir := t.TempDir()
	snap := filepath.Join(dir, "snapshot.json")

	release, err := snapshot.Lock(snap)
	if err != nil {
		t.Fatalf("pre-lock: %v", err)
	}
	defer func() { _ = release() }()

	err = run(t, "-quiet", "-snapshot", snap, srv.URL)
	if err == nil {
		t.Fatal("expected Run to fail while lock is held")
	}
	if !strings.Contains(err.Error(), "already running") {
		t.Fatalf("expected 'already running' error, got %v", err)
	}
}

// TestRunValidationFailurePreservesSnapshot verifies that when validation
// fails, the previously stored snapshot is left untouched.
func TestRunValidationFailurePreservesSnapshot(t *testing.T) {
	srv := serveSite()
	defer srv.Close()
	dir := t.TempDir()
	snap := filepath.Join(dir, "snapshot.json")

	// Seed a "good" previous snapshot.
	good := &snapshot.Snapshot{
		BaseURL: srv.URL,
		Pages:   []snapshot.Page{{URL: srv.URL + "/", StatusCode: 200, ContentType: "text/html"}},
	}
	if err := snapshot.Save(snap, good); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Force a validation error: point at a valid http URL that is
	// unreachable (connection refused) so the crawl succeeds but the page
	// records status 0, which validation rejects.
	err := run(t, "-quiet", "-snapshot", snap, "http://127.0.0.1:1/")
	if err == nil {
		t.Fatal("expected validation failure")
	}
	if !strings.Contains(err.Error(), "validation failed") {
		t.Fatalf("expected validation failure, got %v", err)
	}

	// The good snapshot must be preserved byte-for-byte.
	got, lerr := snapshot.Load(snap)
	if lerr != nil {
		t.Fatalf("load preserved: %v", lerr)
	}
	if len(got.Pages) != 1 || got.Pages[0].URL != srv.URL+"/" {
		t.Fatalf("previous snapshot was modified: %+v", got)
	}
	if _, statErr := os.Stat(snapshot.TempFile(snap)); !os.IsNotExist(statErr) {
		t.Fatal("no temp file should remain after validation failure")
	}
}

// TestRunAtomicReplacement verifies a successful second crawl replaces the
// stored snapshot with the new content (atomic rename via AtomicReplace).
func TestRunAtomicReplacement(t *testing.T) {
	srv := serveSite()
	defer srv.Close()
	dir := t.TempDir()
	snap := filepath.Join(dir, "snapshot.json")

	// First crawl establishes a baseline.
	if err := run(t, "-quiet", "-snapshot", snap, srv.URL); err != nil {
		t.Fatalf("first run: %v", err)
	}
	before, err := snapshot.Load(snap)
	if err != nil {
		t.Fatalf("load before: %v", err)
	}

	// Second crawl should replace it with a comparable (not identical) set.
	if err := run(t, "-quiet", "-snapshot", snap, srv.URL); err != nil {
		t.Fatalf("second run: %v", err)
	}
	after, err := snapshot.Load(snap)
	if err != nil {
		t.Fatalf("load after: %v", err)
	}
	if len(after.Pages) < 1 {
		t.Fatal("replaced snapshot should have pages")
	}
	// The crawl timestamp should differ, proving replacement occurred.
	if !after.CrawledAt.After(before.CrawledAt) && after.CrawledAt.Equal(before.CrawledAt) {
		t.Fatal("expected a fresh crawl timestamp after replacement")
	}
	if _, statErr := os.Stat(snapshot.TempFile(snap)); !os.IsNotExist(statErr) {
		t.Fatal("temp file should be gone after replacement")
	}
}

// TestRunStaleTempCleanup verifies a leftover .tmp is removed at startup.
func TestRunStaleTempCleanup(t *testing.T) {
	srv := serveSite()
	defer srv.Close()
	dir := t.TempDir()
	snap := filepath.Join(dir, "snapshot.json")

	// Leave a stale temp behind.
	if err := os.WriteFile(snapshot.TempFile(snap), []byte("stale"), 0o644); err != nil {
		t.Fatalf("write stale: %v", err)
	}

	if err := run(t, "-quiet", "-snapshot", snap, srv.URL); err != nil {
		t.Fatalf("run: %v", err)
	}
	if _, err := os.Stat(snapshot.TempFile(snap)); !os.IsNotExist(err) {
		t.Fatal("stale temp should have been cleaned up")
	}
}

// TestRunStaleTempCleanupLogs verifies the cleanup is reported (not silent).
func TestRunStaleTempCleanupLogs(t *testing.T) {
	srv := serveSite()
	defer srv.Close()
	dir := t.TempDir()
	snap := filepath.Join(dir, "snapshot.json")

	if err := os.WriteFile(snapshot.TempFile(snap), []byte("stale"), 0o644); err != nil {
		t.Fatalf("write stale: %v", err)
	}

	// Capture stderr to confirm the informational messages are printed.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	old := os.Stderr
	os.Stderr = w
	runErr := run(t, "-quiet", "-snapshot", snap, srv.URL)
	_ = w.Close()
	os.Stderr = old

	var buf strings.Builder
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("copy stderr: %v", err)
	}
	if runErr != nil {
		t.Fatalf("run: %v", runErr)
	}
	out := buf.String()
	if !strings.Contains(out, "Found stale temporary snapshot from a previous execution.") {
		t.Fatalf("expected stale-temp notice, got:\n%s", out)
	}
	if !strings.Contains(out, "Removing "+snapshot.TempFile(snap)+"...") {
		t.Fatalf("expected removing message, got:\n%s", out)
	}
}

// TestRecoverPanicRemovesTempPreservesSnapshotAndExits verifies the panic
// recovery contract: the temp file is removed, the real snapshot is preserved,
// the process is terminated non-zero, and a concise user-friendly message is
// printed (no raw panic or stack trace by default).
func TestRecoverPanicRemovesTempPreservesSnapshotAndExits(t *testing.T) {
	dir := t.TempDir()
	snap := filepath.Join(dir, "snapshot.json")
	tmp := snapshot.TempFile(snap)

	// Seed a real, good snapshot that must survive the panic.
	good := &snapshot.Snapshot{
		BaseURL: "https://e.com",
		Pages:   []snapshot.Page{{URL: "https://e.com/", StatusCode: 200, ContentType: "text/html"}},
	}
	if err := snapshot.Save(snap, good); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Create a temp file that the panic recovery must remove.
	if err := os.WriteFile(tmp, []byte("partial"), 0o644); err != nil {
		t.Fatalf("write temp: %v", err)
	}

	// Capture stderr to verify the friendly message (no raw panic by default).
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	oldStderr := os.Stderr
	os.Stderr = w

	// Intercept the process exit so the test can continue and assert on it.
	oldExit := exitProcess
	exitProcess = func(int) { panic("sitesnap-exit") }
	defer func() { exitProcess = oldExit }()

	var release func() error
	func() {
		defer func() {
			if rec := recover(); rec == nil {
				t.Fatal("expected recoverPanic to terminate the process")
			}
		}()
		defer recoverPanic(&tmp, &release)
		panic("boom")
	}()

	_ = w.Close()
	os.Stderr = oldStderr
	var buf strings.Builder
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("copy stderr: %v", err)
	}
	out := buf.String()

	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Fatal("temp file should be removed by panic recovery")
	}
	got, lerr := snapshot.Load(snap)
	if lerr != nil {
		t.Fatalf("real snapshot should be preserved: %v", lerr)
	}
	if len(got.Pages) != 1 {
		t.Fatalf("real snapshot was modified: %+v", got)
	}
	if !strings.Contains(out, "SiteSnap encountered an unexpected internal error.") {
		t.Fatalf("expected friendly header, got:\n%s", out)
	}
	if !strings.Contains(out, "SITESNAP_DEBUG=1 sitesnap crawl <url>") {
		t.Fatalf("expected debug hint, got:\n%s", out)
	}
	if strings.Contains(out, "panic: boom") || strings.Contains(out, "goroutine") {
		t.Fatalf("normal mode must not print the raw panic/stack, got:\n%s", out)
	}
}

// TestRecoverPanicReleasesLock verifies the snapshot lock is released even on
// an unexpected panic.
func TestRecoverPanicReleasesLock(t *testing.T) {
	dir := t.TempDir()
	snap := filepath.Join(dir, "snapshot.json")

	release, err := snapshot.Lock(snap)
	if err != nil {
		t.Fatalf("pre-lock: %v", err)
	}

	oldExit := exitProcess
	exitProcess = func(int) { panic("sitesnap-exit") }
	defer func() { exitProcess = oldExit }()

	func() {
		defer func() { _ = recover() }()
		defer recoverPanic(new(string), &release)
		panic("boom")
	}()

	if _, err := os.Stat(snapshot.LockFile(snap)); !os.IsNotExist(err) {
		t.Fatal("lock file should be released after panic recovery")
	}
}

// TestRecoverPanicDebugPrintsPanicAndStack verifies that with SITESNAP_DEBUG=1
// the original panic and the full Go stack trace are printed (after cleanup).
func TestRecoverPanicDebugPrintsPanicAndStack(t *testing.T) {
	t.Setenv("SITESNAP_DEBUG", "1")
	dir := t.TempDir()
	snap := filepath.Join(dir, "snapshot.json")
	tmp := snapshot.TempFile(snap)
	if err := os.WriteFile(tmp, []byte("partial"), 0o644); err != nil {
		t.Fatalf("write temp: %v", err)
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	oldStderr := os.Stderr
	os.Stderr = w

	oldExit := exitProcess
	exitProcess = func(int) { panic("sitesnap-exit") }
	defer func() { exitProcess = oldExit }()

	var release func() error
	func() {
		defer func() { _ = recover() }()
		defer recoverPanic(&tmp, &release)
		panic("boom")
	}()

	_ = w.Close()
	os.Stderr = oldStderr
	var buf strings.Builder
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("copy stderr: %v", err)
	}
	out := buf.String()

	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Fatal("temp should be removed in debug mode too")
	}
	if !strings.Contains(out, "panic: boom") {
		t.Fatalf("debug mode must print the panic, got:\n%s", out)
	}
	if !strings.Contains(out, "goroutine") {
		t.Fatalf("debug mode must print the stack trace, got:\n%s", out)
	}
}

// TestRecoverPanicNoopWhenNil verifies that with no panic, recoverPanic is a
// no-op: it leaves the temp file untouched and does not terminate the process.
func TestRecoverPanicNoopWhenNil(t *testing.T) {
	dir := t.TempDir()
	snap := filepath.Join(dir, "snapshot.json")
	tmp := snapshot.TempFile(snap)
	if err := os.WriteFile(tmp, []byte("partial"), 0o644); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	var release func() error
	func() {
		defer recoverPanic(&tmp, &release)
	}()
	if _, err := os.Stat(tmp); err != nil {
		t.Fatal("temp file should remain when there is no panic")
	}
}
