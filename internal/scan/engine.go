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

type ScanProgress struct {
	ScanID      string `json:"scan_id"`
	Bucket      string `json:"bucket"`
	Status      string `json:"status"`
	ObjectsDone int64  `json:"objects_done"`
	TotalFound  int64  `json:"total_found"`
	Elapsed     string `json:"elapsed"`
	Error       string `json:"error,omitempty"`
}

type Engine struct {
	client   s3.S3Client
	store    store.Store

	mu       sync.RWMutex
	progress *ScanProgress
	running  map[string]bool
}

func NewEngine(client s3.S3Client, s store.Store) *Engine {
	return &Engine{
		client:  client,
		store:   s,
		running: make(map[string]bool),
	}
}

func (e *Engine) SetS3Client(client s3.S3Client) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.client = client
}

func (e *Engine) StartFullScan(ctx context.Context, bucket string) (string, error) {
	e.mu.Lock()
	if e.client == nil {
		e.mu.Unlock()
		return "", fmt.Errorf("S3 client not set — log in first")
	}
	if e.running[bucket] {
		e.mu.Unlock()
		return "", fmt.Errorf("scan already in progress for bucket %s", bucket)
	}
	e.running[bucket] = true
	e.mu.Unlock()

	scanID := uuid.New().String()

	record := &store.ScanRecord{
		ID:        scanID,
		Bucket:    bucket,
		Timestamp: time.Now(),
		Status:    "running",
		ScanType:  "full",
	}
	if err := e.store.SaveScan(ctx, record); err != nil {
		e.mu.Lock()
		delete(e.running, bucket)
		e.mu.Unlock()
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
	defer func() {
		e.mu.Lock()
		delete(e.running, bucket)
		e.mu.Unlock()
	}()

	agg := newStatsAggregator()
	var continuationToken *string
	totalFound := int64(0)
	startTime := time.Now()

	for {
		if ctx.Err() != nil {
			e.failScan(scanID, ctx.Err().Error())
			return
		}

		var page *s3.ListResult
		pageErr := retryWithBackoff(ctx, "ListObjectsPage", func() error {
			var rErr error
			page, rErr = e.client.ListObjectsPage(ctx, bucket, continuationToken)
			return rErr
		})
		if pageErr != nil {
			e.failScan(scanID, fmt.Sprintf("list objects: %v", pageErr))
			return
		}
		result := page

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
			_ = e.store.SaveObject(ctx, bucket, objRecord)
		}

		e.updateProgress(scanID, atomic.LoadInt64(&totalFound), time.Since(startTime))

		if !result.IsTruncated {
			break
		}
		continuationToken = result.ContinuationToken
	}

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

func (e *Engine) GetProgress() *ScanProgress {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.progress == nil {
		return nil
	}
	p := *e.progress
	return &p
}

func (e *Engine) StartIncrementalScan(ctx context.Context, bucket string) (string, error) {
	e.mu.Lock()
	if e.client == nil {
		e.mu.Unlock()
		return "", fmt.Errorf("S3 client not set — log in first")
	}
	if e.running[bucket] {
		e.mu.Unlock()
		return "", fmt.Errorf("scan already in progress for bucket %s", bucket)
	}
	e.running[bucket] = true
	e.mu.Unlock()

	scanID := uuid.New().String()

	record := &store.ScanRecord{
		ID:        scanID,
		Bucket:    bucket,
		Timestamp: time.Now(),
		Status:    "running",
		ScanType:  "incremental",
	}
	if err := e.store.SaveScan(ctx, record); err != nil {
		e.mu.Lock()
		delete(e.running, bucket)
		e.mu.Unlock()
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
	defer func() {
		e.mu.Lock()
		delete(e.running, bucket)
		e.mu.Unlock()
	}()

	// Load all previous object keys and build a map of key -> previous object
	// ListObjectKeys returns full Badger keys: "objects/{bucket}/{etag}/{key}"
	previousKeys, _ := e.store.ListObjectKeys(ctx, bucket)
	previousObjects := make(map[string]*store.ObjectRecord)
	for _, k := range previousKeys {
		// Extract object key from "objects/{bucket}/{etag}/{key}"
		prefix := "objects/" + bucket + "/"
		if len(k) > len(prefix) {
			suffix := k[len(prefix):] // "{etag}/{key}"
			if slashIdx := strings.IndexByte(suffix, '/'); slashIdx >= 0 {
				objKey := suffix[slashIdx+1:]
				obj, err := e.store.GetObject(ctx, bucket, objKey)
				if err == nil && obj != nil {
					previousObjects[obj.Key] = obj
				}
			}
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

		var page *s3.ListResult
		pageErr := retryWithBackoff(ctx, "ListObjectsPage", func() error {
			var rErr error
			page, rErr = e.client.ListObjectsPage(ctx, bucket, continuationToken)
			return rErr
		})
		if pageErr != nil {
			e.failScan(scanID, fmt.Sprintf("list objects: %v", pageErr))
			return
		}
		result := page

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

	delta := e.computeDelta(previousObjects, seenKeys, currentKeys)

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

func (e *Engine) computeDelta(previousObjects map[string]*store.ObjectRecord, seenKeys map[string]bool, currentKeys map[string]*store.ObjectRecord) *store.DeltaReport {
	delta := &store.DeltaReport{}

	for key := range seenKeys {
		prevObj, existed := previousObjects[key]
		if !existed {
			delta.New++
		} else {
			currObj := currentKeys[key]
			if currObj != nil && prevObj.ETag != currObj.ETag {
				delta.Modified++
			} else {
				delta.Unchanged++
			}
		}
	}

	// Deleted = objects that were in previous scan but not in current listing
	for key := range previousObjects {
		if !seenKeys[key] {
			delta.Deleted++
		}
	}

	return delta
}

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
		errStr := err.Error()
		if strings.Contains(errStr, "SlowDown") ||
			strings.Contains(errStr, "RequestLimitExceeded") ||
			strings.Contains(errStr, "503") ||
			strings.Contains(errStr, "429") {
			log.Printf("S3 throttling on %s, backing off %v: %v", operation, delay, err)
			time.Sleep(delay + time.Duration(rand.Intn(100))*time.Millisecond)
			continue
		}
		return err
	}

	return fmt.Errorf("%s failed after retries: %w", operation, lastErr)
}
