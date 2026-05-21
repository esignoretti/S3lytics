package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dgraph-io/badger/v4"
)

type Store interface {
	SaveSession(ctx context.Context, data *SessionData) error
	GetSession(ctx context.Context) (*SessionData, error)
	SaveAccount(ctx context.Context, data *AccountData) error
	GetAccount(ctx context.Context) (*AccountData, error)
	ClearAuth(ctx context.Context) error

	SaveProjects(ctx context.Context, projects []Project) error
	GetProjects(ctx context.Context) ([]Project, error)
	SaveBuckets(ctx context.Context, projectID string, buckets []Bucket) error
	GetBuckets(ctx context.Context, projectID string) ([]Bucket, error)

	SaveObject(ctx context.Context, bucket string, obj *ObjectRecord) error
	DeleteObject(ctx context.Context, bucket string, encodedKey string) error
	ListObjectKeys(ctx context.Context, bucket string) ([]string, error)
	GetObject(ctx context.Context, bucket string, encodedKey string) (*ObjectRecord, error)

	SaveScan(ctx context.Context, record *ScanRecord) error
	GetScan(ctx context.Context, id string) (*ScanRecord, error)
	ListScans(ctx context.Context, bucket string) ([]ScanRecord, error)
	DeleteScan(ctx context.Context, id string) error
	SaveScanSummary(ctx context.Context, scanID string, summary *ScanSummary) error
	GetScanSummary(ctx context.Context, scanID string) (*ScanSummary, error)
	SaveScanResult(ctx context.Context, result *ScanResult) error
	GetScanResult(ctx context.Context, scanID string) (*ScanResult, error)

	AddScanToBucketIndex(ctx context.Context, bucket string, scanID string) error
	GetBucketScanIDs(ctx context.Context, bucket string) ([]string, error)

	Close() error
}

type BadgerStore struct {
	db *badger.DB
}

var (
	prefixAuthSession = []byte("auth/session/")
	prefixAuthAccount = []byte("auth/account/")
	keyProjects       = []byte("projects")
	prefixBuckets     = []byte("buckets/")
	prefixObjects     = []byte("objects/")
	prefixScan        = []byte("scans/")
	prefixScanSummary = []byte("scans/summary/")
	prefixScanResult  = []byte("scans/result/")
	prefixBucketIndex = []byte("bucket/index/")
)

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
