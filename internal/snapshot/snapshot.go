// Package snapshot defines the crawl data model and its JSON persistence.
//
// A Snapshot is the complete record of one crawl: every discovered URL with
// its parent, HTTP status code, content type, and (optionally) resource type.
package snapshot

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"syscall"
	"time"
)

// Page is the data collected for a single discovered URL.
type Page struct {
	URL          string `json:"url"`
	ParentURL    string `json:"parent_url"`
	StatusCode   int    `json:"status_code"`
	ContentType  string `json:"content_type"`
	ResourceType string `json:"resource_type,omitempty"`
}

// Snapshot is the full record of one crawl of a website.
type Snapshot struct {
	BaseURL   string    `json:"base_url"`
	CrawledAt time.Time `json:"crawled_at"`
	Pages     []Page    `json:"pages"`
}

// Save writes the snapshot to path as indented JSON, replacing any existing
// file. Used both for the initial baseline and for replacing the stored
// snapshot after a successful comparison.
//
// Pages are sorted by URL (then parent URL) before serialization so that two
// crawls of an unchanged site produce byte-identical output. This determinism
// is what makes deployment diffing meaningful.
func Save(path string, s *Snapshot) error {
	out := *s
	if len(s.Pages) > 0 {
		pages := make([]Page, len(s.Pages))
		copy(pages, s.Pages)
		sort.Slice(pages, func(i, j int) bool {
			if pages[i].URL != pages[j].URL {
				return pages[i].URL < pages[j].URL
			}
			return pages[i].ParentURL < pages[j].ParentURL
		})
		out.Pages = pages
	}
	data, err := json.MarshalIndent(&out, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// Load reads a snapshot previously written with Save.
func Load(path string) (*Snapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s Snapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Exists reports whether a snapshot file is already stored at path.
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// LockFile returns the conventional lock-file path for a snapshot path.
// e.g. "snapshot.json" -> "snapshot.json.lock".
func LockFile(path string) string {
	return path + ".lock"
}

// TempFile returns the conventional temporary file path for a snapshot path.
// e.g. "snapshot.json" -> "snapshot.json.tmp".
func TempFile(path string) string {
	return path + ".tmp"
}

// Lock acquires an exclusive advisory lock for operating on the snapshot at
// path. Only one SiteSnap process may hold the lock at a time.
//
// On success it returns a release function that must be called (typically via
// defer) to free the lock and remove the lock file. If another instance
// already holds the lock, the returned error is of type *LockHeldError and the
// caller should exit gracefully without touching the snapshot.
func Lock(path string) (release func() error, err error) {
	lockPath := LockFile(path)
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("failed to open lock file: %w", err)
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if err == syscall.EWOULDBLOCK {
			return nil, &LockHeldError{Path: lockPath}
		}
		return nil, fmt.Errorf("failed to acquire lock: %w", err)
	}

	// Record the owning PID so a stale lock can be diagnosed. Seek to the
	// start and truncate first so a shorter new PID never leaves stale bytes
	// from a previous (longer) PID in the file (e.g. "8912" not "891256").
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
		return nil, fmt.Errorf("failed to seek lock file: %w", err)
	}
	if err := f.Truncate(0); err != nil {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
		return nil, fmt.Errorf("failed to truncate lock file: %w", err)
	}
	if _, err := f.WriteString(strconv.Itoa(os.Getpid())); err != nil {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
		return nil, fmt.Errorf("failed to record lock owner: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
		return nil, fmt.Errorf("failed to flush lock file: %w", err)
	}

	release = func() error {
		unErr := syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		closeErr := f.Close()
		removeErr := os.Remove(lockPath)
		if unErr != nil {
			return unErr
		}
		if closeErr != nil {
			return closeErr
		}
		return removeErr
	}
	return release, nil
}

// LockHeldError indicates another SiteSnap instance already holds the lock.
type LockHeldError struct {
	Path string
}

func (e *LockHeldError) Error() string {
	return fmt.Sprintf("another sitesnap process holds the lock at %s", e.Path)
}

// CleanupTemp removes a stale temporary snapshot left behind by a previous
// crashed or interrupted run. It returns the path that was removed (empty if
// none) and whether a file was removed. A temp file is only ever a leftover
// artifact: the canonical snapshot is always published via AtomicReplace, so
// removing it is always safe.
func CleanupTemp(path string) (removedPath string, removed bool, err error) {
	tmp := TempFile(path)
	if !Exists(tmp) {
		return "", false, nil
	}
	if err := os.Remove(tmp); err != nil {
		return "", false, fmt.Errorf("failed to remove stale temp file %s: %w", tmp, err)
	}
	return tmp, true, nil
}

// AtomicReplace durably promotes a temporary snapshot file to the canonical
// snapshot path. It flushes the temporary file (and its directory) to disk
// with fsync before performing an atomic rename, minimizing the chance of data
// loss on unexpected power failure.
func AtomicReplace(tmpPath, finalPath string) error {
	f, err := os.OpenFile(tmpPath, os.O_RDWR, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open temp snapshot: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("failed to flush temp snapshot: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("failed to close temp snapshot: %w", err)
	}

	if err := os.Rename(tmpPath, finalPath); err != nil {
		return fmt.Errorf("failed to rename temp snapshot: %w", err)
	}

	// Best-effort fsync of the directory so the rename itself is durable.
	if dir, err := os.Open(filepath.Dir(finalPath)); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}
