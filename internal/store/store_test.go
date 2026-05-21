package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v4"
)

func newTestStore(t *testing.T) *BadgerStore {
	t.Helper()
	dir, err := os.MkdirTemp("", "s3lytics-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	s, err := NewBadgerStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestAuthSessionRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	orig := &SessionData{
		JWT:          "test-jwt",
		RefreshToken: "test-refresh",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
	}

	if err := s.SaveSession(ctx, orig); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetSession(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if got.JWT != orig.JWT {
		t.Errorf("JWT = %q, want %q", got.JWT, orig.JWT)
	}
	if got.RefreshToken != orig.RefreshToken {
		t.Errorf("RefreshToken = %q, want %q", got.RefreshToken, orig.RefreshToken)
	}
}

func TestClearAuth(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_ = s.SaveSession(ctx, &SessionData{JWT: "x"})
	_ = s.SaveAccount(ctx, &AccountData{Email: "x"})

	if err := s.ClearAuth(ctx); err != nil {
		t.Fatal(err)
	}

	if _, err := s.GetSession(ctx); err != badger.ErrKeyNotFound {
		t.Errorf("expected ErrKeyNotFound, got %v", err)
	}
	if _, err := s.GetAccount(ctx); err != badger.ErrKeyNotFound {
		t.Errorf("expected ErrKeyNotFound, got %v", err)
	}
}

func TestProjectsAndBuckets(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	projects := []Project{{ID: "p1", Name: "Project 1"}}
	if err := s.SaveProjects(ctx, projects); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetProjects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "p1" {
		t.Errorf("got %+v, want [p1]", got)
	}

	buckets := []Bucket{{Name: "bucket-1"}}
	if err := s.SaveBuckets(ctx, "p1", buckets); err != nil {
		t.Fatal(err)
	}

	gotBuckets, err := s.GetBuckets(ctx, "p1")
	if err != nil {
		t.Fatal(err)
	}
	if len(gotBuckets) != 1 || gotBuckets[0].Name != "bucket-1" {
		t.Errorf("got %+v, want [bucket-1]", gotBuckets)
	}
}

func TestScanCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	rec := &ScanRecord{
		ID:        "scan-1",
		Bucket:    "test-bucket",
		Project:   "p1",
		Timestamp: time.Now(),
		Status:    "completed",
		ScanType:  "full",
	}

	if err := s.SaveScan(ctx, rec); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetScan(ctx, "scan-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "scan-1" || got.Status != "completed" {
		t.Errorf("got %+v", got)
	}

	scans, err := s.ListScans(ctx, "test-bucket")
	if err != nil {
		t.Fatal(err)
	}
	if len(scans) != 1 {
		t.Errorf("expected 1 scan, got %d", len(scans))
	}
}

func TestScanResult(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	result := &ScanResult{
		Record: ScanRecord{ID: "sr-1", Bucket: "b1"},
		Summary: ScanSummary{
			TotalObjects: 100,
			TotalSize:    1000000,
		},
		Types: []TypeBreakdown{
			{Ext: ".jpg", Count: 50, TotalSize: 500000, Pct: 50.0},
		},
	}

	if err := s.SaveScanResult(ctx, result); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetScanResult(ctx, "sr-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Record.ID != "sr-1" || got.Summary.TotalObjects != 100 {
		t.Errorf("got %+v", got)
	}
	if len(got.Types) != 1 || got.Types[0].Ext != ".jpg" {
		t.Errorf("types = %+v", got.Types)
	}
}

func TestBucketIndex(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.AddScanToBucketIndex(ctx, "b1", "scan-1"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddScanToBucketIndex(ctx, "b1", "scan-2"); err != nil {
		t.Fatal(err)
	}

	ids, err := s.GetBucketScanIDs(ctx, "b1")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 {
		t.Errorf("expected 2 ids, got %d", len(ids))
	}

	_ = s.AddScanToBucketIndex(ctx, "b1", "scan-1")
	ids, _ = s.GetBucketScanIDs(ctx, "b1")
	if len(ids) != 2 {
		t.Errorf("expected still 2 ids after duplicate, got %d", len(ids))
	}
}

func TestObjectTracking(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	obj := &ObjectRecord{
		Key:          "photos/sunset.jpg",
		ETag:         "abc123",
		Size:         50000,
		LastModified: time.Now(),
		StorageClass: "STANDARD",
		ScanID:       "scan-1",
	}

	if err := s.SaveObject(ctx, "bucket-1", obj); err != nil {
		t.Fatal(err)
	}

	keys, err := s.ListObjectKeys(ctx, "bucket-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 {
		t.Errorf("expected 1 object key, got %d", len(keys))
	}

	got, err := s.GetObject(ctx, "bucket-1", "photos/sunset.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if got.ETag != "abc123" || got.Key != "photos/sunset.jpg" {
		t.Errorf("got %+v", got)
	}

	if err := s.DeleteObject(ctx, "bucket-1", "photos/sunset.jpg"); err != nil {
		t.Fatal(err)
	}

	_, err = s.GetObject(ctx, "bucket-1", "photos/sunset.jpg")
	if err != badger.ErrKeyNotFound {
		t.Errorf("expected ErrKeyNotFound after delete, got %v", err)
	}
}
