# S3lytics

A single-user Go web application for analyzing Cubbit DS3 S3-compatible buckets. It provides rich visual reports, deep scanning, incremental scans, bucket comparison, and trend analysis.

## Features

- **Cubbit IAM authentication** — challenge-response (Ed25519) with 2FA support via the Cubbit IAM API
- **Project & bucket browser** — select your Cubbit project and browse its S3 buckets
- **Full & incremental scans** — scan bucket contents with progress tracking and automatic delta detection
- **Parallel scan engine** — prefix-based worker fan-out for faster bucket scanning; configurable workers, batch size, and timeout
- **Deep scan analyzers** — duplicates, multipart uploads, access audit, encryption, versioning, large files, naming compliance, cost estimation, and virus scanning (ClamAV)
- **Rich visual reports** — type/age/storage class breakdowns with Chart.js charts
- **Bucket comparison** — compare two scans side by side
- **Scan history** — browse, filter, and delete past scan results
- **Cubbit design system** — dark-themed UI with Cubbit brand colors and typography

## Prerequisites

- Go 1.26+
- A Cubbit DS3 account with IAM access (coordinator at `api.eu00wi.cubbit.services`)

## Quick Start

```bash
git clone https://github.com/esignoretti/S3lytics.git
cd S3lytics
make build
./build/s3lytics
```

Open http://localhost:8080 and sign in with your Cubbit IAM credentials.

## Usage

1. **Sign in** with your Cubbit email and password (2FA supported)
2. **Select a project** from the dropdown on the Dashboard
3. **Select a bucket** — buckets are loaded automatically for the selected project
4. **Start a scan** — Full Scan (complete listing) or Incremental Scan (delta since last scan)
5. **Wait for completion** — the progress page updates every 2 seconds; redirects to the report when done
6. **Explore the report** — object statistics, type/age/storage breakdowns, and deep scan findings
7. **Compare scans** — select two scan IDs from the history page
8. **Review history** — all scans are persisted in the local BadgerDB database

## Configuration

| Flag | Default | Description |
|---|---|---|
| `--port` | `8080` | HTTP server port |
| `--data` | `~/.s3lytics/data` | BadgerDB data directory |
| `--scan-workers` | `4` | Parallel prefix scanners (1-32) |
| `--scan-batch-size` | `500` | Objects per DB write batch (100-5000) |
| `--scan-prefix-timeout` | `30` | Prefix discovery timeout in seconds |

Scan performance can also be adjusted at runtime via the Settings page.

## Architecture

```
cmd/s3lytics/          — application entry point, wiring, graceful shutdown
internal/
  auth/                — Cubbit IAM challenge-response authentication, session management
  s3/                  — AWS SDK v2 S3 client with custom Cubbit endpoint resolver
  scan/                — scan engine: full/incremental scans, progress tracking, backoff
    deep/              — deep scan analyzers (duplicates, multipart, access, encryption, ...)
  store/               — BadgerDB persistence layer (sessions, accounts, projects, buckets, scans)
  web/                 — HTML templates, static assets, template renderer, HTTP handlers
```

### Auth Flow

1. Challenge request with email → receive salt + challenge
2. Derive Ed25519 key from password + salt → sign challenge
3. Signin with signed challenge → receive JWT + `_refresh` cookie
4. Refresh token via `/iam/v1/auth/refresh/access` with `_refresh` cookie
5. Forge JWT per project IAM user via `/iam/v1/auth/forge/access` with `_refresh` cookie
6. Create/list API keys via `/keyvault/api/v3/keys` with forge JWT as Bearer
7. List buckets via S3 with the API key credentials

## License

GPL-3.0
