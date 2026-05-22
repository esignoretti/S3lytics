# Parallel Scan Engine Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Parallelize the S3 bucket scanner using prefix-based worker fan-out, page prefetching, and batched BadgerDB writes, plus expose tunables in the UI.

**Architecture:** Three-stage pipeline: Dispatcher discovers top-level prefixes via S3 delimiter, fans them out to N worker goroutines (each paginating independently with page prefetch), and a Collector goroutine merges stats + batches DB writes. Config struct (`Workers`, `BatchSize`, `PrefixTimeout`) is settable via CLI flags, saved settings, or the web UI.

**Tech Stack:** Go 1.26, chi, BadgerDB, aws-sdk-go-v2, htmx

---

### Task 1: Add `ListPrefixes` to S3Client interface + implementation

**Files:**
- Modify: `internal/s3/client.go:33-44`
- Modify: `internal/s3/list.go`
- Test: covered by mock in scan tests (Task 7)

- [ ] **Step 1: Add `ListPrefixes` to the `S3Client` interface**

In `internal/s3/client.go`, add to the interface block:

```go
ListPrefixes(ctx context.Context, bucket, prefix string) ([]string, error)
```

- [ ] **Step 2: Implement `ListPrefixes` on `CubbitS3Client`**

In `internal/s3/list.go`, add:

```go
func (c *CubbitS3Client) ListPrefixes(ctx context.Context, bucket, prefix string) ([]string, error) {
    input := &s3.ListObjectsV2Input{
        Bucket:    &bucket,
        Prefix:    &prefix,
        Delimiter: aws.String("/"),
        MaxKeys:   aws.Int32(1000),
    }
    resp, err := c.client.ListObjectsV2(ctx, input)
    if err != nil {
        return nil, fmt.Errorf("list prefixes: %w", err)
    }
    prefixes := make([]string, 0, len(resp.CommonPrefixes))
    for _, cp := range resp.CommonPrefixes {
        prefixes = append(prefixes, *cp.Prefix)
    }
    return prefixes, nil
}
```

Add `"github.com/aws/aws-sdk-go-v2/aws"` to imports.

- [ ] **Step 3: Commit**

```bash
git add internal/s3/client.go internal/s3/list.go
git commit -m "feat: add ListPrefixes to S3Client interface and CubbitS3Client implementation"
```

---

### Task 2: Add `ListPrefixes` stub to mock S3 client in tests

**Files:**
- Modify: `internal/scan/scan_test.go`

- [ ] **Step 1: Add `ListPrefixes` stub to `mockS3Client`**

In `internal/scan/scan_test.go`, add to the `mockS3Client` struct:

```go
func (m *mockS3Client) ListPrefixes(ctx context.Context, bucket, prefix string) ([]string, error) {
    return nil, nil
}
```

Place it after the `ListBuckets` method.

- [ ] **Step 2: Commit**

```bash
git add internal/scan/scan_test.go
git commit -m "test: add ListPrefixes stub to mockS3Client"
```

---

### Task 3: Add `Config` struct, `SetConfig`, and batched object storage to scan engine

**Files:**
- Modify: `internal/scan/engine.go`
- Test: `internal/scan/scan_test.go`

- [ ] **Step 1: Add `Config` struct and `collectorMsg` type at the top of `engine.go`**

```go
type Config struct {
    Workers            int
    BatchSize          int
    PrefixTimeout      time.Duration
}

type collectorMsg struct {
    obj  *store.ObjectRecord
    err  error
}
```

- [ ] **Step 2: Add `config` and `objChan` fields to the `Engine` struct, plus `SetConfig`**

```go
type Engine struct {
    client   s3.S3Client
    store    store.Store

    mu       sync.RWMutex
    progress *ScanProgress
    running  map[string]bool
    config   Config
    objChan  chan collectorMsg
}
```

Add `SetConfig` method:

```go
func (e *Engine) SetConfig(cfg Config) {
    e.mu.Lock()
    defer e.mu.Unlock()
    if cfg.Workers < 1 {
        cfg.Workers = 1
    }
    if cfg.Workers > 32 {
        cfg.Workers = 32
    }
    if cfg.BatchSize < 100 {
        cfg.BatchSize = 100
    }
    if cfg.BatchSize > 5000 {
        cfg.BatchSize = 5000
    }
    if cfg.PrefixTimeout <= 0 {
        cfg.PrefixTimeout = 30 * time.Second
    }
    e.config = cfg
}
```

- [ ] **Step 3: Write a failing test for `SetConfig` clamping**

In `internal/scan/scan_test.go`:

```go
func TestSetConfigClamping(t *testing.T) {
    st := newTestStore(t)
    engine := NewEngine(nil, st)

    engine.SetConfig(Config{Workers: 0, BatchSize: 50, PrefixTimeout: 0})
    if engine.config.Workers != 1 {
        t.Errorf("expected Workers clamped to 1, got %d", engine.config.Workers)
    }
    if engine.config.BatchSize != 100 {
        t.Errorf("expected BatchSize clamped to 100, got %d", engine.config.BatchSize)
    }
    if engine.config.PrefixTimeout != 30*time.Second {
        t.Errorf("expected PrefixTimeout default 30s, got %v", engine.config.PrefixTimeout)
    }

    engine.SetConfig(Config{Workers: 100, BatchSize: 10000})
    if engine.config.Workers != 32 {
        t.Errorf("expected Workers clamped to 32, got %d", engine.config.Workers)
    }
    if engine.config.BatchSize != 5000 {
        t.Errorf("expected BatchSize clamped to 5000, got %d", engine.config.BatchSize)
    }
}
```

- [ ] **Step 4: Run test to verify it fails**

Run: `cd internal/scan && go test -run TestSetConfigClamping -v`
Expected: Compilation error (Config type exists but field not on Engine yet) or FAIL

- [ ] **Step 5: Add default config in `NewEngine`**

Update `NewEngine`:

```go
func NewEngine(client s3.S3Client, s store.Store) *Engine {
    return &Engine{
        client:  client,
        store:   s,
        running: make(map[string]bool),
        config: Config{
            Workers:       4,
            BatchSize:     500,
            PrefixTimeout: 30 * time.Second,
        },
    }
}
```

- [ ] **Step 6: Add `saveObjectBatch` helper to engine**

This is used by the collector to batch-write objects:

```go
func (e *Engine) saveObjectBatch(ctx context.Context, batch []*store.ObjectRecord, bucket string) error {
    if len(batch) == 0 {
        return nil
    }
    return e.store.SaveObjects(ctx, bucket, batch)
}
```

- [ ] **Step 7: Add `SaveObjects` to the `Store` interface**

In `internal/store/store.go`, add to the interface:

```go
SaveObjects(ctx context.Context, bucket string, objects []*ObjectRecord) error
```

- [ ] **Step 8: Implement `SaveObjects` on `BadgerStore`**

```go
func (s *BadgerStore) SaveObjects(ctx context.Context, bucket string, objects []*ObjectRecord) error {
    return s.db.Update(func(txn *badger.Txn) error {
        for _, obj := range objects {
            key := objectKey(bucket, obj.ETag+"/"+obj.Key)
            data, err := json.Marshal(obj)
            if err != nil {
                return fmt.Errorf("marshal object: %w", err)
            }
            if err := txn.Set(key, data); err != nil {
                return fmt.Errorf("set object: %w", err)
            }
        }
        return nil
    })
}
```

- [ ] **Step 9: Run tests to verify everything passes**

Run: `cd internal/scan && go test -run TestSetConfigClamping -v`
Expected: PASS

Run: `cd internal/store && go test -run TestSaveObjects -v` (might not exist yet — run all store tests)
Expected: PASS

- [ ] **Step 10: Commit**

```bash
git add internal/scan/engine.go internal/scan/scan_test.go internal/store/store.go
git commit -m "feat: add scan Config struct, SetConfig with clamping, batched SaveObjects store method"
```

---

### Task 4: Implement the Dispatcher + Worker pipeline

**Files:**
- Modify: `internal/scan/engine.go`
- Test: `internal/scan/scan_test.go`

- [ ] **Step 1: Implement `discoverPrefixes` method**

```go
func (e *Engine) discoverPrefixes(ctx context.Context, bucket string) ([]string, error) {
    timeout := e.config.PrefixTimeout
    ctx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()

    prefixes, err := e.client.ListPrefixes(ctx, bucket, "")
    if err != nil {
        return nil, fmt.Errorf("discover prefixes: %w", err)
    }
    if len(prefixes) == 0 {
        return []string{""}, nil
    }
    return prefixes, nil
}
```

- [ ] **Step 2: Implement `runDispatcher` method**

```go
func (e *Engine) runDispatcher(ctx context.Context, bucket string, prefixChan chan<- string) error {
    prefixes, err := e.discoverPrefixes(ctx, bucket)
    if err != nil {
        log.Printf("scan: prefix discovery failed, using single prefix: %v", err)
        prefixChan <- ""
        return nil
    }

    for _, p := range prefixes {
        select {
        case prefixChan <- p:
        case <-ctx.Done():
            return ctx.Err()
        }
    }
    return nil
}
```

- [ ] **Step 3: Implement `runWorker` method**

```go
func (e *Engine) runWorker(ctx context.Context, bucket, prefix string, objChan chan<- collectorMsg) {
    var continuationToken *string
    var nextPage <-chan pageResult
    var isTruncated bool

    for {
        select {
        case result := <-nextPage:
            if result.err != nil {
                objChan <- collectorMsg{err: result.err}
                return
            }
            for _, obj := range result.page.Objects {
                objChan <- collectorMsg{
                    obj: &store.ObjectRecord{
                        Key:          obj.Key,
                        ETag:         obj.ETag,
                        Size:         obj.Size,
                        LastModified: obj.LastModified,
                        StorageClass: obj.StorageClass,
                    },
                }
            }
            isTruncated = result.page.IsTruncated
            continuationToken = result.page.ContinuationToken
        case <-ctx.Done():
            return
        }

        if !isTruncated {
            break
        }

        ch := make(chan pageResult, 1)
        nextPage = ch
        token := continuationToken
        go func() {
            page, err := e.client.ListObjectsPage(ctx, bucket, token)
            ch <- pageResult{page, err}
        }()
    }
}
```

Place `pageResult` at package level (or inside engine file):

```go
type pageResult struct {
    page *s3.ListResult
    err  error
}
```

Note: This worker does NOT assign `ScanID` yet — the collector will do that when it has the scan ID context.

- [ ] **Step 4: Implement `runCollector` method**

```go
func (e *Engine) runCollector(ctx context.Context, scanID, bucket string, objChan <-chan collectorMsg, workerDone <-chan struct{}) error {
    agg := newStatsAggregator()
    batch := make([]*store.ObjectRecord, 0, e.config.BatchSize)
    startTime := time.Now()

    flush := func() error {
        if len(batch) == 0 {
            return nil
        }
        for _, obj := range batch {
            agg.addObject(s3.ObjectInfo{
                Key:          obj.Key,
                ETag:         obj.ETag,
                Size:         obj.Size,
                LastModified: obj.LastModified,
                StorageClass: obj.StorageClass,
            })
        }
        if err := e.saveObjectBatch(ctx, batch, bucket); err != nil {
            return fmt.Errorf("flush batch: %w", err)
        }
        e.updateProgress(scanID, agg.totalObjects, time.Since(startTime))
        batch = batch[:0]
        return nil
    }

    for {
        select {
        case msg, ok := <-objChan:
            if !ok {
                // channel closed, flush and exit
                return flush()
            }
            if msg.err != nil {
                return msg.err
            }
            msg.obj.ScanID = scanID
            batch = append(batch, msg.obj)
            if len(batch) >= e.config.BatchSize {
                if err := flush(); err != nil {
                    return err
                }
            }
        case <-workerDone:
            // all workers finished, drain remaining messages then flush
            for msg := range objChan {
                if msg.err != nil {
                    return msg.err
                }
                msg.obj.ScanID = scanID
                batch = append(batch, msg.obj)
                if len(batch) >= e.config.BatchSize {
                    if err := flush(); err != nil {
                        return err
                    }
                }
            }
            return flush()
        case <-ctx.Done():
            return ctx.Err()
        }
    }
}
```

- [ ] **Step 5: Wire the pipeline in `runFullScan`**

Replace the current `runFullScan` body:

```go
func (e *Engine) runFullScan(ctx context.Context, scanID, bucket string) {
    defer func() {
        e.mu.Lock()
        delete(e.running, bucket)
        e.mu.Unlock()
    }()

    ctx, cancel := context.WithCancel(ctx)
    defer cancel()

    prefixChan := make(chan string, e.config.Workers)
    objChan := make(chan collectorMsg, 1000)
    workerDone := make(chan struct{})
    startTime := time.Now()

    // Start collector
    collectorErr := make(chan error, 1)
    go func() {
        collectorErr <- e.runCollector(ctx, scanID, bucket, objChan, workerDone)
    }()

    // Discover and dispatch prefixes
    go func() {
        if err := e.runDispatcher(ctx, bucket, prefixChan); err != nil {
            objChan <- collectorMsg{err: err}
        }
        close(prefixChan)
    }()

    // Start workers
    var wg sync.WaitGroup
    for i := 0; i < e.config.Workers; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for prefix := range prefixChan {
                // skip empty context check
                e.runWorker(ctx, bucket, prefix, objChan)
            }
        }()
    }

    // Wait for all workers to finish
    wg.Wait()
    close(workerDone)

    // Wait for collector to finish
    if err := <-collectorErr; err != nil {
        e.failScan(scanID, fmt.Sprintf("scan failed: %v", err))
        return
    }

    // Build and save scan result
    // (This code is replaced in Step 6 below with the full implementation using collector result)
```

- [ ] **Step 6: Finalize `runFullScan` with collector result**

```go
func (e *Engine) runFullScan(ctx context.Context, scanID, bucket string) {
    defer func() {
        e.mu.Lock()
        delete(e.running, bucket)
        e.mu.Unlock()
    }()

    ctx, cancel := context.WithCancel(ctx)
    defer cancel()

    prefixChan := make(chan string, e.config.Workers)
    objChan := make(chan collectorMsg, 1000)
    workerDone := make(chan struct{})
    startTime := time.Now()

    // Start collector
    type collResult struct {
        agg *statsAggregator
        err error
    }
    collCh := make(chan collResult, 1)
    go func() {
        agg, err := e.runCollector(ctx, scanID, bucket, objChan, workerDone)
        collCh <- collResult{agg, err}
    }()

    // Discover and dispatch prefixes
    go func() {
        if err := e.runDispatcher(ctx, bucket, prefixChan); err != nil {
            objChan <- collectorMsg{err: err}
        }
        close(prefixChan)
    }()

    // Start workers
    var wg sync.WaitGroup
    for i := 0; i < e.config.Workers; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for prefix := range prefixChan {
                e.runWorker(ctx, bucket, prefix, objChan)
            }
        }()
    }

    // Wait for all workers to finish
    wg.Wait()
    close(workerDone)
    // Workers won't send more objects, collector will drain and flush

    result := <-collCh
    if result.err != nil {
        e.failScan(scanID, fmt.Sprintf("scan failed: %v", result.err))
        return
    }

    agg := result.agg
    summary := agg.buildSummary()
    scanResult := &store.ScanResult{
        Record: store.ScanRecord{
            ID:        scanID,
            Bucket:    bucket,
            Timestamp: startTime,
            Duration:  time.Since(startTime).String(),
            Status:    "completed",
            ScanType:  "full",
        },
        Summary:  summary,
        Types:    agg.buildTypes(),
        Ages:     agg.buildAges(),
        Storage:  agg.buildStorage(),
        Prefixes: agg.buildPrefixes(),
    }

    if err := e.store.SaveScanResult(ctx, scanResult); err != nil {
        e.failScan(scanID, fmt.Sprintf("save result: %v", err))
        return
    }

    record, _ := e.store.GetScan(ctx, scanID)
    record.Status = "completed"
    record.Duration = time.Since(startTime).String()
    _ = e.store.SaveScan(ctx, record)
    _ = e.store.AddScanToBucketIndex(ctx, bucket, scanID)

    e.setProgress(&ScanProgress{
        ScanID:      scanID,
        Bucket:      bucket,
        Status:      "completed",
        ObjectsDone: summary.TotalObjects,
        TotalFound:  summary.TotalObjects,
        Elapsed:     time.Since(startTime).String(),
    })
}
```

- [ ] **Step 7: Run existing tests**

Run: `cd internal/scan && go test -v -count=1`
Expected: All existing scan tests pass (TestFullScanBasic, TestScanTypeBreakdown, TestScanProgressTracking, TestConcurrentScanGuard, TestSetS3Client)

If any fail, fix the issue — likely the mock returns 0 prefixes which causes the dispatcher to send `""` prefix, and the worker should still process it.

- [ ] **Step 8: Commit**

```bash
git add internal/scan/engine.go
git commit -m "feat: implement Dispatcher/Worker/Collector scan pipeline with page prefetch and batch writes"
```

---

### Task 5: Update `runIncrementalScan` to use the new pipeline

**Files:**
- Modify: `internal/scan/engine.go`

- [ ] **Step 1: Rewrite `runIncrementalScan`**

Replace the body with the same pipeline as `runFullScan`, but with delta computation added to the collector. Add a `previousObjects` parameter to the collector, and compute delta after all objects are processed.

Modify `runCollector` to accept an optional `previousObjects` map:

```go
func (e *Engine) runCollector(ctx context.Context, scanID, bucket string, objChan <-chan collectorMsg, workerDone <-chan struct{}, previousObjects map[string]*store.ObjectRecord) (*statsAggregator, *store.DeltaReport, error) {
```

If `previousObjects` is nil, skip delta computation. Otherwise, populate `seenKeys` and compute delta after flush.

Add after the final flush in the collector:

```go
// Compute delta for incremental scans
var delta *store.DeltaReport
if previousObjects != nil {
    seenKeys := make(map[string]bool)
    for _, obj := range allProcessedObjects { // need to track all keys seen
        seenKeys[obj.Key] = true
    }
    delta = e.computeDelta(previousObjects, seenKeys, currentKeys)
}
```

Actually, simpler: track `seenKeys` and `currentKeys` in the collector as objects arrive. Add two maps to the collector scope:

```go
var seenKeys map[string]bool
var currentKeys map[string]*store.ObjectRecord
if previousObjects != nil {
    seenKeys = make(map[string]bool)
    currentKeys = make(map[string]*store.ObjectRecord)
}
```

When processing each msg.obj:

```go
if seenKeys != nil {
    seenKeys[msg.obj.Key] = true
    currentKeys[msg.obj.Key] = msg.obj
}
```

After final flush, compute delta if applicable.

- [ ] **Step 2: Adapt `runIncrementalScan` to call the pipeline**

```go
func (e *Engine) runIncrementalScan(ctx context.Context, scanID, bucket string) {
    defer func() {
        e.mu.Lock()
        delete(e.running, bucket)
        e.mu.Unlock()
    }()

    // Load previous objects for delta
    previousKeys, _ := e.store.ListObjectKeys(ctx, bucket)
    previousObjects := make(map[string]*store.ObjectRecord)
    for _, k := range previousKeys {
        prefix := "objects/" + bucket + "/"
        if len(k) > len(prefix) {
            suffix := k[len(prefix):]
            if slashIdx := strings.IndexByte(suffix, '/'); slashIdx >= 0 {
                objKey := suffix[slashIdx+1:]
                obj, err := e.store.GetObject(ctx, bucket, objKey)
                if err == nil && obj != nil {
                    previousObjects[obj.Key] = obj
                }
            }
        }
    }

    ctx, cancel := context.WithCancel(ctx)
    defer cancel()

    prefixChan := make(chan string, e.config.Workers)
    objChan := make(chan collectorMsg, 1000)
    workerDone := make(chan struct{})
    startTime := time.Now()

    type collResult struct {
        agg   *statsAggregator
        delta *store.DeltaReport
        err   error
    }
    collCh := make(chan collResult, 1)
    go func() {
        agg, delta, err := e.runCollector(ctx, scanID, bucket, objChan, workerDone, previousObjects)
        collCh <- collResult{agg, delta, err}
    }()

    go func() {
        if err := e.runDispatcher(ctx, bucket, prefixChan); err != nil {
            objChan <- collectorMsg{err: err}
        }
        close(prefixChan)
    }()

    var wg sync.WaitGroup
    for i := 0; i < e.config.Workers; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for prefix := range prefixChan {
                e.runWorker(ctx, bucket, prefix, objChan)
            }
        }()
    }

    wg.Wait()
    close(workerDone)

    result := <-collCh
    if result.err != nil {
        e.failScan(scanID, fmt.Sprintf("scan failed: %v", result.err))
        return
    }

    agg := result.agg
    summary := agg.buildSummary()
    scanResult := &store.ScanResult{
        Record: store.ScanRecord{
            ID:        scanID,
            Bucket:    bucket,
            Timestamp: startTime,
            Duration:  time.Since(startTime).String(),
            Status:    "completed",
            ScanType:  "incremental",
        },
        Summary:  summary,
        Types:    agg.buildTypes(),
        Ages:     agg.buildAges(),
        Storage:  agg.buildStorage(),
        Prefixes: agg.buildPrefixes(),
        Delta:    result.delta,
    }

    if err := e.store.SaveScanResult(ctx, scanResult); err != nil {
        e.failScan(scanID, fmt.Sprintf("save result: %v", err))
        return
    }

    record, _ := e.store.GetScan(ctx, scanID)
    record.Status = "completed"
    record.Duration = time.Since(startTime).String()
    _ = e.store.SaveScan(ctx, record)
    _ = e.store.AddScanToBucketIndex(ctx, bucket, scanID)

    e.setProgress(&ScanProgress{
        ScanID:      scanID,
        Bucket:      bucket,
        Status:      "completed",
        ObjectsDone: summary.TotalObjects,
        TotalFound:  summary.TotalObjects,
        Elapsed:     time.Since(startTime).String(),
    })
}
```

- [ ] **Step 3: Run incremental scan tests**

Run: `cd internal/scan && go test -run TestIncremental -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/scan/engine.go
git commit -m "feat: adapt incremental scan to use Dispatcher/Worker/Collector pipeline with delta"
```

---

### Task 6: Add new tests for worker pool, error propagation, and batching

**Files:**
- Modify: `internal/scan/scan_test.go`

- [ ] **Step 1: Write test for worker pool with multiple prefixes**

```go
func TestWorkerPoolMultiplePrefixes(t *testing.T) {
    st := newTestStore(t)
    mock := &mockS3Client{
        objects: []s3.ObjectInfo{
            {Key: "a.txt", ETag: "e1", Size: 100, LastModified: time.Now()},
            {Key: "b.txt", ETag: "e2", Size: 200, LastModified: time.Now()},
        },
    }
    // Override ListPrefixes to return two prefixes
    mock.listPrefixesResult = []string{"logs/", "media/"}

    engine := NewEngine(mock, st)
    engine.SetConfig(Config{Workers: 2, BatchSize: 100, PrefixTimeout: 5 * time.Second})
    ctx := context.Background()

    scanID, err := engine.StartFullScan(ctx, "test-bucket")
    if err != nil {
        t.Fatal(err)
    }
    waitForScanComplete(t, engine)

    result, err := st.GetScanResult(ctx, scanID)
    if err != nil {
        t.Fatal(err)
    }
    if result.Summary.TotalObjects != 2 {
        t.Errorf("expected 2 objects, got %d", result.Summary.TotalObjects)
    }
}
```

Add `listPrefixesResult` field to `mockS3Client` and update the `ListPrefixes` stub:

```go
type mockS3Client struct {
    objects            []s3.ObjectInfo
    listPrefixesResult []string
}

func (m *mockS3Client) ListPrefixes(ctx context.Context, bucket, prefix string) ([]string, error) {
    return m.listPrefixesResult, nil
}
```

- [ ] **Step 2: Write test for error propagation from one worker**

```go
func TestWorkerPoolErrorPropagation(t *testing.T) {
    st := newTestStore(t)
    mock := &mockS3Client{
        listPrefixesResult: []string{"ok/", "fail/"},
    }
    // Make ListObjectsPage fail for "fail/" prefix based on bucket name
    // The simplest approach: use a flag that gets set on second call
    callCount := 0
    mock.listObjectsPageFn = func(ctx context.Context, bucket string, token *string) (*s3.ListResult, error) {
        callCount++
        if callCount > 1 {
            return nil, fmt.Errorf("simulated error")
        }
        return &s3.ListResult{Objects: []s3.ObjectInfo{
            {Key: "ok/a.txt", ETag: "e1", Size: 100, LastModified: time.Now()},
        }, IsTruncated: false}, nil
    }

    engine := NewEngine(mock, st)
    engine.SetConfig(Config{Workers: 2, BatchSize: 100, PrefixTimeout: 5 * time.Second})
    ctx := context.Background()

    _, err := engine.StartFullScan(ctx, "test-bucket")
    if err != nil {
        t.Fatal(err)
    }
    waitForScanComplete(t, engine)

    progress := engine.GetProgress()
    if progress.Status != "failed" {
        t.Errorf("expected scan to fail, got status %s", progress.Status)
    }
}
```

Add `listObjectsPageFn` field to `mockS3Client` and update `ListObjectsPage` to use it if non-nil:

```go
type mockS3Client struct {
    objects            []s3.ObjectInfo
    listPrefixesResult []string
    listObjectsPageFn  func(ctx context.Context, bucket string, token *string) (*s3.ListResult, error)
}

func (m *mockS3Client) ListObjectsPage(ctx context.Context, bucket string, continuationToken *string) (*s3.ListResult, error) {
    if m.listObjectsPageFn != nil {
        return m.listObjectsPageFn(ctx, bucket, continuationToken)
    }
    return &s3.ListResult{
        Objects:     m.objects,
        IsTruncated: false,
    }, nil
}
```

- [ ] **Step 3: Run all scan tests**

Run: `cd internal/scan && go test -v -count=1`
Expected: All tests pass

- [ ] **Step 4: Commit**

```bash
git add internal/scan/scan_test.go
git commit -m "test: add worker pool and error propagation tests"
```

---

### Task 7: Add scan config fields to SettingsData and web settings

**Files:**
- Modify: `internal/web/render.go`
- Modify: `internal/web/templates/settings.html`
- Modify: `internal/web/handlers/handlers.go`

- [ ] **Step 1: Add scan config fields to `SettingsData`**

In `internal/web/render.go`, add to `SettingsData`:

```go
ScanWorkers          int     `json:"scan_workers"`
ScanBatchSize        int     `json:"scan_batch_size"`
ScanPrefixTimeoutSec int     `json:"scan_prefix_timeout"`
```

- [ ] **Step 2: Add "Scan Performance" card to settings template**

In `internal/web/templates/settings.html`, add before the save button:

```html
<div class="card settings-section">
    <h2>Scan Performance</h2>
    <div class="form-group">
        <label for="scan_workers">Parallel Workers</label>
        <input type="range" id="scan_workers" name="scan_workers"
               min="1" max="32" value="{{.Settings.ScanWorkers}}"
               oninput="document.getElementById('scan_workers_val').textContent=this.value">
        <span id="scan_workers_val" style="margin-left:0.5rem;font-weight:700;">{{.Settings.ScanWorkers}}</span>
    </div>
    <div class="form-group">
        <label for="scan_batch_size">DB Batch Size (objects)</label>
        <input type="number" id="scan_batch_size" name="scan_batch_size"
               value="{{.Settings.ScanBatchSize}}" min="100" max="5000" step="100">
        <span class="form-help">Higher = fewer transactions, more memory</span>
    </div>
    <div class="form-group">
        <label for="scan_prefix_timeout">Prefix Discovery Timeout (seconds)</label>
        <input type="number" id="scan_prefix_timeout" name="scan_prefix_timeout"
               value="{{.Settings.ScanPrefixTimeoutSec}}" min="10" max="120">
        <span class="form-help">If timeout expires, falls back to single-prefix mode</span>
    </div>
</div>
```

- [ ] **Step 3: Update `PostSettings` handler to read scan config fields**

In `internal/web/handlers/handlers.go`, inside `PostSettings`, add after the existing field reads:

```go
if workers, err := strconv.Atoi(r.FormValue("scan_workers")); err == nil && workers >= 1 && workers <= 32 {
    settings.ScanWorkers = workers
}
if batchSize, err := strconv.Atoi(r.FormValue("scan_batch_size")); err == nil && batchSize >= 100 && batchSize <= 5000 {
    settings.ScanBatchSize = batchSize
}
if timeout, err := strconv.Atoi(r.FormValue("scan_prefix_timeout")); err == nil && timeout >= 10 && timeout <= 120 {
    settings.ScanPrefixTimeoutSec = timeout
}
```

- [ ] **Step 4: Update `loadSettings` defaults**

In `loadSettings`, add:

```go
ScanWorkers:          4,
ScanBatchSize:        500,
ScanPrefixTimeoutSec: 30,
```

- [ ] **Step 5: Apply scan config to engine on settings save**

After saving settings in `PostSettings`, call:

```go
h.ScanEngine.SetConfig(scan.Config{
    Workers:       settings.ScanWorkers,
    BatchSize:     settings.ScanBatchSize,
    PrefixTimeout: time.Duration(settings.ScanPrefixTimeoutSec) * time.Second,
})
```

Also apply on startup in `GetSettings` (the initial load). Best place: in `main.go` after creating the handler, call:

```go
h.ScanEngine.SetConfig(scan.Config{
    Workers:       *scanWorkers,
    BatchSize:     *scanBatchSize,
    PrefixTimeout: time.Duration(*scanPrefixTimeout) * time.Second,
})
```

Where `scanWorkers`, `scanBatchSize`, `scanPrefixTimeout` are CLI flags.

But also check saved settings on startup: after loading settings from store, apply them to the engine.

- [ ] **Step 6: Run vet + build**

Run: `cd /Users/esignoretti/Documents/OpenCode/S3lytics && go vet ./... && go build ./...`
Expected: clean

- [ ] **Step 7: Commit**

```bash
git add internal/web/render.go internal/web/templates/settings.html internal/web/handlers/handlers.go
git commit -m "feat: add scan performance config to settings page and apply to engine"
```

---

### Task 8: Add CLI flags for scan config and wire into startup

**Files:**
- Modify: `cmd/s3lytics/main.go`

- [ ] **Step 1: Add CLI flags**

```go
scanWorkers := flag.Int("scan-workers", 4, "Parallel prefix scanners (1-32)")
scanBatchSize := flag.Int("scan-batch-size", 500, "Objects per DB write batch (100-5000)")
scanPrefixTimeout := flag.Int("scan-prefix-timeout", 30, "Prefix discovery timeout in seconds")
```

- [ ] **Step 2: Apply config to engine after handler creation**

Before `h.RegisterRoutes(r)` (or right after `h := &handlers.Handler{...}`), add:

```go
h.ScanEngine.SetConfig(scan.Config{
    Workers:       clamp(*scanWorkers, 1, 32),
    BatchSize:     clamp(*scanBatchSize, 100, 5000),
    PrefixTimeout: time.Duration(clamp(*scanPrefixTimeout, 10, 120)) * time.Second,
})
```

Add a small helper at the bottom of `main.go`:

```go
func clamp(v, min, max int) int {
    if v < min {
        return min
    }
    if v > max {
        return v
    }
    return v
}
```

Also load saved settings from store and apply on top:

```go
if saved, err := badgerStore.GetScanResult(ctx, "__settings__"); err == nil && saved != nil {
    // Can't decode settings from ScanResult easily — rely on handler.loadSettings
    // The handler will apply on GetSettings. For engine startup, CLI flags are fine.
}
```

Actually, since settings are stored as a `ScanResult` stub (hacky existing pattern), direct loading is awkward. For now, CLI flags as defaults + UI settings applied on save is sufficient. The engine will get the config again when the user visits settings or saves settings.

- [ ] **Step 3: Run vet + build**

Run: `cd /Users/esignoretti/Documents/OpenCode/S3lytics && go vet ./... && go build ./...`
Expected: clean

- [ ] **Step 4: Run all tests**

Run: `go test ./... -count=1`
Expected: all pass

- [ ] **Step 5: Commit**

```bash
git add cmd/s3lytics/main.go
git commit -m "feat: add --scan-workers, --scan-batch-size, --scan-prefix-timeout CLI flags"
```

---

### Task 9: Self-review and fix plan issues

- [ ] **Step 1: Verify the `mockS3Client` has all required methods**

Check that `mockS3Client` now has `ListPrefixes`. Run: `go vet ./internal/scan/`. Expected: clean.

If `go vet` fails because `mockS3Client` doesn't implement `S3Client`, add the missing method.

- [ ] **Step 2: Run full test suite**

Run: `go test ./... -count=1 -v 2>&1 | tail -30`
Expected: all tests pass.

- [ ] **Step 3: Fix any compilation issues**

If `collectorMsg` references types not imported, fix imports. If `pageResult` is referenced but not defined, define it. If `agg.totalObjects` is accessed directly (it's unexported), add a getter method to `statsAggregator` or make the field accessible within the package.

Add to `internal/scan/stats.go`:
```go
func (a *statsAggregator) TotalObjects() int64 {
    return a.totalObjects
}
```

- [ ] **Step 4: Commit any fixes**

```bash
git add -A
git commit -m "fix: compilation fixes after full test run"
```

---

### Spec Coverage Check

| Spec Section | Task |
|---|---|
| 3.1 Config struct + SetConfig | Task 3 |
| 3.2 ListPrefixes S3 method | Task 1 |
| 3.3 Dispatcher | Task 4 |
| 3.4 Worker + page prefetch | Task 4 |
| 3.5 Collector + batch writes | Task 4 |
| 3.6 Stats aggregator (unchanged) | — (no change needed) |
| 3.7 Delta computation | Task 5 |
| 3.8 Error handling | Tasks 4, 6 |
| 4. Thread safety | Tasks 3-5 (built into design) |
| 5.1 SettingsData fields | Task 7 |
| 5.2 Settings page | Task 7 |
| 5.3 Persistence | Task 7 |
| 5.4 CLI flags | Task 8 |
| 7. Testing | Tasks 2, 3, 6 |
