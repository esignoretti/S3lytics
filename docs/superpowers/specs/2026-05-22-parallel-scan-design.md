# Parallel Scan Engine — Design Spec

**Date:** 2026-05-22
**Status:** Draft
**Project:** S3lytics

## 1. Problem

The current scan pipeline is single-threaded: one goroutine paginates through `ListObjectsV2`, processes each object one at a time, and writes each object to BadgerDB via an individual `SaveObject` call. For large buckets (100K+ objects) this produces two bottlenecks:

1. **Sequential S3 pagination** — each page must finish before the next can start, wasting round-trip latency.
2. **Per-object BadgerDB writes** — 100K objects = 100K individual transactions. Badger is 10-100x more efficient with batched writes.

Additionally, the scan is entirely sequential across prefixes — if a bucket has directories `logs/`, `media/`, `backups/`, they are listed one after the other even though S3 allows independent continuation tokens per prefix.

## 2. Design

Replace the single-goroutine scan with a three-stage pipeline:

```
                    ┌─────────────┐
                    │ Dispatcher  │  discovers prefixes via delimiter="/"
                    └──────┬──────┘
                           │ prefix queue (buffered chan)
            ┌──────────────┼──────────────┐
            ▼              ▼              ▼
       ┌─────────┐   ┌─────────┐   ┌─────────┐
       │Worker 1 │   │Worker 2 │   │Worker N │  each goroutine = one prefix
       │ (page)  │   │ (page)  │   │ (page)  │  own pagination loop
       └────┬────┘   └────┬────┘   └────┬────┘
            │             │             │
            └──────────────┼──────────────┘
                           ▼
                    ┌─────────────┐
                    │ Collector   │  merges stats, batches DB writes
                    └─────────────┘
```

The **Dispatcher** runs one extra S3 API call to discover top-level prefixes, then fans them out to **Workers**. Each Worker does its own full pagination of its prefix. Both Workers and Dispatcher send objects to a shared **Collector** channel. The Collector accumulates statistics in memory and writes objects to BadgerDB in batches of `ScanBatchSize`.

If the prefix discovery returns no prefixes (flat bucket), a single synthetic prefix `""` is created and one worker handles everything — the system degrades gracefully to essentially sequential behavior, but still benefits from the write batching the Collector provides.

## 3. Components

### 3.1 Scan Config (`scan.Config`)

```go
type Config struct {
    Workers            int           // parallel prefix scanners (1-32, default 4)
    BatchSize          int           // objects per DB write batch (100-5000, default 500)
    PrefixTimeout      time.Duration // timeout for prefix discovery (default 30s)
}
```

The `Engine` struct gains a mutable `config` field (initially set from CLI defaults, zero-value uses sane defaults). The handler calls `engine.SetConfig(cfg)` after loading saved settings on startup, and again whenever settings are updated via the POST handler. If `SetConfig` is never called, the engine uses defaults that match CLI flags.

```go
func (e *Engine) SetConfig(cfg Config) {
    e.mu.Lock()
    defer e.mu.Unlock()
    e.config = cfg
}
```

### 3.2 S3 Client — `ListPrefixes`

New method on the `S3Client` interface:

```go
ListPrefixes(ctx context.Context, bucket, prefix string) ([]string, error)
```

Implementation calls `ListObjectsV2` with `Delimiter: "/"` and `Prefix: prefix`, extracts `CommonPrefixes`. This is used only for the prefix discovery pass.

### 3.3 Dispatcher

- Called once per scan, before workers start
- Calls `s3Client.ListPrefixes(ctx, bucket, "")`
- If zero prefixes returned, queues a single `""` prefix
- If prefixes exceed `Workers`, queues first `Workers` prefixes; remaining are queued as workers become free (channel backpressure)
- On timeout (`PrefixTimeout`), cancels and falls back to a single `""` prefix (degraded mode — still benefits from write batching)
- Closes the prefix channel when done, signalling workers to exit

### 3.4 Worker

- Each worker goroutine receives a single prefix string
- Runs the standard pagination loop using `ListObjectsPage` with `Prefix: prefix`
- Page prefetch: while processing page N's objects, the next page fetch is already in flight (goroutine per page)
- Sends each parsed `ObjectRecord` to the collector channel
- On S3 error: sends an error signal to the collector, which cancels the root context (all workers shut down)
- On context cancellation: exits immediately

**Page prefetch implementation:**

```go
// worker goroutine
type pageResult struct {
    page *s3.ListResult
    err  error
}

var nextPage <-chan pageResult  // nil on first iteration
var cancel context.CancelFunc   // shared cancel from dispatcher
for {
    select {
    case result := <-nextPage:
        if result.err != nil {
            objChan <- collectorMsg{err: result.err}
            return
        }
        // process result.page objects, send each to objChan
    case <-ctx.Done():
        return
    }
    // kick off next page fetch if truncated
    if !isTruncated {
        break
    }
    ch := make(chan pageResult, 1)
    nextPage = ch
    go func(token *string) {
        page, err := e.client.ListObjectsPage(ctx, bucket, token)
        ch <- pageResult{page, err}
    }(continuationToken)
}
```

Each page fetch runs in its own goroutine; results are sent back on a channel. This overlaps S3 network latency with object processing. If the prefetch fails, the error is sent to the collector, which cancels the root context (shutting down all workers).

### 3.5 Collector

- Single goroutine, receives objects from all workers via a shared `chan ObjectRecord`
- Accumulates statistics using `statsAggregator` (unchanged logic, now single-threaded in collector)
- Batches BadgerDB writes: every `BatchSize` objects (or on scan completion), flushes accumulated objects in a single `db.Update()` transaction using `txn.Set()` calls
- Also writes the delta comparison for incremental scans
- On error signal from any worker: cancels root context
- When all workers are done: flushes final batch, saves scan result

**Channel design:**

```go
type collectorMsg struct {
    obj  *store.ObjectRecord
    err  error
}
objChan := make(chan collectorMsg, 1000) // buffered to decouple workers from collector
```

### 3.6 Stats Aggregator

The existing `statsAggregator` is used as-is. Since the collector is the sole caller of `addObject`, it needs no locking. All maps remain single-goroutine.

### 3.7 Delta Computation (Incremental Scans)

The existing delta logic is moved into the collector. Before the scan starts, previous object keys are loaded into memory (as today). As the collector processes objects from the channel, it populates the `seenKeys` map. After all workers complete, the collector computes the delta using the same `computeDelta` logic.

### 3.8 Error Handling

| Scenario | Behavior |
|---|---|
| S3 page fetch fails (non-retriable) | Worker sends error to collector → collector cancels context → all workers exit → scan marked failed |
| S3 page fetch fails (retriable) | Retried via existing `retryWithBackoff` before error is sent |
| Prefix discovery timeout | Dispatcher creates single `""` prefix, logs warning |
| Worker panics | Recovered in defer; error sent to collector |
| Collector write failure | Scan marked failed, context cancelled |
| Context cancelled | All goroutines check `ctx.Done()` in their loops and exit promptly |

## 4. Thread Safety

| Data | Strategy |
|---|---|
| `running` map | `sync.Mutex` (unchanged) |
| Object channel | `chan collectorMsg` with buffer=1000 |
| Stats aggregator | Single goroutine (collector), no lock |
| DB writes | Collector goroutine only, batched |
| Error flag | `atomic.Bool` (shared across workers) |
| Progress struct | `sync.RWMutex` (unchanged) |
| Prefix queue | `chan string` with buffer=Workers |

## 5. Settings UI

### 5.1 Data Model

Add to `web.SettingsData`:

```go
ScanWorkers          int   `json:"scan_workers"`
ScanBatchSize        int   `json:"scan_batch_size"`
ScanPrefixTimeoutSec int   `json:"scan_prefix_timeout"`
```

### 5.2 Settings Page

New "Scan Performance" card:

```html
<div class="card settings-section">
    <h2>Scan Performance</h2>
    <div class="form-group">
        <label for="scan_workers">Parallel Workers</label>
        <input type="range" id="scan_workers" name="scan_workers"
               min="1" max="32" value="{{.Settings.ScanWorkers}}"
               oninput="document.getElementById('scan_workers_val').textContent=this.value">
        <span id="scan_workers_val">{{.Settings.ScanWorkers}}</span>
    </div>
    <div class="form-group">
        <label for="scan_batch_size">DB Batch Size (objects)</label>
        <input type="number" id="scan_batch_size" name="scan_batch_size"
               value="{{.Settings.ScanBatchSize}}" min="100" max="5000" step="100">
    </div>
    <div class="form-group">
        <label for="scan_prefix_timeout">Prefix Discovery Timeout (seconds)</label>
        <input type="number" id="scan_prefix_timeout" name="scan_prefix_timeout"
               value="{{.Settings.ScanPrefixTimeoutSec}}" min="10" max="120">
    </div>
</div>
```

### 5.3 Persistence

Same mechanism as existing settings: saved as a BadgerDB `ScanResult` with ID `"__settings__"` on POST, loaded on startup. The handler reads saved settings and passes them to `scan.NewEngine`.

### 5.4 CLI Flags (Defaults)

```go
flag.IntVar(&cfg.ScanWorkers, "scan-workers", 4, "parallel prefix scanners (1-32)")
flag.IntVar(&cfg.ScanBatchSize, "scan-batch-size", 500, "objects per DB write batch")
flag.IntVar(&cfg.ScanPrefixTimeout, "scan-prefix-timeout", 30, "prefix discovery timeout in seconds")
```

Saved UI settings override CLI defaults. If no settings are saved, CLI defaults are used.

## 6. File Changes

| File | Change |
|---|---|
| `internal/scan/engine.go` | Add `Config` struct, replace `NewEngine` signature, implement pipeline |
| `internal/scan/scan_test.go` | Update tests for new `Engine` signature, add worker pool tests |
| `internal/s3/client.go` | Add `ListPrefixes` to `S3Client` interface |
| `internal/s3/list.go` | Implement `ListPrefixes` |
| `internal/web/render.go` | Add scan config fields to `SettingsData` |
| `internal/web/templates/settings.html` | Add "Scan Performance" card |
| `internal/web/handlers/handlers.go` | Read/save scan config in `PostSettings`, pass to engine |
| `cmd/s3lytics/main.go` | Add CLI flags for scan config, wire into handler |
| `docs/superpowers/specs/2026-05-22-parallel-scan-design.md` | This document |

## 7. Testing Strategy

- **Unit tests**: Worker pool with mock S3 client returning multiple prefixes, verify all objects collected
- **Batch write test**: Verify collector flushes at correct batch boundaries
- **Error propagation**: Mock S3 error on one prefix, verify scan marked failed and other workers cancelled
- **Prefix discovery timeout**: Mock slow prefix discovery, verify fallback to single prefix
- **Existing tests**: All current scan tests must pass with the new engine constructor (mock S3 returns single prefix `""`, one worker processes it)
- **Settings persistence**: Test save/load round-trip for scan config fields

## 8. Future Work (Out of Scope)

- S3 Batch Operations integration for truly massive buckets (100M+ objects)
- Distributed scan workers across multiple machines
- Real-time progress per-worker in the UI
