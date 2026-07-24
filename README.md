# SiteSnap

SiteSnap is a fast, concurrent website crawler written in Go for deployment verification, website auditing, and snapshot generation.

Unlike traditional crawlers that focus on SEO metrics, SiteSnap answers one simple question:

> **"Did my deployment break the website?"**

---

## Features

- Fast concurrent crawling
- Automatic sitemap discovery
- Sitemap index support
- Compressed (`.xml.gz`) sitemap support
- HTML link extraction
- Internal link discovery
- Parent-child relationship tracking
- Content-Type detection
- Resource type classification
- Path exclusion support
- Redirect handling
- Canonical URL normalization
- JSON snapshot output
- Progress reporting API

---

## Installation

Clone the repository and build the binary:

```bash
git clone https://github.com/RajanCodesDev/sitesnap.git

cd sitesnap

go build ./cmd/sitesnap
```

---

## Usage

Basic crawl:

```bash
sitesnap crawl https://example.com
```

### Common Flags

| Flag | Description |
|------|-------------|
| `--workers 20` | Number of concurrent workers |
| `--timeout 30s` | HTTP request timeout |
| `--exclude /admin` | Exclude a path from crawling |
| `--output report.json` | Save crawl results as JSON |

Example:

```bash
sitesnap crawl https://example.com \
    --workers 20 \
    --exclude /admin \
    --exclude /checkout \
    --output report.json
```

---

## What SiteSnap Crawls

SiteSnap discovers and records:

- HTML pages
- CSS files
- JavaScript
- Images
- Fonts
- PDFs
- Other linked resources

---

## Sitemap Support

SiteSnap automatically:

- Downloads `robots.txt`
- Discovers all `Sitemap:` entries
- Falls back to `/sitemap.xml` when necessary
- Supports sitemap indexes
- Supports compressed (`.xml.gz`) sitemaps

No manual sitemap configuration is required.

---

## Excluding Paths

Exclude one or more paths during crawling:

```bash
sitesnap crawl https://example.com \
    --exclude /admin \
    --exclude /checkout \
    --exclude /private
```

Multiple `--exclude` flags are supported.

---

## Output

The generated JSON snapshot contains metadata for every discovered resource, including:

- URL
- Parent URL
- HTTP Status
- Content Type
- Resource Type

This makes SiteSnap suitable for:

- Deployment verification
- Website inventory
- Snapshot generation
- Automated reporting
- Future deployment comparisons

---

## Typical Workflow

```text
Start URL
    │
    ▼
Download robots.txt
    │
    ▼
Discover sitemaps
    │
    ▼
Queue URLs
    │
    ▼
Concurrent crawling
    │
    ▼
Extract internal links
    │
    ▼
Classify resources
    │
    ▼
Generate JSON snapshot
```

---

## Roadmap

Planned features include:

- HTML reports
- Markdown reports
- CSV export
- Deployment diff reports
- Broken link reports
- GitHub Actions integration
- CI/CD summaries

---

## License

MIT License