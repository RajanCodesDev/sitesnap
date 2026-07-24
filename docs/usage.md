# SiteSnap

> **Deploy with confidence.**
>
> SiteSnap is a fast, concurrent website crawler that creates deterministic snapshots of your website and compares them across deployments, helping you detect regressions before your users do.

---

## Why SiteSnap?

Most uptime monitors only tell you **if** your website is online.

SiteSnap tells you **what changed**.

After every deployment, SiteSnap crawls your website, compares it against the previous snapshot, and reports:

* ✅ Added pages
* ❌ Removed pages
* 🔄 HTTP status code changes (e.g. `200 → 404`)
* 📄 Content-Type changes
* 🔍 Duplicate URL variants
* ⚠️ Crawl-quality issues

Perfect for deployment verification, CI/CD pipelines, staging validation, and production monitoring.

---

## Features

* ⚡ Fast concurrent crawler
* 📸 Deterministic website snapshots
* 🔍 Automatic deployment comparison
* 🚨 Duplicate URL detection
* 📊 Human-readable and JSON reports
* 🛡 Snapshot validation
* 🔒 Atomic snapshot replacement
* 🚀 CI/CD friendly
* 🧹 Automatic crash recovery
* 💾 Single static Go binary
* 📦 Zero external dependencies (Go standard library only)

---

## Installation

Clone the repository and build SiteSnap:

```bash
git clone https://github.com/RajanCodesDev/sitesnap.git

cd sitesnap

go build -o sitesnap .
```

Or install directly (once Go module support is published):

```bash
go install github.com/RajanCodesDev/sitesnap@latest
```

---

## Quick Start

First crawl creates the baseline snapshot.

```bash
sitesnap https://example.com
```

Subsequent crawls automatically compare against the stored snapshot.

```bash
sitesnap https://example.com
```

Example output:

```text
Crawled 121 URLs in 4.2s

Snapshot Validation

Warnings: 2
Errors: 0

✓ Snapshot validation passed.

=== SiteSnap Deployment Report ===

Added URLs: 1
Removed URLs: 0
Status Code Changes: 2
Content-Type Changes: 0

Stored snapshot replaced at snapshot.json.
```

---

## Common Use Cases

### Deployment Verification

Run SiteSnap immediately after every deployment to detect unexpected changes before users do.

```bash
sitesnap https://staging.example.com
```

---

### CI/CD Quality Gate

Fail your deployment if validation detects warnings.

```bash
sitesnap -strict -json https://staging.example.com
```

---

### Snapshot Auditing

Compare against the current baseline without replacing it.

```bash
sitesnap -no-replace https://example.com
```

---

### Website Quality Checks

SiteSnap also detects issues such as:

* Mixed trailing slash usage
* Duplicate URL variants
* Broken internal links
* Crawl-quality problems
* Missing pages

---

## How It Works

```text
Deploy Website
        │
        ▼
 Crawl Website
        │
        ▼
Validate Snapshot
        │
        ▼
Compare With Previous Snapshot
        │
        ▼
Generate Deployment Report
        │
        ▼
Update Baseline Snapshot
```

The first crawl creates a baseline.

Every future crawl automatically compares against that baseline.

---

## Documentation

Detailed documentation is available in the `docs` directory.

* **USAGE.md** — Installation, CLI flags, examples, validation, JSON output, CI/CD usage
* **ARCHITECTURE.md** — Internal design, crawler architecture, snapshots, locking, validation, concurrency, and implementation details

---

## Example Scenarios

### A deployment accidentally removed several pages

```text
Removed URLs

https://example.com/blog/post-1
https://example.com/blog/post-2
```

---

### A page now returns 404

```text
Status Code Changes

https://example.com/contact

200 → 404
```

---

### Duplicate URLs detected

```text
Warnings

Duplicate URL Variant

https://example.com/about
https://example.com/about/
```

---

## Why SiteSnap?

SiteSnap is intentionally focused.

It is **not**:

* an SEO crawler
* a vulnerability scanner
* a website monitoring platform
* a broken-link checker

It is a deployment verification tool designed to answer one question:

> **"What changed after this deployment?"**

---

## Roadmap

Future releases may include:

* HTML reports
* GitHub Action
* Configuration files
* Sitemap seeding
* Ignore patterns
* Historical snapshots

The core architecture is considered stable. Future releases will focus on additional features rather than redesigning existing functionality.

---

## Contributing

Bug reports, ideas, and pull requests are welcome.

If you encounter an unexpected internal error, please include:

* SiteSnap version
* Operating system
* Command executed
* Complete output
* Stack trace (when using `SITESNAP_DEBUG=1`)

---

## License

This project is licensed under the MIT License.
