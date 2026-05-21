# S3lytics — Phase 2: BadgerDB Store Layer & Data Models

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the BadgerDB store abstraction and data model types used throughout the application.

**Architecture:** A `store` package under `internal/store/` owns all BadgerDB access. Data model types live in `internal/store/models.go`. The store exposes a `Store` interface + `BadgerStore` implementation. All encoding uses `encoding/json` (pre-encoded values stored as bytes). Key prefixes follow the schema from the design doc.

**Tech Stack:** `github.com/dgraph-io/badger/v4`, `encoding/json`

**Pre-requisites:** Phase 1 complete — module initialized, directories exist.

---

### Task 1: Add BadgerDB dependency and define data models

**Files:**
- Create: `internal/store/models.go`

- [ ] **Step 1: Add badger dependency**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go get github.com/dgraph-io/badger/v4
```

Expected: `go: added github.com/dgraph-io/badger/v4`

- [ ] **Step 2: Write models.go**

```go
package store

import "time"

// --- Auth ---

type SessionData struct {
	JWT          string    `json:"jwt"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type AccountData struct {
	EndpointGateway string `json:"endpoint_gateway"`
	Email           string `json:"email"`
	UserID          string `json:"user_id"`
}

// --- Projects & Buckets ---

type Project struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Bucket struct {
	Name         string `json:"name"`
	CreationDate string `json:"creation_date,omitempty"`
}

// --- Objects ---

type ObjectRecord struct {
	Key          string    `json:"key"`
	ETag         string    `json:"etag"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"last_modified"`
	StorageClass string    `json:"storage_class"`
	ScanID       string    `json:"scan_id"`
}

// --- Scans ---

type ScanRecord struct {
	ID        string    `json:"id"`
	Bucket    string    `json:"bucket"`
	Project   string    `json:"project"`
	Timestamp time.Time `json:"timestamp"`
	Duration  string    `json:"duration"`
	Status    string    `json:"status"`       // pending, running, completed, failed, partial
	ScanType  string    `json:"scan_type"`    // full, incremental
}

type ScanSummary struct {
	TotalObjects int64   `json:"total_objects"`
	TotalSize    int64   `json:"total_size"`
	AvgSize      float64 `json:"avg_size"`
	MedianSize   int64   `json:"median_size"`
	MaxSize      int64   `json:"max_size"`
	EmptyObjects int64   `json:"empty_objects"`
}

type TypeBreakdown struct {
	Ext       string  `json:"ext"`
	Count     int64   `json:"count"`
	TotalSize int64   `json:"total_size"`
	Pct       float64 `json:"pct"`
}

type AgeBucket struct {
	Label string `json:"label"`
	Count int64  `json:"count"`
	Size  int64  `json:"size"`
}

type StorageBreakdown struct {
	Class string `json:"class"`
	Count int64  `json:"count"`
	Size  int64  `json:"size"`
}

type PrefixBreakdown struct {
	Prefix string `json:"prefix"`
	Count  int64  `json:"count"`
	Size   int64  `json:"size"`
}

type DeltaReport struct {
	New       int64 `json:"new"`
	Modified  int64 `json:"modified"`
	Deleted   int64 `json:"deleted"`
	Unchanged int64 `json:"unchanged"`
}

// --- Deep Scan Results ---

type DuplicateGroup struct {
	ETag      string   `json:"etag"`
	Count     int      `json:"count"`
	TotalSize int64    `json:"total_size"`
	Keys      []string `json:"keys"`
}

type MultipartUpload struct {
	UploadID  string    `json:"upload_id"`
	Key       string    `json:"key"`
	Initiated time.Time `json:"initiated"`
	Size      int64     `json:"size"`
}

type AccessFinding struct {
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Detail   string `json:"detail"`
}

type DeepAccessAudit struct {
	PublicAccessBlocked bool            `json:"public_access_blocked"`
	BucketPolicy        string          `json:"bucket_policy"`
	Findings            []AccessFinding `json:"findings"`
}

type DeepEncryption struct {
	EncryptedPct    float64  `json:"encrypted_pct"`
	Algorithms      []string `json:"algorithms"`
	UnencryptedKeys []string `json:"unencrypted_keys"`
}

type DeepVersioning struct {
	TotalVersions    int64 `json:"total_versions"`
	NonCurrentCount  int64 `json:"non_current_count"`
	WastedBytes      int64 `json:"wasted_bytes"`
}

type LargeFile struct {
	Key          string    `json:"key"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"last_modified"`
}

type DeepNaming struct {
	Pattern           string   `json:"pattern"`
	NonCompliantCount int      `json:"non_compliant_count"`
	Examples          []string `json:"examples"`
}

type CostBreakdown struct {
	Class       string  `json:"class"`
	MonthlyCost float64 `json:"monthly_cost"`
}

type DeepCostEstimate struct {
	MonthlyTotal float64         `json:"monthly_total"`
	Breakdown    []CostBreakdown `json:"breakdown"`
}

type VirusResult struct {
	Status   string   `json:"status"`   // skipped, completed, error
	Scanned  int      `json:"scanned"`
	Infected []string `json:"infected"`
	Errors   []string `json:"errors"`
}

// --- Scan Result (top-level container for one scan) ---

type ScanResult struct {
	Record ScanRecord `json:"record"`
	Summary ScanSummary `json:"summary"`
	Types  []TypeBreakdown  `json:"types,omitempty"`
	Ages   []AgeBucket      `json:"ages,omitempty"`
	Storage []StorageBreakdown `json:"storage,omitempty"`
	Prefixes []PrefixBreakdown  `json:"prefixes,omitempty"`
	Delta  *DeltaReport      `json:"delta,omitempty"`

	// Deep scan results
	Duplicates   []DuplicateGroup  `json:"duplicates,omitempty"`
	Multiparts   []MultipartUpload `json:"multiparts,omitempty"`
	AccessAudit  *DeepAccessAudit  `json:"access_audit,omitempty"`
	Encryption   *DeepEncryption   `json:"encryption,omitempty"`
	Versioning   *DeepVersioning   `json:"versioning,omitempty"`
	LargeFiles   []LargeFile       `json:"large_files,omitempty"`
	Naming       *DeepNaming       `json:"naming,omitempty"`
	CostEstimate *DeepCostEstimate `json:"cost_estimate,omitempty"`
	Virus        *VirusResult      `json:"virus,omitempty"`
}
```

- [ ] **Step 3: Verify it compiles (models-only check, no tests yet)**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go vet ./internal/store/
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/store/models.go go.mod go.sum && git commit -m "feat: add data model types for auth, scans, deep scan results"
```

---

### Task 2: Store interface and BadgerStore open/close

**Files:**
- Create: `internal/store/store.go`

- [ ] **Step 1: Write the Store interface and BadgerStore**

```go
package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dgraph-io/badger/v4"
)

// Store defines the persistence contract.
type Store interface {
	// Auth
	SaveSession(ctx context.Context, data *SessionData) error
	GetSession(ctx context.Context) (*SessionData, error)
	SaveAccount(ctx context.Context, data *AccountData) error
	GetAccount(ctx context.Context) (*AccountData, error)
	ClearAuth(ctx context.Context) error

	// Projects & buckets
	SaveProjects(ctx context.Context, projects []Project) error
	GetProjects(ctx context.Context) ([]Project, error)
	SaveBuckets(ctx context.Context, projectID string, buckets []Bucket) error
	GetBuckets(ctx context.Context, projectID string) ([]Bucket, error)

	// Objects (for incremental scan tracking)
	SaveObject(ctx context.Context, bucket string, obj *ObjectRecord) error
	DeleteObject(ctx context.Context, bucket string, encodedKey string) error
	ListObjectKeys(ctx context.Context, bucket string) ([]string, error)
	GetObject(ctx context.Context, bucket string, encodedKey string) (*ObjectRecord, error)

	// Scans
	SaveScan(ctx context.Context, record *ScanRecord) error
	GetScan(ctx context.Context, id string) (*ScanRecord, error)
	ListScans(ctx context.Context, bucket string) ([]ScanRecord, error)
	DeleteScan(ctx context.Context, id string) error
	SaveScanSummary(ctx context.Context, scanID string, summary *ScanSummary) error
	GetScanSummary(ctx context.Context, scanID string) (*ScanSummary, error)
	SaveScanResult(ctx context.Context, result *ScanResult) error
	GetScanResult(ctx context.Context, scanID string) (*ScanResult, error)

	// Bucket scan index
	AddScanToBucketIndex(ctx context.Context, bucket string, scanID string) error
	GetBucketScanIDs(ctx context.Context, bucket string) ([]string, error)

	// Lifecycle
	Close() error
}

// BadgerStore implements Store backed by BadgerDB.
type BadgerStore struct {
	db *badger.DB
}

// prefix helpers
var (
	prefixAuthSession        = []byte("auth/session/")
	prefixAuthAccount        = []byte("auth/account/")
	keyProjects              = []byte("projects")
	prefixBuckets            = []byte("buckets/")
	prefixObjects            = []byte("objects/")
	prefixScan               = []byte("scans/")
	prefixScanSummary        = []byte("scans/summary/")
	prefixScanResult         = []byte("scans/result/")
	prefixBucketIndex        = []byte("bucket/index/")
)

// NewBadgerStore opens (or creates) a BadgerDB at dir.
func NewBadgerStore(dir string) (*BadgerStore, error) {
	opts := badger.DefaultOptions(dir).
		WithLogger(nil).
		WithSyncWrites(false)

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("badger open: %w", err)
	}
	return &BadgerStore{db: db}, nil
}

func (s *BadgerStore) Close() error {
	return s.db.Close()
}

// --- helpers ---

func (s *BadgerStore) get(ctx context.Context, key []byte) ([]byte, error) {
	var val []byte
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}
		val, err = item.ValueCopy(nil)
		return err
	})
	if err != nil {
		return nil, err
	}
	return val, nil
}

func (s *BadgerStore) set(ctx context.Context, key []byte, val interface{}) error {
	data, err := json.Marshal(val)
	if err != nil {
		return err
	}
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, data)
	})
}

func (s *BadgerStore) del(ctx context.Context, key []byte) error {
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Delete(key)
	})
}

func (s *BadgerStore) iterateKeys(ctx context.Context, prefix []byte) ([]string, error) {
	var keys []string
	err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			key := string(it.Item().KeyCopy(nil))
			keys = append(keys, key)
		}
		return nil
	})
	return keys, err
}

func (s *BadgerStore) iterateValues(ctx context.Context, prefix []byte) ([][]byte, error) {
	var vals [][]byte
	err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			val, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}
			vals = append(vals, val)
		}
		return nil
	})
	return vals, err
}
```

- [ ] **Step 2: Verify it compiles**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go vet ./internal/store/
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/store/store.go && git commit -m "feat: add Store interface and BadgerStore open/close/helpers"
```

---

### Task 3: Auth store methods

**Files:**
- Modify: `internal/store/store.go` (append methods)

- [ ] **Step 1: Write auth store methods**

Append to `internal/store/store.go`:

```go
// --- Auth methods ---

func (s *BadgerStore) SaveSession(ctx context.Context, data *SessionData) error {
	return s.set(ctx, prefixAuthSession, data)
}

func (s *BadgerStore) GetSession(ctx context.Context) (*SessionData, error) {
	data, err := s.get(ctx, prefixAuthSession)
	if err != nil {
		return nil, err
	}
	var sd SessionData
	if err := json.Unmarshal(data, &sd); err != nil {
		return nil, err
	}
	return &sd, nil
}

func (s *BadgerStore) SaveAccount(ctx context.Context, data *AccountData) error {
	return s.set(ctx, prefixAuthAccount, data)
}

func (s *BadgerStore) GetAccount(ctx context.Context) (*AccountData, error) {
	data, err := s.get(ctx, prefixAuthAccount)
	if err != nil {
		return nil, err
	}
	var ad AccountData
	if err := json.Unmarshal(data, &ad); err != nil {
		return nil, err
	}
	return &ad, nil
}

func (s *BadgerStore) ClearAuth(ctx context.Context) error {
	if err := s.del(ctx, prefixAuthSession); err != nil && err != badger.ErrKeyNotFound {
		return err
	}
	if err := s.del(ctx, prefixAuthAccount); err != nil && err != badger.ErrKeyNotFound {
		return err
	}
	return nil
}
```

- [ ] **Step 2: Verify compilation**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go vet ./internal/store/
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/store/store.go && git commit -m "feat: add auth store methods (session, account, clear)"
```

---

### Task 4: Projects & buckets store methods

**Files:**
- Modify: `internal/store/store.go` (append methods)

- [ ] **Step 1: Write projects/buckets methods**

Append to `internal/store/store.go`:

```go
// --- Projects & buckets ---

func (s *BadgerStore) SaveProjects(ctx context.Context, projects []Project) error {
	return s.set(ctx, keyProjects, projects)
}

func (s *BadgerStore) GetProjects(ctx context.Context) ([]Project, error) {
	data, err := s.get(ctx, keyProjects)
	if err != nil {
		return nil, err
	}
	var projects []Project
	if err := json.Unmarshal(data, &projects); err != nil {
		return nil, err
	}
	return projects, nil
}

func (s *BadgerStore) SaveBuckets(ctx context.Context, projectID string, buckets []Bucket) error {
	key := append(prefixBuckets, []byte(projectID)...)
	return s.set(ctx, key, buckets)
}

func (s *BadgerStore) GetBuckets(ctx context.Context, projectID string) ([]Bucket, error) {
	key := append(prefixBuckets, []byte(projectID)...)
	data, err := s.get(ctx, key)
	if err != nil {
		return nil, err
	}
	var buckets []Bucket
	if err := json.Unmarshal(data, &buckets); err != nil {
		return nil, err
	}
	return buckets, nil
}
```

- [ ] **Step 2: Verify compilation**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go vet ./internal/store/
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/store/store.go && git commit -m "feat: add projects and buckets store methods"
```

---

### Task 5: Object tracking store methods

**Files:**
- Modify: `internal/store/store.go` (append methods)

- [ ] **Step 1: Write object tracking methods**

Append to `internal/store/store.go`:

```go
// --- Object tracking ---

func objectKey(bucket string, encodedKey string) []byte {
	return append(prefixObjects, []byte(bucket+"/"+encodedKey)...)
}

func (s *BadgerStore) SaveObject(ctx context.Context, bucket string, obj *ObjectRecord) error {
	// Use ETag + "/" + encodedKey as the suffix
	key := objectKey(bucket, obj.ETag+"/"+obj.Key)
	return s.set(ctx, key, obj)
}

func (s *BadgerStore) DeleteObject(ctx context.Context, bucket string, encodedKey string) error {
	// We need the full key. Use iterateKeys with prefix to find and delete.
	prefix := append(prefixObjects, []byte(bucket+"/")...)
	keys, err := s.iterateKeys(ctx, prefix)
	if err != nil {
		return err
	}
	for _, k := range keys {
		// k format: "objects/{bucket}/{etag}/{key}"
		// We encoded as objects/{bucket}/{etag}/{key}
		// The encodedKey is the original object key
		// We need to find the key that ends with the given encodedKey
		if len(k) > len(prefix) && k[len(prefix):] == encodedKey {
			return s.del(ctx, []byte(k))
		}
		// Also check if encodedKey contains ETag+"/"+key
		if containsSuffix(k, encodedKey) {
			return s.del(ctx, []byte(k))
		}
	}
	return nil
}

func containsSuffix(s, suffix string) bool {
	if len(s) < len(suffix) {
		return false
	}
	return s[len(s)-len(suffix):] == suffix
}

func (s *BadgerStore) ListObjectKeys(ctx context.Context, bucket string) ([]string, error) {
	prefix := append(prefixObjects, []byte(bucket+"/")...)
	return s.iterateKeys(ctx, prefix)
}

func (s *BadgerStore) GetObject(ctx context.Context, bucket string, encodedKey string) (*ObjectRecord, error) {
	// Need to find it by iterating with prefix
	prefix := append(prefixObjects, []byte(bucket+"/")...)
	vals, err := s.iterateValues(ctx, prefix)
	if err != nil {
		return nil, err
	}
	for _, data := range vals {
		var obj ObjectRecord
		if err := json.Unmarshal(data, &obj); err != nil {
			continue
		}
		if obj.Key == encodedKey {
			return &obj, nil
		}
	}
	return nil, badger.ErrKeyNotFound
}
```

- [ ] **Step 2: Verify compilation**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go vet ./internal/store/
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/store/store.go && git commit -m "feat: add object tracking store methods"
```

---

### Task 6: Scan CRUD and bucket index store methods

**Files:**
- Modify: `internal/store/store.go` (append methods)

- [ ] **Step 1: Write scan CRUD methods**

Append to `internal/store/store.go`:

```go
// --- Scans ---

func scanKey(id string) []byte {
	return append(prefixScan, []byte(id)...)
}

func scanSummaryKey(scanID string) []byte {
	return append(prefixScanSummary, []byte(scanID)...)
}

func scanResultKey(scanID string) []byte {
	return append(prefixScanResult, []byte(scanID)...)
}

func (s *BadgerStore) SaveScan(ctx context.Context, record *ScanRecord) error {
	return s.set(ctx, scanKey(record.ID), record)
}

func (s *BadgerStore) GetScan(ctx context.Context, id string) (*ScanRecord, error) {
	data, err := s.get(ctx, scanKey(id))
	if err != nil {
		return nil, err
	}
	var rec ScanRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

func (s *BadgerStore) ListScans(ctx context.Context, bucket string) ([]ScanRecord, error) {
	vals, err := s.iterateValues(ctx, prefixScan)
	if err != nil {
		return nil, err
	}
	var scans []ScanRecord
	for _, data := range vals {
		var rec ScanRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			continue
		}
		if bucket == "" || rec.Bucket == bucket {
			scans = append(scans, rec)
		}
	}
	return scans, nil
}

func (s *BadgerStore) DeleteScan(ctx context.Context, id string) error {
	// Delete scan record, summary, result
	if err := s.del(ctx, scanKey(id)); err != nil && err != badger.ErrKeyNotFound {
		return err
	}
	if err := s.del(ctx, scanSummaryKey(id)); err != nil && err != badger.ErrKeyNotFound {
		return err
	}
	if err := s.del(ctx, scanResultKey(id)); err != nil && err != badger.ErrKeyNotFound {
		return err
	}
	return nil
}

func (s *BadgerStore) SaveScanSummary(ctx context.Context, scanID string, summary *ScanSummary) error {
	return s.set(ctx, scanSummaryKey(scanID), summary)
}

func (s *BadgerStore) GetScanSummary(ctx context.Context, scanID string) (*ScanSummary, error) {
	data, err := s.get(ctx, scanSummaryKey(scanID))
	if err != nil {
		return nil, err
	}
	var summary ScanSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return nil, err
	}
	return &summary, nil
}

func (s *BadgerStore) SaveScanResult(ctx context.Context, result *ScanResult) error {
	return s.set(ctx, scanResultKey(result.Record.ID), result)
}

func (s *BadgerStore) GetScanResult(ctx context.Context, scanID string) (*ScanResult, error) {
	data, err := s.get(ctx, scanResultKey(scanID))
	if err != nil {
		return nil, err
	}
	var result ScanResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// --- Bucket scan index ---

func (s *BadgerStore) AddScanToBucketIndex(ctx context.Context, bucket string, scanID string) error {
	key := append(prefixBucketIndex, []byte(bucket)...)
	var ids []string
	existing, err := s.get(ctx, key)
	if err == nil {
		if err := json.Unmarshal(existing, &ids); err != nil {
			ids = nil
		}
	}
	// Avoid duplicates
	for _, id := range ids {
		if id == scanID {
			return nil
		}
	}
	ids = append(ids, scanID)
	return s.set(ctx, key, ids)
}

func (s *BadgerStore) GetBucketScanIDs(ctx context.Context, bucket string) ([]string, error) {
	key := append(prefixBucketIndex, []byte(bucket)...)
	data, err := s.get(ctx, key)
	if err != nil {
		return nil, err
	}
	var ids []string
	if err := json.Unmarshal(data, &ids); err != nil {
		return nil, err
	}
	return ids, nil
}
```

- [ ] **Step 2: Verify compilation**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go vet ./internal/store/
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/store/store.go && git commit -m "feat: add scan CRUD and bucket index store methods"
```

---

### Task 7: Store unit tests

**Files:**
- Create: `internal/store/store_test.go`

- [ ] **Step 1: Write store tests**

```go
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

	// List scans
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

	// Add duplicate should not create new entry
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
```

- [ ] **Step 2: Run the tests**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go test ./internal/store/ -v -count=1 -timeout=30s
```

Expected: all 6 tests pass.

- [ ] **Step 3: Commit**

```bash
git add internal/store/store_test.go && git commit -m "test: add store layer unit tests covering auth, projects, scans, objects, bucket index"
```

---

**End of Phase 2. Phase 2 deliverables:**
- [x] Data model types (`internal/store/models.go`)
- [x] `Store` interface + `BadgerStore` implementation (`internal/store/store.go`)
- [x] Auth store methods (session, account, clear)
- [x] Projects & buckets store methods
- [x] Object tracking store methods
- [x] Scan CRUD + bucket index methods
- [x] 6 unit tests passing
- [x] BadgerDB dependency added

**Ready for Phase 3: S3 client abstraction and operations.**
