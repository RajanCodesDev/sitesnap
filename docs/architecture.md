# SiteSnap Architecture

This document describes the internal architecture of SiteSnap, including the crawler, snapshot pipeline, concurrency model, validation system, and design decisions.

---

# Design Goals

SiteSnap was designed around a few core principles:

* Fast concurrent crawling
* Deterministic snapshots
* Safe snapshot replacement
* Minimal dependencies
* Predictable behavior
* CI/CD friendly operation
* Simple architecture

The project intentionally relies only on Go's standard library wherever possible.

---

# High-Level Architecture

```text
                User
                  │
                  ▼
          Command Line Interface
                  │
                  ▼
            Configuration
                  │
                  ▼
        Snapshot Coordinator
                  │
      ┌───────────┴───────────┐
      ▼                       ▼
 Website Crawler        Snapshot Loader
      │                       │
      ▼                       ▼
URL Collection        Previous Snapshot
      │                       │
      └───────────┬───────────┘
                  ▼
            Comparator
                  │
                  ▼
          Validation Engine
                  │
                  ▼
             Report Builder
                  │
                  ▼
        Atomic Snapshot Writer
```

Each stage performs one responsibility, making the system easy to maintain and extend.

---

# Crawl Pipeline

Every execution follows the same sequence.

```text
Start

↓

Load configuration

↓

Acquire snapshot lock

↓

Load previous snapshot

↓

Concurrent crawl

↓

Normalize URLs

↓

Build snapshot

↓

Validate snapshot

↓

Compare snapshots

↓

Generate report

↓

Atomically replace snapshot

↓

Release lock
```

---

# Website Crawler

The crawler is responsible for discovering pages and collecting metadata.

Each crawled page records:

* URL
* HTTP status code
* Content-Type
* Internal links

Only internal links are followed.

External resources are ignored.

---

# Worker Pool

SiteSnap uses a fixed-size worker pool.

```text
          URL Queue

             │

 ┌───────────┼───────────┐

 ▼           ▼           ▼

Worker     Worker     Worker

 ▼           ▼           ▼

HTTP GET   HTTP GET   HTTP GET

 ▼           ▼           ▼

 Extract     Extract     Extract

 ▼           ▼           ▼

New URLs → Queue
```

Benefits:

* Controlled concurrency
* Low memory usage
* High throughput
* Predictable CPU utilization

The number of workers is configurable using the `-workers` flag.

---

# URL Normalization

Before storage or comparison, URLs are normalized.

Typical normalization includes:

* Removing fragments (`#section`)
* Resolving relative links
* Removing duplicate paths
* Canonicalizing URLs
* Consistent trailing-slash handling

Normalization ensures that equivalent URLs are compared consistently.

---

# Snapshot Format

A snapshot represents the current state of the website.

Each page contains metadata similar to:

```text
URL
Status Code
Content-Type
Outgoing Links
```

Snapshots are serialized to JSON for portability and easy inspection.

---

# Snapshot Lifecycle

## First Crawl

```text
Website

↓

Crawl

↓

Snapshot

↓

Validate

↓

Store snapshot.json
```

No comparison occurs.

---

## Later Crawls

```text
Website

↓

Crawl

↓

Validate

↓

Load previous snapshot

↓

Compare

↓

Report

↓

Replace snapshot
```

Each deployment is compared against the previous baseline.

---

# Comparison Engine

The comparison engine performs set-based comparisons between snapshots.

It detects:

* Added pages
* Removed pages
* HTTP status changes
* Content-Type changes

This allows SiteSnap to identify deployment regressions quickly.

---

# Validation Pipeline

Before a snapshot is accepted, multiple validation passes are executed.

Examples include:

* Invalid URLs
* Missing status codes
* Empty content types
* Duplicate URL variants
* Unexpected page-count drops
* Suspicious crawl results

Validation prevents corrupt or misleading snapshots from becoming the new baseline.

---

# Duplicate URL Detection

Multiple URLs may represent the same resource.

Example:

```text
/about
/about/
```

These are grouped into duplicate sets.

The report includes:

* Canonical URL
* Duplicate variants
* Discovery locations

This helps identify routing inconsistencies and SEO issues.

---

# Concurrency Model

SiteSnap separates crawling from reporting.

```text
Workers

↓

Collect Pages

↓

Coordinator

↓

Snapshot

↓

Validation

↓

Comparison

↓

Output
```

Workers never modify shared report structures directly.

Instead, collected data is synchronized through the coordinator.

This minimizes contention and simplifies synchronization.

---

# Snapshot Locking

Only one SiteSnap instance may update a snapshot at a time.

A lock file is created before snapshot operations begin.

```text
Acquire Lock

↓

Read Snapshot

↓

Compare

↓

Write Temporary Snapshot

↓

Atomic Rename

↓

Release Lock
```

This prevents concurrent executions from corrupting stored snapshots.

---

# Atomic Snapshot Replacement

Snapshots are never written directly over the existing file.

Instead:

```text
snapshot.tmp

↓

Write

↓

Validate

↓

Rename

↓

snapshot.json
```

Using an atomic rename ensures that the snapshot is always either:

* the old valid version, or
* the new valid version.

Partial writes cannot leave the snapshot corrupted.

---

# Error Handling

Operational errors are returned normally.

Unexpected internal panics are recovered gracefully.

Default behavior:

* Print a concise error message
* Exit with status code `1`

When debugging:

```bash
SITESNAP_DEBUG=1 sitesnap https://example.com
```

the original panic and stack trace are displayed.

---

# Performance Considerations

Performance depends primarily on:

* Website latency
* Number of pages
* Worker count
* Server response time

The crawler is I/O-bound rather than CPU-bound for most workloads.

Increasing workers generally improves throughput until network latency or the target server becomes the bottleneck.

---

# Design Decisions

## Why JSON?

* Human-readable
* Easy to inspect
* Easy to diff
* Portable
* Standard tooling support

---

## Why a Worker Pool?

A fixed worker pool provides:

* Stable resource usage
* Predictable scheduling
* Better scalability than creating a goroutine per request

---

## Why Atomic Renames?

Atomic replacement guarantees that a snapshot is never left in a partially written state, even if the process exits unexpectedly.

---

## Why Validate Before Saving?

An invalid snapshot should never become the new baseline.

Validation ensures future comparisons remain trustworthy.

---

## Why Standard Library?

Using the Go standard library keeps SiteSnap:

* Lightweight
* Portable
* Easy to audit
* Easy to build
* Free from unnecessary runtime dependencies

---

# Extending SiteSnap

The architecture is intentionally modular.

Future features can be added with minimal impact to existing components, such as:

* HTML reports
* Historical snapshots
* Ignore rules
* Sitemap support
* GitHub Action integration
* Configuration files
* Additional validation rules

---

# Project Structure

```text
cmd/
internal/
    crawler/
    compare/
    snapshot/
    validate/
    report/
    lock/
    output/

docs/

README.md
```

Each package has a single, well-defined responsibility, keeping the codebase maintainable and easy to navigate.

---

# Summary

SiteSnap is built around a straightforward pipeline:

```text
Website
    │
    ▼
Concurrent Crawl
    │
    ▼
Snapshot
    │
    ▼
Validation
    │
    ▼
Comparison
    │
    ▼
Report
    │
    ▼
Atomic Snapshot Storage
```

By combining deterministic crawling, snapshot comparison, validation, and safe persistence, SiteSnap provides a reliable way to detect website regressions after every deployment.
