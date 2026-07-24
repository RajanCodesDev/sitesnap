package snapshot

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snap.json")

	if Exists(path) {
		t.Fatal("snapshot should not exist yet")
	}

	s := &Snapshot{
		BaseURL:   "https://e.com",
		CrawledAt: time.Now().UTC().Truncate(time.Second),
		Pages: []Page{
			{URL: "https://e.com/", ParentURL: "", StatusCode: 200, ContentType: "text/html", ResourceType: "html"},
		},
	}
	if err := Save(path, s); err != nil {
		t.Fatalf("save: %v", err)
	}
	if !Exists(path) {
		t.Fatal("snapshot should exist after save")
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.BaseURL != s.BaseURL || len(got.Pages) != 1 || got.Pages[0].URL != "https://e.com/" {
		t.Fatalf("round trip mismatch: %+v", got)
	}
}

func TestLoadMissing(t *testing.T) {
	if _, err := Load(filepath.Join(os.TempDir(), "does-not-exist-12345.json")); err == nil {
		t.Fatal("expected error loading missing snapshot")
	}
}

// TestLockExcludesConcurrentProcesses verifies that a second Lock call fails
// with a *LockHeldError while the first holder is active, and that the lock is
// released (and the file removed) when the first holder releases it.
func TestLockExcludesConcurrentProcesses(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snap.json")

	release1, err := Lock(path)
	if err != nil {
		t.Fatalf("first lock: %v", err)
	}
	if _, err := os.Stat(LockFile(path)); err != nil {
		t.Fatalf("lock file should exist while held: %v", err)
	}

	// A second attempt must be rejected.
	if _, err := Lock(path); err == nil {
		t.Fatal("expected second lock to fail")
	} else {
		var held *LockHeldError
		if !errors.As(err, &held) {
			t.Fatalf("expected *LockHeldError, got %T: %v", err, err)
		}
	}

	// After release, a new lock can be acquired and the file is gone.
	if err := release1(); err != nil {
		t.Fatalf("release: %v", err)
	}
	if _, err := os.Stat(LockFile(path)); !os.IsNotExist(err) {
		t.Fatalf("lock file should be removed after release: %v", err)
	}
	if _, err := Lock(path); err != nil {
		t.Fatalf("lock after release should succeed: %v", err)
	}
}

// TestLockReleaseOnPanic verifies the lock is freed even when the caller
// panics, by using a deferred release.
func TestLockReleaseOnPanic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snap.json")

	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("expected panic")
			}
		}()
		release, err := Lock(path)
		if err != nil {
			t.Fatalf("lock: %v", err)
		}
		defer func() { _ = release() }()
		panic("simulated crash")
	}()

	if _, err := os.Stat(LockFile(path)); !os.IsNotExist(err) {
		t.Fatalf("lock file should be removed after panic-time release: %v", err)
	}
}

// TestCleanupTempRemovesStaleFile verifies a leftover .tmp is detected and
// removed, and that a missing temp reports no removal.
func TestCleanupTempRemovesStaleFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snap.json")
	tmp := TempFile(path)

	// No temp yet: nothing removed.
	if _, removed, err := CleanupTemp(path); err != nil || removed {
		t.Fatalf("expected no removal, got removed=%v err=%v", removed, err)
	}

	// Write a stale temp, then clean it up.
	if err := os.WriteFile(tmp, []byte("stale"), 0o644); err != nil {
		t.Fatalf("write stale: %v", err)
	}
	_, removed, err := CleanupTemp(path)
	if err != nil || !removed {
		t.Fatalf("expected removal, got removed=%v err=%v", removed, err)
	}
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Fatalf("stale temp should be gone: %v", err)
	}
}

// TestAtomicReplacePromotesTemp verifies the temp is renamed to the final
// path and removed from its temp location.
func TestAtomicReplacePromotesTemp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snap.json")
	tmp := TempFile(path)

	s := &Snapshot{BaseURL: "https://e.com", Pages: []Page{{URL: "https://e.com/", StatusCode: 200, ContentType: "text/html"}}}
	if err := Save(tmp, s); err != nil {
		t.Fatalf("save temp: %v", err)
	}
	if err := AtomicReplace(tmp, path); err != nil {
		t.Fatalf("atomic replace: %v", err)
	}
	if !Exists(path) {
		t.Fatal("final snapshot should exist after replace")
	}
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Fatalf("temp should be gone after replace: %v", err)
	}
	got, err := Load(path)
	if err != nil || len(got.Pages) != 1 {
		t.Fatalf("loaded replaced snapshot mismatch: %v %+v", err, got)
	}
}

// TestAtomicReplacePreservesExistingOnFailure verifies that if the temp file
// is missing, AtomicReplace errors and the existing snapshot is untouched.
func TestAtomicReplacePreservesExistingOnFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snap.json")
	tmp := TempFile(path)

	s := &Snapshot{BaseURL: "https://e.com", Pages: []Page{{URL: "https://e.com/", StatusCode: 200, ContentType: "text/html"}}}
	if err := Save(path, s); err != nil {
		t.Fatalf("save: %v", err)
	}

	// tmp does not exist -> error, and the existing snapshot must remain.
	if err := AtomicReplace(tmp, path); err == nil {
		t.Fatal("expected error when temp missing")
	}
	if !Exists(path) {
		t.Fatal("existing snapshot must be preserved on failed replace")
	}
}

// TestLockRewritesPID verifies the lock file never retains stale bytes from a
// previous (longer) PID. We simulate a stale longer PID, then acquire the lock
// and confirm the file contains exactly the new PID.
func TestLockRewritesPID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snap.json")
	lockPath := LockFile(path)

	// Pre-seed the lock file with a longer "stale" PID and hold the flock so
	// Lock must rewrite (not append) the new, shorter PID.
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatalf("open lock: %v", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		t.Fatalf("flock: %v", err)
	}
	if _, err := f.WriteString("123456"); err != nil {
		_ = f.Close()
		t.Fatalf("seed pid: %v", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		t.Fatalf("sync: %v", err)
	}

	// Acquire via the package API (it will rewrite the PID). We must release
	// our manual flock first so Lock can take it.
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_UN); err != nil {
		_ = f.Close()
		t.Fatalf("unflock: %v", err)
	}
	_ = f.Close()

	release, err := Lock(path)
	if err != nil {
		t.Fatalf("lock: %v", err)
	}
	defer func() { _ = release() }()

	data, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("read lock: %v", err)
	}
	got := strings.TrimSpace(string(data))
	if got != strconv.Itoa(os.Getpid()) {
		t.Fatalf("lock file should contain only current PID %d, got %q", os.Getpid(), got)
	}
	if len(got) > len(strconv.Itoa(os.Getpid())) {
		t.Fatalf("lock file contains stale bytes: %q", got)
	}
}
