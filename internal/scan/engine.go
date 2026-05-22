package scan

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"sync"
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

type Config struct {
	Workers       int
	BatchSize     int
	PrefixTimeout time.Duration
}

type collectorMsg struct {
	obj *store.ObjectRecord
	err error
}

type pageResult struct {
	page *s3.ListResult
	err  error
}

type Engine struct {
	client   s3.S3Client
	store    store.Store

	mu       sync.RWMutex
	progress *ScanProgress
	running  map[string]bool
	config   Config
}

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

func (e *Engine) SetS3Client(client s3.S3Client) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.client = client
}

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

func (e *Engine) saveObjectBatch(ctx context.Context, batch []*store.ObjectRecord, bucket string) error {
	if len(batch) == 0 {
		return nil
	}
	return e.store.SaveObjects(ctx, bucket, batch)
}

func (e *Engine) discoverPrefixes(ctx context.Context, bucket string) ([]string, error) {
	timeout := e.config.PrefixTimeout
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	prefixes, err := e.client.ListPrefixes(ctx, bucket, "")
	if err != nil {
		return nil, err
	}
	if len(prefixes) == 0 {
		return []string{""}, nil
	}
	return prefixes, nil
}

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

func (e *Engine) runWorker(ctx context.Context, bucket, prefix string, objChan chan<- collectorMsg) {
	var continuationToken *string
	isTruncated := true

	for isTruncated {
		if ctx.Err() != nil {
			return
		}

		page, err := e.client.ListObjectsPage(ctx, bucket, continuationToken)
		if err != nil {
			objChan <- collectorMsg{err: err}
			return
		}

		for _, obj := range page.Objects {
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

		isTruncated = page.IsTruncated
		continuationToken = page.ContinuationToken
	}
}

func (e *Engine) runCollector(ctx context.Context, scanID, bucket string, objChan <-chan collectorMsg, previousObjects map[string]*store.ObjectRecord) (*statsAggregator, *store.DeltaReport, error) {
	agg := newStatsAggregator()
	batch := make([]*store.ObjectRecord, 0, e.config.BatchSize)
	startTime := time.Now()

	var seenKeys map[string]bool
	var currentKeys map[string]*store.ObjectRecord
	if previousObjects != nil {
		seenKeys = make(map[string]bool)
		currentKeys = make(map[string]*store.ObjectRecord)
	}

	flushBatch := func() error {
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
			return err
		}
		e.updateProgress(scanID, agg.totalObjects, time.Since(startTime))
		batch = batch[:0]
		return nil
	}

	for {
		select {
		case msg, ok := <-objChan:
			if !ok {
				if err := flushBatch(); err != nil {
					return nil, nil, err
				}
				var delta *store.DeltaReport
				if previousObjects != nil {
					delta = e.computeDelta(previousObjects, seenKeys, currentKeys)
				}
				return agg, delta, nil
			}
			if msg.err != nil {
				return nil, nil, msg.err
			}
			msg.obj.ScanID = scanID
			batch = append(batch, msg.obj)
			if seenKeys != nil {
				seenKeys[msg.obj.Key] = true
				currentKeys[msg.obj.Key] = msg.obj
			}
			if len(batch) >= e.config.BatchSize {
				if err := flushBatch(); err != nil {
					return nil, nil, err
				}
			}
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		}
	}
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

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	prefixChan := make(chan string, e.config.Workers)
	objChan := make(chan collectorMsg, 1000)
	startTime := time.Now()

	type collResult struct {
		agg   *statsAggregator
		delta *store.DeltaReport
		err   error
	}
	collCh := make(chan collResult, 1)
	go func() {
		agg, delta, err := e.runCollector(ctx, scanID, bucket, objChan, nil)
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
	close(objChan)

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
	startTime := time.Now()

	type collResult struct {
		agg   *statsAggregator
		delta *store.DeltaReport
		err   error
	}
	collCh := make(chan collResult, 1)
	go func() {
		agg, delta, err := e.runCollector(ctx, scanID, bucket, objChan, previousObjects)
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
	close(objChan)

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
