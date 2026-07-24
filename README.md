# SiteSnap

SiteSnap is a high-performance website deployment validation tool written in Go.

Instead of checking only whether a website is online, SiteSnap creates a complete snapshot of your website, compares it with previous deployments, and immediately highlights what changed.

Designed for DevOps engineers, SREs, release engineers, and QA teams.

## Features

- Fast concurrent crawler
- Automatic sitemap discovery (`robots.txt`)
- Website snapshot generation
- Deployment comparison
- Detects:
  - Added URLs
  - Removed URLs
  - HTTP status code changes
  - Content-Type changes
- Snapshot validation
- Duplicate URL detection
- CSV report export
- JSON report export
- Atomic snapshot replacement
- Crash-safe snapshot storage
- File locking to prevent concurrent runs
- Pure Go implementation

## Installation

### Debian / Ubuntu

```bash
sudo add-apt-repository ppa:<your-ppa>
sudo apt update
sudo apt install sitesnap
```

### Build from source

```bash
git clone https://github.com/RajanCodesDev/sitesnap.git
cd sitesnap
go build ./cmd/sitesnap
```

## Usage

Create a baseline snapshot:

```bash
sitesnap https://example.com
```

Generate CSV reports:

```bash
sitesnap --csv reports/ https://example.com
```

Generate JSON output:

```bash
sitesnap --json https://example.com
```

Run without replacing the stored snapshot:

```bash
sitesnap --no-replace https://example.com
```

Validate crawl strictly:

```bash
sitesnap --strict https://example.com
```

## Reports

SiteSnap can export:

```
snapshot.csv
added.csv
removed.csv
status_changes.csv
content_type_changes.csv
```

## Why SiteSnap?

Monitoring tells you when a website goes down.

SiteSnap tells you exactly what changed after a deployment.

Typical use cases:

- Deployment verification
- Release validation
- CI/CD pipelines
- Website migrations
- SEO integrity checks
- Large website regression testing

## Example Workflow

```
Previous Snapshot
        │
        ▼
 Crawl Website
        │
        ▼
 Create Snapshot
        │
        ▼
 Validate Snapshot
        │
        ▼
 Compare
        │
        ▼
 Export Reports
        │
        ▼
 Replace Stored Snapshot
```

## License

MIT