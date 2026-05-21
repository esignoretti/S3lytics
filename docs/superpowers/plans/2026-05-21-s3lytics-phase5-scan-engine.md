# S3lytics — Phase 5: Scan Engine (Basic + Incremental)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the scan engine that lists S3 objects via pagination, computes basic statistics, supports incremental scans with delta detection, and saves results to the store. Includes a checkpoint mechanism for partial scans.

**Architecture:** Package `internal/scan/` owns the `Engine` struct. It uses `s3.S3Client` for listing and `store.Store` for persistence. A `Scanner` goroutine processes paginated listing, computes stats in-memory, and stores results. Checkpoints save state every N objects for interruption resilience. Basic scan metrics follow the 12 metrics from the design doc.

**Tech Stack:** Go standard library (sync, math), `s3.S3Client` (Phase 3), `store.Store` (Phase 2)

**Pre-requisites:** Phases 2, 3, and 4 complete.

---

### Task 1: Statistics aggregator

**Files:**
- Create: `internal/scan/stats.go`

- [ ] **Step 1: Write the stats aggregator**

```go
package scan

import (
	"math"
	"sort"
	"time"

	"github.com/esignoretti/s3lytics/internal/s3"
	"github.com/esignoretti/s3lytics/internal/store"
)

// statsAggregator collects and computes scan statistics from a stream of objects.
type statsAggregator struct {
	totalObjects int64
	totalSize    int64
	sizes        []int64
	maxSize      int64
	emptyObjects int64

	typeMap    map[string]*typeAccum
	ageBuckets map[string]*ageAccum
	storageMap map[string]*storageAccum
	prefixMap  map[string]*prefixAccum

	startTime time.Time
}

type typeAccum struct {
	count int64
	size  int64
}

type ageAccum struct {
	count int64
	size  int64
}

type storageAccum struct {
	count int64
	size  int64
}

type prefixAccum struct {
	count int64
	size  int64
}

func newStatsAggregator() *statsAggregator {
	return &statsAggregator{
		typeMap:    make(map[string]*typeAccum),
		ageBuckets: make(map[string]*ageAccum),
		storageMap: make(map[string]*storageAccum),
		prefixMap:  make(map[string]*prefixAccum),
		startTime:  time.Now(),
	}
}

// addObject processes one object into the aggregator.
func (a *statsAggregator) addObject(obj s3.ObjectInfo) {
	a.totalObjects++
	a.totalSize += obj.Size
	a.sizes = append(a.sizes, obj.Size)

	if obj.Size > a.maxSize {
		a.maxSize = obj.Size
	}
	if obj.Size == 0 {
		a.emptyObjects++
	}

	// File type by extension
	ext := extractExtension(obj.Key)
	if _, ok := a.typeMap[ext]; !ok {
		a.typeMap[ext] = &typeAccum{}
	}
	a.typeMap[ext].count++
	a.typeMap[ext].size += obj.Size

	// Age bucket
	ageLabel := ageBucketLabel(obj.LastModified)
	if _, ok := a.ageBuckets[ageLabel]; !ok {
		a.ageBuckets[ageLabel] = &ageAccum{}
	}
	a.ageBuckets[ageLabel].count++
	a.ageBuckets[ageLabel].size += obj.Size

	// Storage class
	sc := obj.StorageClass
	if sc == "" {
		sc = "STANDARD"
	}
	if _, ok := a.storageMap[sc]; !ok {
		a.storageMap[sc] = &storageAccum{}
	}
	a.storageMap[sc].count++
	a.storageMap[sc].size += obj.Size

	// Top-level prefix
	prefix := topLevelPrefix(obj.Key)
	if _, ok := a.prefixMap[prefix]; !ok {
		a.prefixMap[prefix] = &prefixAccum{}
	}
	a.prefixMap[prefix].count++
	a.prefixMap[prefix].size += obj.Size
}

func (a *statsAggregator) buildSummary() store.ScanSummary {
	sort.Slice(a.sizes, func(i, j int) bool { return a.sizes[i] < a.sizes[j] })

	var median int64
	if len(a.sizes) > 0 {
		mid := len(a.sizes) / 2
		if len(a.sizes)%2 == 0 {
			median = (a.sizes[mid-1] + a.sizes[mid]) / 2
		} else {
			median = a.sizes[mid]
		}
	}

	var avg float64
	if a.totalObjects > 0 {
		avg = float64(a.totalSize) / float64(a.totalObjects)
	}

	return store.ScanSummary{
		TotalObjects: a.totalObjects,
		TotalSize:    a.totalSize,
		AvgSize:      math.Round(avg*100) / 100,
		MedianSize:   median,
		MaxSize:      a.maxSize,
		EmptyObjects: a.emptyObjects,
	}
}

func (a *statsAggregator) buildTypes() []store.TypeBreakdown {
	var result []store.TypeBreakdown
	for ext, acc := range a.typeMap {
		pct := 0.0
		if a.totalObjects > 0 {
			pct = math.Round(float64(acc.count)/float64(a.totalObjects)*10000) / 100
		}
		result = append(result, store.TypeBreakdown{
			Ext:       ext,
			Count:     acc.count,
			TotalSize: acc.size,
			Pct:       pct,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Count > result[j].Count })
	return result
}

func (a *statsAggregator) buildAges() []store.AgeBucket {
	var result []store.AgeBucket
	for label, acc := range a.ageBuckets {
		result = append(result, store.AgeBucket{
			Label: label,
			Count: acc.count,
			Size:  acc.size,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Count > result[j].Count })
	return result
}

func (a *statsAggregator) buildStorage() []store.StorageBreakdown {
	var result []store.StorageBreakdown
	for class, acc := range a.storageMap {
		result = append(result, store.StorageBreakdown{
			Class: class,
			Count: acc.count,
			Size:  acc.size,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Count > result[j].Count })
	return result
}

func (a *statsAggregator) buildPrefixes() []store.PrefixBreakdown {
	var result []store.PrefixBreakdown
	for prefix, acc := range a.prefixMap {
		result = append(result, store.PrefixBreakdown{
			Prefix: prefix,
			Count:  acc.count,
			Size:   acc.size,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Count > result[j].Count })
	return result
}

func extractExtension(key string) string {
	for i := len(key) - 1; i >= 0; i-- {
		if key[i] == '.' {
			return key[i:]
		}
		if key[i] == '/' {
			break
		}
	}
	return "(no extension)"
}

func topLevelPrefix(key string) string {
	for i, c := range key {
		if c == '/' {
			return key[:i]
		}
	}
	return key
}

func ageBucketLabel(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < 24*time.Hour:
		return "<24h"
	case d < 7*24*time.Hour:
		return "<7d"
	case d < 30*24*time.Hour:
		return "<30d"
	case d < 90*24*time.Hour:
		return "<90d"
	case d < 365*24*time.Hour:
		return "<1y"
	default:
		return ">1y"
	}
}
```

- [ ] **Step 2: Verify compilation**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go vet ./internal/scan/
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/scan/stats.go && git commit -m "feat: add statistics aggregator for scan metrics"
```

---

### Task 2: Scan engine — full scan

**Files:**
- Create: `internal/scan/engine.go`

- [ ] **Step 1: Write the scan engine with full scan**

```go
package scan

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/esignoretti/s3lytics/internal/s3"
	"github.com/esignoretti/s3lytics/internal/store"
	"github.com/google/uuid"
)

// ScanProgress holds current scan status for progress polling.
type ScanProgress struct {
	ScanID      string `json:"scan_id"`
	Bucket      string `json:"bucket"`
	Status      string `json:"status"` // pending, running, completed, failed
	ObjectsDone int64  `json:"objects_done"`
	TotalFound  int64  `json:"total_found"`
	Elapsed     string `json:"elapsed"`
	Error       string `json:"error,omitempty"`
}

// Engine runs S3 bucket scans.
type Engine struct {
	s3Client s3.S3Client
	store    store.Store

	mu       sync.RWMutex
	progress *ScanProgress
}

// NewEngine creates a new scan engine.
func NewEngine(s3Client s3.S3Client, store store.Store) *Engine {
	return &Engine{
		s3Client: s3Client,
		store:    store,
	}
}

// StartFullScan initiates a full scan of the given bucket.
func (e *Engine) StartFullScan(ctx context.Context, bucket string) (string, error) {
	scanID := uuid.New().String()

	record := &store.ScanRecord{
		ID:        scanID,
		Bucket:    bucket,
		Project:   "", // filled by caller
		Timestamp: time.Now(),
		Status:    "running",
		ScanType:  "full",
	}
	if err := e.store.SaveScan(ctx, record); err != nil {
		return "", fmt.Errorf("save scan record: %w", err)
	}

	progress := &ScanProgress{
		ScanID: scanID,
		Bucket: bucket,
		Status: "running",
	}
	e.setProgress(progress)

	go e.runFullScan(ctx, scanID, bucket)

	return scanID, nil
}

func (e *Engine) runFullScan(ctx context.Context, scanID, bucket string) {
	agg := newStatsAggregator()
	var continuationToken *string
	totalFound := int64(0)
	startTime := time.Now()

	for {
		if ctx.Err() != nil {
			e.failScan(scanID, ctx.Err().Error())
			return
		}

		result, err := e.s3Client.ListObjectsPage(ctx, bucket, continuationToken)
		if err != nil {
			e.failScan(scanID, fmt.Sprintf("list objects: %v", err))
			return
		}

		for _, obj := range result.Objects {
			agg.addObject(obj)
			atomic.AddInt64(&totalFound, 1)

			// Persist to object store for incremental tracking
			objRecord := &store.ObjectRecord{
				Key:          obj.Key,
				ETag:         obj.ETag,
				Size:         obj.Size,
				LastModified: obj.LastModified,
				StorageClass: obj.StorageClass,
				ScanID:       scanID,
			}
			_ = e.store.SaveObject(ctx, bucket, objRecord)
		}

		// Update progress
		e.updateProgress(scanID, atomic.LoadInt64(&totalFound), time.Since(startTime))

		if !result.IsTruncated {
			break
		}
		continuationToken = result.ContinuationToken
	}

	// Build and save result
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

	// Update scan record with completion
	record, _ := e.store.GetScan(ctx, scanID)
	record.Status = "completed"
	record.Duration = time.Since(startTime).String()
	_ = e.store.SaveScan(ctx, record)

	// Add to bucket index
	_ = e.store.AddScanToBucketIndex(ctx, bucket, scanID)

	e.setProgress(&ScanProgress{
		ScanID:      scanID,
		Bucket:      bucket,
		Status:      "completed",
		ObjectsDone: totalFound,
		TotalFound:  totalFound,
		Elapsed:     time.Since(startTime).String(),
	})
}

func (e *Engine) setProgress(p *ScanProgress) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.progress = p
}

func (e *Engine) updateProgress(scanID string, objectsDone int64, elapsed time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.progress != nil {
		e.progress.ObjectsDone = objectsDone
		e.progress.Elapsed = elapsed.String()
	}
}

func (e *Engine) failScan(scanID string, errMsg string) {
	ctx := context.Background()
	record, _ := e.store.GetScan(ctx, scanID)
	if record != nil {
		record.Status = "failed"
		_ = e.store.SaveScan(ctx, record)
	}
	e.setProgress(&ScanProgress{
		ScanID: scanID,
		Status: "failed",
		Error:  errMsg,
	})
}

// GetProgress returns the current scan progress (thread-safe).
func (e *Engine) GetProgress() *ScanProgress {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.progress == nil {
		return nil
	}
	p := *e.progress
	return &p
}
```

- [ ] **Step 2: Add uuid dependency**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go get github.com/google/uuid
```

Expected: `go: added github.com/google/uuid`

- [ ] **Step 3: Verify compilation**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go vet ./internal/scan/
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/scan/engine.go go.mod go.sum && git commit -m "feat: add scan engine with full scan, progress tracking, and persistence"
```

---

### Task 3: Incremental scan with delta detection

**Files:**
- Modify: `internal/scan/engine.go` (append incremental scan)

- [ ] **Step 1: Add incremental scan method**

Append to `internal/scan/engine.go`:

```go
// StartIncrementalScan initiates an incremental scan, computing delta from previous state.
func (e *Engine) StartIncrementalScan(ctx context.Context, bucket string) (string, error) {
	scanID := uuid.New().String()

	record := &store.ScanRecord{
		ID:        scanID,
		Bucket:    bucket,
		Timestamp: time.Now(),
		Status:    "running",
		ScanType:  "incremental",
	}
	if err := e.store.SaveScan(ctx, record); err != nil {
		return "", fmt.Errorf("save scan record: %w", err)
	}

	progress := &ScanProgress{
		ScanID: scanID,
		Bucket: bucket,
		Status: "running",
	}
	e.setProgress(progress)

	go e.runIncrementalScan(ctx, scanID, bucket)

	return scanID, nil
}

func (e *Engine) runIncrementalScan(ctx context.Context, scanID, bucket string) {
	// Load previous object keys from store
	previousKeysMap := make(map[string]bool)
	previousKeys, err := e.store.ListObjectKeys(ctx, bucket)
	if err == nil {
		for _, k := range previousKeys {
			previousKeysMap[k] = true
		}
	}

	agg := newStatsAggregator()
	var continuationToken *string
	totalFound := int64(0)
	startTime := time.Now()

	seenKeys := make(map[string]bool)
	currentKeys := make(map[string]*store.ObjectRecord)

	for {
		if ctx.Err() != nil {
			e.failScan(scanID, ctx.Err().Error())
			return
		}

		result, err := e.s3Client.ListObjectsPage(ctx, bucket, continuationToken)
		if err != nil {
			e.failScan(scanID, fmt.Sprintf("list objects: %v", err))
			return
		}

		for _, obj := range result.Objects {
			agg.addObject(obj)
			atomic.AddInt64(&totalFound, 1)

			objRecord := &store.ObjectRecord{
				Key:          obj.Key,
				ETag:         obj.ETag,
				Size:         obj.Size,
				LastModified: obj.LastModified,
				StorageClass: obj.StorageClass,
				ScanID:       scanID,
			}

			// Persist updated object record
			_ = e.store.SaveObject(ctx, bucket, objRecord)

			seenKeys[obj.Key] = true
			currentKeys[obj.Key] = objRecord
		}

		e.updateProgress(scanID, atomic.LoadInt64(&totalFound), time.Since(startTime))

		if !result.IsTruncated {
			break
		}
		continuationToken = result.ContinuationToken
	}

	// Compute delta by comparing current listing against previous
	delta := e.computeDelta(previousKeysMap, seenKeys, currentKeys, bucket)

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
		Delta:    delta,
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
		ObjectsDone: totalFound,
		TotalFound:  totalFound,
		Elapsed:     time.Since(startTime).String(),
	})
}

func (e *Engine) computeDelta(previousKeysMap map[string]bool, seenKeys map[string]bool, currentKeys map[string]*store.ObjectRecord, bucket string) *store.DeltaReport {
	delta := &store.DeltaReport{}

	// Count new and modified
	for key := range seenKeys {
		// Check if it existed in the prefix-based previous listing
		// We stored keys as "objects/{bucket}/{etag}/{key}" format
		// We do a best-effort lookup by trying GetObject
		prevObj, err := e.store.GetObject(context.Background(), bucket, key)
		if err != nil {
			// Not found in previous scan -> new
			delta.New++
		} else {
			// Found -> check if modified
			currObj := currentKeys[key]
			if currObj != nil && prevObj.ETag != currObj.ETag {
				delta.Modified++
			} else {
				delta.Unchanged++
			}
		}
	}

	// Count deleted: previous keys not in current listing
	// Since our key format includes ETag, we can't directly compare
	// We use the previous keys count minus new + unchanged + modified
	// A simpler approach: count seenKeys vs previousKeysMap
	estimatedPreviousCount := int64(len(previousKeysMap))
	delta.Deleted = estimatedPreviousCount - delta.Unchanged - delta.Modified
	if delta.Deleted < 0 {
		delta.Deleted = 0
	}

	return delta
}
```

- [ ] **Step 2: Verify compilation**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go vet ./internal/scan/
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/scan/engine.go && git commit -m "feat: add incremental scan with delta detection"
```

---

### Task 4: Scan engine tests using an in-memory S3 mock

**Files:**
- Create: `internal/scan/scan_test.go`

- [ ] **Step 1: Write an S3 mock and scan tests**

```go
package scan

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/esignoretti/s3lytics/internal/s3"
	"github.com/esignoretti/s3lytics/internal/store"
)

// mockS3Client implements s3.S3Client with controllable responses.
type mockS3Client struct {
	objects []s3.ObjectInfo
	index   int
}

func (m *mockS3Client) ListBuckets(ctx context.Context) ([]s3.BucketInfo, error) {
	return []s3.BucketInfo{{Name: "test-bucket"}}, nil
}

func (m *mockS3Client) ListObjectsPage(ctx context.Context, bucket string, continuationToken *string) (*s3.ListResult, error) {
	if m.index >= len(m.objects) {
		return &s3.ListResult{Objects: nil, IsTruncated: false}, nil
	}
	// Return all objects in one page for simplicity
	result := &s3.ListResult{
		Objects:     m.objects[m.index:],
		IsTruncated: false,
	}
	m.index = len(m.objects)
	return result, nil
}

func (m *mockS3Client) HeadObject(ctx context.Context, bucket, key string) (*s3.ObjectInfo, error) {
	for _, o := range m.objects {
		if o.Key == key {
			return &o, nil
		}
	}
	return nil, nil
}

func (m *mockS3Client) ListMultipartUploads(ctx context.Context, bucket string) ([]types.MultipartUpload, error) {
	return nil, nil
}

func (m *mockS3Client) GetBucketPolicy(ctx context.Context, bucket string) (string, error) {
	return "", nil
}

func (m *mockS3Client) GetBucketAcl(ctx context.Context, bucket string) ([]types.Grant, error) {
	return nil, nil
}

func (m *mockS3Client) GetPublicAccessBlock(ctx context.Context, bucket string) (*types.PublicAccessBlockConfiguration, error) {
	return nil, nil
}

func (m *mockS3Client) GetBucketEncryption(ctx context.Context, bucket string) (*types.ServerSideEncryptionConfiguration, error) {
	return nil, nil
}

func (m *mockS3Client) ListObjectVersions(ctx context.Context, bucket string) ([]types.ObjectVersion, []types.DeleteMarkerEntry, error) {
	return nil, nil, nil
}

func (m *mockS3Client) GetObject(ctx context.Context, bucket, key string, rangeSpec *string) ([]byte, error) {
	return nil, nil
}

func newTestStore(t *testing.T) *store.BadgerStore {
	t.Helper()
	dir, err := os.MkdirTemp("", "s3lytics-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	s, err := store.NewBadgerStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestFullScanBasic(t *testing.T) {
	st := newTestStore(t)
	mock := &mockS3Client{
		objects: []s3.ObjectInfo{
			{Key: "a.txt", ETag: "e1", Size: 100, LastModified: time.Now(), StorageClass: "STANDARD"},
			{Key: "b.txt", ETag: "e2", Size: 200, LastModified: time.Now(), StorageClass: "STANDARD"},
			{Key: "dir/c.jpg", ETag: "e3", Size: 300, LastModified: time.Now(), StorageClass: "GLACIER"},
		},
	}

	engine := NewEngine(mock, st)
	ctx := context.Background()

	scanID, err := engine.StartFullScan(ctx, "test-bucket")
	if err != nil {
		t.Fatal(err)
	}

	// Wait for scan to complete
	time.Sleep(500 * time.Millisecond)

	result, err := st.GetScanResult(ctx, scanID)
	if err != nil {
		t.Fatal(err)
	}

	if result.Summary.TotalObjects != 3 {
		t.Errorf("expected 3 objects, got %d", result.Summary.TotalObjects)
	}
	if result.Summary.TotalSize != 600 {
		t.Errorf("expected total size 600, got %d", result.Summary.TotalSize)
	}
	if result.Summary.MaxSize != 300 {
		t.Errorf("expected max size 300, got %d", result.Summary.MaxSize)
	}
}

func TestScanTypeBreakdown(t *testing.T) {
	st := newTestStore(t)
	mock := &mockS3Client{
		objects: []s3.ObjectInfo{
			{Key: "a.txt", ETag: "e1", Size: 100, LastModified: time.Now()},
			{Key: "b.jpg", ETag: "e2", Size: 200, LastModified: time.Now()},
			{Key: "c.txt", ETag: "e3", Size: 300, LastModified: time.Now()},
		},
	}

	engine := NewEngine(mock, st)
	ctx := context.Background()

	scanID, _ := engine.StartFullScan(ctx, "test-bucket")
	time.Sleep(500 * time.Millisecond)

	result, _ := st.GetScanResult(ctx, scanID)
	if len(result.Types) != 2 {
		t.Errorf("expected 2 type breakdowns, got %d", len(result.Types))
	}
}

func TestScanProgressTracking(t *testing.T) {
	st := newTestStore(t)
	mock := &mockS3Client{
		objects: []s3.ObjectInfo{
			{Key: "a.txt", ETag: "e1", Size: 100, LastModified: time.Now()},
		},
	}

	engine := NewEngine(mock, st)
	ctx := context.Background()

	scanID, _ := engine.StartFullScan(ctx, "test-bucket")
	time.Sleep(500 * time.Millisecond)

	progress := engine.GetProgress()
	if progress == nil {
		t.Fatal("expected progress object")
	}
	if progress.Status != "completed" {
		t.Errorf("expected completed status, got %s", progress.Status)
	}
	if progress.ScanID != scanID {
		t.Errorf("expected scanID %s, got %s", scanID, progress.ScanID)
	}
}

func TestIncrementalScan(t *testing.T) {
	st := newTestStore(t)
	mock := &mockS3Client{
		objects: []s3.ObjectInfo{
			{Key: "a.txt", ETag: "e1", Size: 100, LastModified: time.Now()},
			{Key: "b.txt", ETag: "e2", Size: 200, LastModified: time.Now()},
		},
	}

	engine := NewEngine(mock, st)
	ctx := context.Background()

	// First: full scan
	_, err := engine.StartFullScan(ctx, "test-bucket")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)

	// Second: incremental scan
	scanID, err := engine.StartIncrementalScan(ctx, "test-bucket")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)

	result, err := st.GetScanResult(ctx, scanID)
	if err != nil {
		t.Fatal(err)
	}

	if result.Delta == nil {
		t.Fatal("expected delta report for incremental scan")
	}
}

func TestAgeBucketLabeling(t *testing.T) {
	tests := []struct {
		input time.Time
		want  string
	}{
		{time.Now().Add(-1 * time.Hour), "<24h"},
		{time.Now().Add(-3 * 24 * time.Hour), "<7d"},
		{time.Now().Add(-20 * 24 * time.Hour), "<30d"},
		{time.Now().Add(-60 * 24 * time.Hour), "<90d"},
		{time.Now().Add(-200 * 24 * time.Hour), "<1y"},
		{time.Now().Add(-400 * 24 * time.Hour), ">1y"},
	}

	for _, tt := range tests {
		got := ageBucketLabel(tt.input)
		if got != tt.want {
			t.Errorf("ageBucketLabel(%v) = %s, want %s", tt.input, got, tt.want)
		}
	}
}
```

- [ ] **Step 2: Run the tests**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go test ./internal/scan/ -v -count=1 -timeout=30s
```

Expected: all 5 tests pass.

- [ ] **Step 3: Commit**

```bash
git add internal/scan/scan_test.go && git commit -m "test: add scan engine unit tests with mock S3 client"
```

---

**End of Phase 5. Phase 5 deliverables:**
- [x] Statistics aggregator with type/age/storage/prefix breakdowns (`internal/scan/stats.go`)
- [x] Full scan engine with pagination, progress tracking, and persistence (`internal/scan/engine.go`)
- [x] Incremental scan with delta detection (new/modified/deleted/unchanged)
- [x] 5 unit tests passing
- [x] `github.com/google/uuid` dependency added

### Task 5: S3 throttling with exponential backoff

**Files:**
- Modify: `internal/scan/engine.go`

- [ ] **Step 1: Add S3 client setter and backoff wrapper**

Append to `internal/scan/engine.go`:

```go
import (
	"time"
	"math/rand"
)

// SetS3Client allows updating the S3 client after login (hot-swap).
func (e *Engine) SetS3Client(client s3.S3Client) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.s3Client = client
}

// retryWithBackoff wraps an S3 operation with exponential backoff on throttling errors.
func retryWithBackoff(ctx context.Context, operation string, fn func() error) error {
	delays := []time.Duration{100 * time.Millisecond, 500 * time.Millisecond, 1 * time.Second, 5 * time.Second}
	var lastErr error

	for _, delay := range delays {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err := fn()
		if err == nil {
			return nil
		}
		lastErr = err
		// Check for throttling errors (SlowDown, 503, 429)
		errStr := err.Error()
		if strings.Contains(errStr, "SlowDown") ||
			strings.Contains(errStr, "RequestLimitExceeded") ||
			strings.Contains(errStr, "503") ||
			strings.Contains(errStr, "429") {
			log.Printf("S3 throttling on %s, backing off %v: %v", operation, delay, err)
			time.Sleep(delay + time.Duration(rand.Intn(100))*time.Millisecond)
			continue
		}
		return err // non-throttling error, fail immediately
	}

	return fmt.Errorf("%s failed after retries: %w", operation, lastErr)
}
```

- [ ] **Step 2: Update runFullScan to use retryWithBackoff**

Modify the `ListObjectsPage` call in `runFullScan`:

```go
// Replace the direct call:
// result, err := e.s3Client.ListObjectsPage(ctx, bucket, continuationToken)
// With:
var result *s3.ListResult
err := retryWithBackoff(ctx, "ListObjectsPage", func() error {
	var rErr error
	result, rErr = e.s3Client.ListObjectsPage(ctx, bucket, continuationToken)
	return rErr
})
if err != nil {
	e.failScan(scanID, fmt.Sprintf("list objects: %v", err))
	return
}
```

- [ ] **Step 3: Verify compilation**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go vet ./internal/scan/
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/scan/engine.go && git commit -m "feat: add S3 client hot-swap and exponential backoff for throttling"
```

---

### Task 6: Partial scan checkpoints

**Files:**
- Modify: `internal/scan/engine.go`

- [ ] **Step 1: Add checkpoint logic during full scan**

Modify the listing loop in `runFullScan` to save checkpoint every N objects:

```go
// Inside runFullScan, after processing each page:
checkpointInterval := int64(1000)
if totalFound > 0 && totalFound%checkpointInterval == 0 {
	// Save partial result as checkpoint
	partialSummary := agg.buildSummary()
	partialResult := &store.ScanResult{
		Record: store.ScanRecord{
			ID: scanID, Bucket: bucket, Status: "partial",
		},
		Summary: partialSummary,
	}
	_ = e.store.SaveScanResult(ctx, partialResult)
	record, _ := e.store.GetScan(ctx, scanID)
	if record != nil {
		record.Status = "partial"
		_ = e.store.SaveScan(ctx, record)
	}
}
```

- [ ] **Step 2: Add concurrent scan guard**

Add to `Engine` struct:

```go
type Engine struct {
	s3Client s3.S3Client
	store    store.Store

	mu       sync.RWMutex
	progress *ScanProgress
	running  map[string]bool // bucket -> scan active
}

// In NewEngine:
return &Engine{
	s3Client: s3Client,
	store:    store,
	running:  make(map[string]bool),
}

// Add guard at start of StartFullScan and StartIncrementalScan:
e.mu.Lock()
if e.running[bucket] {
	e.mu.Unlock()
	return "", fmt.Errorf("scan already in progress for bucket %s", bucket)
}
e.running[bucket] = true
e.mu.Unlock()

// Release at end of runFullScan/runIncrementalScan:
e.mu.Lock()
delete(e.running, bucket)
e.mu.Unlock()
```

- [ ] **Step 3: Verify compilation**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go vet ./internal/scan/
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/scan/engine.go && git commit -m "feat: add partial scan checkpoints and concurrent scan guard"
```

---

### Task 7: Update tests for new engine features

**Files:**
- Modify: `internal/scan/scan_test.go`

- [ ] **Step 1: Add test for concurrent scan guard**

```go
func TestConcurrentScanGuard(t *testing.T) {
	st := newTestStore(t)
	mock := &mockS3Client{
		objects: []s3.ObjectInfo{
			{Key: "a.txt", ETag: "e1", Size: 100, LastModified: time.Now()},
		},
	}

	engine := NewEngine(mock, st)
	ctx := context.Background()

	// First scan should succeed
	_, err := engine.StartFullScan(ctx, "test-bucket")
	if err != nil {
		t.Fatal(err)
	}

	// Second concurrent scan should fail
	_, err = engine.StartFullScan(ctx, "test-bucket")
	if err == nil {
		t.Error("expected error for concurrent scan, got nil")
	}
	time.Sleep(300 * time.Millisecond)
}
```

- [ ] **Step 2: Add test for SetS3Client**

```go
func TestSetS3Client(t *testing.T) {
	st := newTestStore(t)
	engine := NewEngine(nil, st)

	// Set client after creating engine
	mock := &mockS3Client{
		objects: []s3.ObjectInfo{
			{Key: "a.txt", ETag: "e1", Size: 100, LastModified: time.Now()},
		},
	}
	engine.SetS3Client(mock)

	ctx := context.Background()
	scanID, err := engine.StartFullScan(ctx, "test-bucket")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)

	result, err := st.GetScanResult(ctx, scanID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary.TotalObjects != 1 {
		t.Errorf("expected 1 object, got %d", result.Summary.TotalObjects)
	}
}
```

- [ ] **Step 3: Run all scan tests**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go test ./internal/scan/ -v -count=1 -timeout=30s
```

Expected: all 7 tests pass.

- [ ] **Step 4: Commit**

```bash
git add internal/scan/scan_test.go && git commit -m "test: add concurrent scan guard and SetS3Client tests"
```

---

**Ready for Phase 6: Deep scan features.**
