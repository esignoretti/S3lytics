# S3lytics — Design Document

**Date**: 2026-05-21
**Status**: Draft

## 1. Overview

S3lytics is a single-user Go web application that analyzes Cubbit S3 buckets and generates rich visual reports. It supports basic bucket statistics, deep scanning features, historical storage, bucket comparison, and trend analysis. Scans are incremental — rescans of the same bucket only process differences (new, modified, deleted objects).

### Tech Stack

| Component | Choice | Rationale |
|---|---|---|
| Language | Go | As requested |
| HTTP router | `chi` | Lightweight, idiomatic Go |
| Frontend | `html/template` + HTMX + Chart.js | No build step, interactive, good graphics |
| S3 client | `aws-sdk-go-v2` | Official AWS SDK with custom endpoint support |
| Local DB | BadgerDB | Embedded, fast key-value, no server needed |
| Virus scan | ClamAV (clamd socket) | Optional, filters for ext/size/date/count |
| Crypto | `golang.org/x/crypto/ed25519` | Curve25519 for Cubbit IAM auth |

## 2. Architecture

```
┌──────────────────────────────────────────────────────┐
│  S3lytics (single Go binary)                         │
│                                                      │
│  ┌────────────────────────────────────────────────┐  │
│  │  HTTP Server (chi)                              │  │
│  │  ┌──────────┐ ┌──────────┐ ┌────────────────┐  │  │
│  │  │ Static   │ │ Templates│ │ API Handlers   │  │  │
│  │  │ (/assets)│ │ (/views) │ │ (/api/...)     │  │  │
│  │  └──────────┘ └──────────┘ └───────┬────────┘  │  │
│  └─────────────────────────────────────┼──────────┘  │
│                                        │             │
│  ┌─────────────────────────────────────▼──────────┐  │
│  │  Services Layer                               │  │
│  │  ┌──────────┐ ┌─────────┐ ┌────────────────┐  │  │
│  │  │ Auth     │ │ Scan    │ │ Object Store   │  │  │
│  │  │ (Cubbit  │ │ Engine  │ │ (local cache   │  │  │
│  │  │  IAM Go) │ │         │ │  of listing)   │  │  │
│  │  └──────────┘ └────┬────┘ └────────────────┘  │  │
│  └─────────────────────┼────────────────────────┘  │
│                        │                           │
│  ┌─────────────────────▼────────────────────────┐  │
│  │  BadgerDB                                    │  │
│  │  scans / objects / settings / auth           │  │
│  └──────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────┘
```

### Component Responsibilities

- **Auth Service**: Reimplements Cubbit IAM challenge-response protocol (Curve25519 signing, JWT management, token refresh, API key management)
- **Scan Engine**: Goroutine-based scanner that lists S3 objects, computes diffs for incremental scans, runs analyzers
- **Object Store**: BadgerDB layer for persisting object listings, scan results, settings
- **Web UI**: Go templates rendered server-side with HTMX for dynamic updates

## 3. Authentication Flow (Cubbit IAM Reimplementation)

Protocol reverse-engineered from `DS3Lib/Sources/DS3Lib`:

```
1. POST {iam_url}/challenge
   Body: {email, tenant_id?}
   Response: {salt, challenge}

2. key = SHA256(password_bytes + salt_bytes)  → seed for Ed25519
   private_key = Ed25519.NewKeyFromSeed(seed)
   signature = private_key.Sign(challenge_string)
   signed_challenge = base64(signature)

3. POST {iam_url}/signin
   Body: {email, signed_challenge, tfa_code?, tenant_id?}
   Response: JWT token (body) + _refresh cookie (Set-Cookie)
   → Creates AccountSession{token, refreshToken}

4. GET {iam_url}/accounts/me
   Header: Authorization: Bearer {jwt}
   Response: Account{endpoint_gateway, ...}

5. GET {iam_url}/projects
   Header: Authorization: Bearer {jwt}
   Response: [Project{id, name, ...}]

6. GET {iam_url}/forge-jwt?user_id={id}
   Header: Cookie: _refresh={refreshToken}
   Response: IAM-scoped JWT token

7. POST {iam_url}/keys/{name}?user_id={id}
   Header: Authorization: Bearer {iam_jwt}
   Response: DS3ApiKey{api_key, secret_key}
```

**Token lifecycle**:
- JWT access tokens are proactively refreshed when expiring within 5 minutes
- Refresh tokens are stored in the session and used to obtain new JWTs
- If refresh fails, user is redirected to login page
- Session is persisted to BadgerDB (encrypted) for reload between app restarts

**Login page fields**:
- Email (required)
- Password (required)
- 2FA code (optional)
- Tenant ID (optional)
- API server URL (optional, defaults to Cubbit IAM production URL)

## 4. S3 Operations

Using `aws-sdk-go-v2` with `s3.NewFromConfig` and a custom endpoint resolver:

- `ListObjectsV2` with pagination (continuation token)
- `ListMultipartUploads` for stale upload detection
- `GetBucketPolicy`, `GetBucketAcl`, `GetPublicAccessBlock` for access audit
- `GetBucketEncryption` for encryption audit
- `ListObjectVersions` for versioning waste analysis
- `HeadObject` only for new/modified objects during incremental scans

## 5. BadgerDB Schema

Key prefixes, values are protobuf or msgpack-encoded:

```
# Auth
auth/session           → encrypted {jwt, refresh_token, expires_at}
auth/account           → {endpoint_gateway, email, ...}

# Projects & buckets (cached)
projects               → JSON array of projects
buckets/{project_id}   → JSON array of buckets

# Object listing (for incremental scans)
objects/{bucket}/{encoded_key} → {etag, size, last_modified, storage_class, scan_id}

# Scan results
scans/{id}             → {bucket, project, timestamp, duration, status, scan_type}
scans/{id}/summary     → {total_objects, total_size, avg_size, median_size, max_size, ...}
scans/{id}/types       → [{ext, count, total_size, percentage}]
scans/{id}/ages        → [{bucket:"<24h", count, size}, ...]
scans/{id}/storage     → [{class:"STANDARD", count, size}, ...]
scans/{id}/prefixes    → [{prefix, count, size}]
scans/{id}/delta       → {new: N, modified: N, deleted: N, unchanged: N}  (for incremental)

# Deep scan results
scans/{id}/deep_duplicates     → [{etag, count, total_size, keys[]}]
scans/{id}/deep_multiparts     → [{upload_id, key, initiated, size}]
scans/{id}/deep_access_audit   → {public_access_blocked, bucket_policy, findings[]}
scans/{id}/deep_encryption     → {encrypted_pct, algorithms[], unencrypted_keys[]}
scans/{id}/deep_versioning     → {total_versions, non_current_count, wasted_bytes}
scans/{id}/deep_large_files    → [{key, size, last_modified}]
scans/{id}/deep_naming         → {pattern, non_compliant_count, examples[]}
scans/{id}/deep_cost_estimate  → {monthly_cost, breakdown_by_class[]}

# Virus scan
scans/{id}/virus_results       → {status, scanned, infected[], errors[]}

# Bucket scan index
bucket/{name}/scans            → ordered list of scan IDs (for trends)
```

## 6. Basic Scan Report Metrics

| Metric | Source | Visualization |
|---|---|---|
| Total objects count | S3 listing | Summary card |
| Total bucket size | S3 listing | Summary card |
| Average / median / max size | S3 listing | Summary cards |
| File type distribution | Extension from object key | Pie chart + table |
| Last modified distribution | LastModified field | Bar chart |
| Storage class breakdown | StorageClass field | Bar chart |
| Top-level prefix sizes | Prefix extraction | Table + treemap |
| Empty objects (size=0) | Size field | Count + list |
| Largest objects | Size field | Table |
| Scan duration, object count/sec | Timestamps | Summary card |
| Delta report (incremental only) | Diff against stored listing | New/modified/deleted counts |

## 7. Deep Scan Features

All deep scan features run after the basic scan completescars:

### Duplicate Detection
- Group objects by ETag
- Find ETags appearing more than once
- Report: total wasted bytes, groups listing

### Incomplete Multipart Uploads
- `ListMultipartUploads` API call
- Report: stale uploads with age, size, object key

### Access / Security Audit
- `GetBucketPolicy` → parsed statements
- `GetBucketAcl` → grants
- `GetPublicAccessBlock` → block status
- Report: findings with severity (public access, cross-account grants, etc.)

### Encryption Audit
- `GetBucketEncryption` → default encryption
- For new/modified objects: `HeadObject` to check SSE status
- Report: % encrypted, unencrypted samples

### Versioning Waste
- `ListObjectVersions` (if versioning enabled)
- Count non-current versions and calculate wasted space
- Report: wasted bytes, version count breakdown

### Large File Heatmap
- Configurable size threshold (default: 100 MB)
- Sorted by size descending
- Report: top N largest files table

### Naming Convention Check
- Configurable regex pattern (default: none)
- Flag non-compliant keys
- Report: count, example violations

### Cost Estimation
- Lookup table of $/GB/month per storage class
- Report: total monthly cost, breakdown by class

### Virus Scan (ClamAV)
- Optional, disabled by default
- Filters: file extensions, max size, last modified range, max count
- Streams selected objects through `clamd` TCP/socket
- Report: scanned count, infected files, errors

## 8. Incremental Scan Design

**First scan** (full):
1. List all objects via paginated `ListObjectsV2`
2. For each object: store `{key, etag, size, last_modified, storage_class}` in BadgerDB under `objects/{bucket}/{key}`
3. Compute all stats from the full listing
4. Store scan result

**Subsequent scans** (incremental):
1. Fresh `ListObjectsV2` (required to know current state)
2. For each listed object:
   - Not in DB → new → process into stats
   - In DB, same etag → unchanged → skip, carry forward previous stats
   - In DB, different etag → modified → reprocess (new size, type, etc.)
3. After full listing, find DB objects not in fresh listing → deleted → subtract from stats
4. Compute final stats: previous_stats + new + modified - deleted
5. Update object store with current state
6. Store delta report: `{new: N, modified: N, deleted: N, unchanged: N}`

**Trade-offs**:
- Pro: Subsequent scans are much faster (no HeadObject calls for unchanged objects)
- Pro: Delta reports showing what actually changed
- Pro: Lower S3 request costs
- Con: BadgerDB stores full object listing (~100 bytes/object)
- Con: Cannot avoid `ListObjectsV2` cost (full listing required anyway)
- Con: DB corruption means next scan must be full

## 9. Web UI — Pages and Layout

### Navigation (sidebar)
- Dashboard
- Scan History
- Settings

### Pages

**Login**: `/login`
- Form: email, password, 2FA code, tenant, API server URL
- Submit → authenticate → redirect to dashboard

**Dashboard**: `/`
- Project selector (dropdown)
- Bucket selector (populated after project selection)
- "Start New Scan" button (with deep scan toggles and virus scan filters)
- "View History" button for selected bucket
- "Compare" button to pick two past scans

**Scan Report**: `/scan/{id}`
- Summary cards row
- Basic sections (types, ages, storage classes, prefixes)
- Deep scan sections (collapsible accordion)
- Virus scan section
- "Download Report" button (PDF or HTML export)
- "Delete This Scan" button

**Scan In Progress**: `/scan/{id}/progress`
- Polled via HTMX every 2s
- Shows progress bar, objects processed so far, elapsed time
- On completion, auto-redirects to report page

**History**: `/history?bucket={name}`
- Table of all scans for this bucket
- Checkboxes to select two for comparison
- "Compare Selected" button

**Comparison**: `/compare?scan_a={id}&scan_b={id}`
- Side-by-side summary cards with delta↑/↓
- Side-by-side breakdown tables
- If same bucket (3+ scans): trend time-series charts
  - Total objects over time (line)
  - Total size over time (line)
  - File type drift (stacked area)

**Settings**: `/settings`
- ClamAV socket path (default: `/var/run/clamav/clamd.sock`)
- Deep scan default toggles
- Naming convention regex pattern
- Large file threshold (MB)
- Cost estimation $/GB/month per storage class

## 10. Error Handling & Resilience

- **Partial scans**: Checkpoint saved every N objects to BadgerDB. Resume from last continuation token.
- **S3 throttling**: Exponential backoff (100ms, 500ms, 1s, 5s) on SlowDown/rate limit errors
- **Session expiry**: 5-minute threshold auto-refresh. If refresh fails → session deleted → redirect to login
- **BadgerDB corruption**: On open failure, fall back to in-memory-only mode; warn user
- **Concurrent scans**: One scan at a time per bucket; queue additional scan requests
- **Graceful shutdown**: Wait for running scan to checkpoint before exit
- **File system**: Embedded `embed.FS` for all templates and static assets — single binary deployment

## 11. Future Considerations

- Multi-user server with own auth (password hashing, session management)
- Scheduled scans (cron-like, run automatically)
- Alerting (email/webhook on findings like public bucket, virus found)
- Export to PDF
- CLI-only mode for headless operation
- Distributed scan workers (for very large buckets)

## 12. Non-Goals (v1)

- Real-time scan (always requires listing first)
- S3 write operations (except the read-only analysis)
- Multi-region / cross-account support
- Mobile-responsive UI (desktop-first)
