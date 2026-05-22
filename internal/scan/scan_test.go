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

type mockS3Client struct {
	objects []s3.ObjectInfo
}

func (m *mockS3Client) ListBuckets(ctx context.Context) ([]s3.BucketInfo, error) {
	return []s3.BucketInfo{{Name: "test-bucket"}}, nil
}

func (m *mockS3Client) ListPrefixes(ctx context.Context, bucket, prefix string) ([]string, error) {
	return nil, nil
}

func (m *mockS3Client) ListObjectsPage(ctx context.Context, bucket string, continuationToken *string) (*s3.ListResult, error) {
	return &s3.ListResult{
		Objects:     m.objects,
		IsTruncated: false,
	}, nil
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

func waitForScanComplete(t *testing.T, engine *Engine) {
	t.Helper()
	for i := 0; i < 100; i++ {
		p := engine.GetProgress()
		if p != nil && p.Status == "completed" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("scan did not complete within timeout")
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

	waitForScanComplete(t, engine)

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

	scanID, err := engine.StartFullScan(ctx, "test-bucket")
	if err != nil {
		t.Fatal(err)
	}
	waitForScanComplete(t, engine)

	result, err := st.GetScanResult(ctx, scanID)
	if err != nil {
		t.Fatal(err)
	}
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

	scanID, err := engine.StartFullScan(ctx, "test-bucket")
	if err != nil {
		t.Fatal(err)
	}
	waitForScanComplete(t, engine)

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

	// First: full scan with 2 objects
	_, err := engine.StartFullScan(ctx, "test-bucket")
	if err != nil {
		t.Fatal(err)
	}
	waitForScanComplete(t, engine)

	// Change mock to return a modified set: keep a.txt (same etag), modify b.txt, add c.txt
	mock.objects = []s3.ObjectInfo{
		{Key: "a.txt", ETag: "e1", Size: 100, LastModified: time.Now()},
		{Key: "b.txt", ETag: "e2-modified", Size: 250, LastModified: time.Now()},
		{Key: "c.txt", ETag: "e3", Size: 300, LastModified: time.Now()},
	}

	// Second: incremental scan
	scanID2, err := engine.StartIncrementalScan(ctx, "test-bucket")
	if err != nil {
		t.Fatal(err)
	}
	waitForScanComplete(t, engine)

	result, err := st.GetScanResult(ctx, scanID2)
	if err != nil {
		t.Fatal(err)
	}

	if result.Delta == nil {
		t.Fatal("expected delta report for incremental scan")
	}
	if result.Delta.New != 1 {
		t.Errorf("expected 1 new object (c.txt), got %d", result.Delta.New)
	}
	if result.Delta.Modified != 1 {
		t.Errorf("expected 1 modified (b.txt), got %d", result.Delta.Modified)
	}
	if result.Delta.Unchanged != 1 {
		t.Errorf("expected 1 unchanged (a.txt), got %d", result.Delta.Unchanged)
	}
	if result.Delta.Deleted != 0 {
		t.Errorf("expected 0 deleted, got %d", result.Delta.Deleted)
	}
}

func TestIncrementalScanDeleted(t *testing.T) {
	st := newTestStore(t)
	mock := &mockS3Client{
		objects: []s3.ObjectInfo{
			{Key: "a.txt", ETag: "e1", Size: 100, LastModified: time.Now()},
			{Key: "b.txt", ETag: "e2", Size: 200, LastModified: time.Now()},
		},
	}

	engine := NewEngine(mock, st)
	ctx := context.Background()

	_, err := engine.StartFullScan(ctx, "test-bucket")
	if err != nil {
		t.Fatal(err)
	}
	waitForScanComplete(t, engine)

	// Remove b.txt from the listing
	mock.objects = []s3.ObjectInfo{
		{Key: "a.txt", ETag: "e1", Size: 100, LastModified: time.Now()},
	}

	scanID2, err := engine.StartIncrementalScan(ctx, "test-bucket")
	if err != nil {
		t.Fatal(err)
	}
	waitForScanComplete(t, engine)

	result, err := st.GetScanResult(ctx, scanID2)
	if err != nil {
		t.Fatal(err)
	}
	if result.Delta == nil {
		t.Fatal("expected delta report")
	}
	if result.Delta.Deleted != 1 {
		t.Errorf("expected 1 deleted (b.txt), got %d", result.Delta.Deleted)
	}
	if result.Delta.New != 0 {
		t.Errorf("expected 0 new, got %d", result.Delta.New)
	}
	if result.Delta.Modified != 0 {
		t.Errorf("expected 0 modified, got %d", result.Delta.Modified)
	}
	if result.Delta.Unchanged != 1 {
		t.Errorf("expected 1 unchanged (a.txt), got %d", result.Delta.Unchanged)
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

func TestConcurrentScanGuard(t *testing.T) {
	st := newTestStore(t)
	mock := &mockS3Client{
		objects: []s3.ObjectInfo{
			{Key: "a.txt", ETag: "e1", Size: 100, LastModified: time.Now()},
		},
	}

	engine := NewEngine(mock, st)
	ctx := context.Background()

	_, err := engine.StartFullScan(ctx, "test-bucket")
	if err != nil {
		t.Fatal(err)
	}

	_, err = engine.StartFullScan(ctx, "test-bucket")
	if err == nil {
		t.Error("expected error for concurrent scan, got nil")
	}

	waitForScanComplete(t, engine)
}

func TestSetS3Client(t *testing.T) {
	st := newTestStore(t)
	engine := NewEngine(nil, st)

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
	waitForScanComplete(t, engine)

	result, err := st.GetScanResult(ctx, scanID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary.TotalObjects != 1 {
		t.Errorf("expected 1 object, got %d", result.Summary.TotalObjects)
	}
}

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
