package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dgraph-io/badger/v4"
)

type Store interface {
	SaveSession(ctx context.Context, data *SessionData) error
	GetSession(ctx context.Context) (*SessionData, error)
	SaveAccount(ctx context.Context, data *AccountData) error
	GetAccount(ctx context.Context) (*AccountData, error)
	ClearAuth(ctx context.Context) error

	SaveS3Credential(ctx context.Context, cred *S3Credential) error
	GetS3Credential(ctx context.Context, name string) (*S3Credential, error)
	DeleteS3Credential(ctx context.Context, name string) error
	ListS3Credentials(ctx context.Context) ([]*S3Credential, error)

	SaveProjects(ctx context.Context, projects []Project) error
	GetProjects(ctx context.Context) ([]Project, error)
	SaveBuckets(ctx context.Context, projectID string, buckets []Bucket) error
	GetBuckets(ctx context.Context, projectID string) ([]Bucket, error)
	ClearAllBuckets(ctx context.Context) error

	SaveObject(ctx context.Context, bucket string, obj *ObjectRecord) error
	SaveObjects(ctx context.Context, bucket string, objects []*ObjectRecord) error
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
	prefixS3Cred      = []byte("auth/s3cred/")
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
	return s.db.Update(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		var toDelete [][]byte
		for it.Seek(prefixS3Cred); it.ValidForPrefix(prefixS3Cred); it.Next() {
			toDelete = append(toDelete, it.Item().KeyCopy(nil))
		}
		for _, k := range toDelete {
			if err := txn.Delete(k); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *BadgerStore) SaveS3Credential(ctx context.Context, cred *S3Credential) error {
	return s.set(ctx, append(prefixS3Cred, []byte(cred.Name)...), cred)
}

func (s *BadgerStore) GetS3Credential(ctx context.Context, name string) (*S3Credential, error) {
	data, err := s.get(ctx, append(prefixS3Cred, []byte(name)...))
	if err != nil {
		return nil, err
	}
	var cred S3Credential
	if err := json.Unmarshal(data, &cred); err != nil {
		return nil, err
	}
	return &cred, nil
}

func (s *BadgerStore) DeleteS3Credential(ctx context.Context, name string) error {
	err := s.del(ctx, append(prefixS3Cred, []byte(name)...))
	if err != nil && err != badger.ErrKeyNotFound {
		return err
	}
	return nil
}

func (s *BadgerStore) ListS3Credentials(ctx context.Context) ([]*S3Credential, error) {
	var out []*S3Credential
	err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Seek(prefixS3Cred); it.ValidForPrefix(prefixS3Cred); it.Next() {
			var cred S3Credential
			err := it.Item().Value(func(v []byte) error {
				return json.Unmarshal(v, &cred)
			})
			if err != nil {
				return err
			}
			out = append(out, &cred)
		}
		return nil
	})
	return out, err
}

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

func (s *BadgerStore) ClearAllBuckets(ctx context.Context) error {
	return s.db.Update(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		var toDelete [][]byte
		for it.Seek(prefixBuckets); it.ValidForPrefix(prefixBuckets); it.Next() {
			toDelete = append(toDelete, it.Item().KeyCopy(nil))
		}
		for _, k := range toDelete {
			if err := txn.Delete(k); err != nil {
				return err
			}
		}
		return nil
	})
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

func objectKey(bucket string, encodedKey string) []byte {
	return append(prefixObjects, []byte(bucket+"/"+encodedKey)...)
}

func (s *BadgerStore) SaveObject(ctx context.Context, bucket string, obj *ObjectRecord) error {
	key := objectKey(bucket, obj.ETag+"/"+obj.Key)
	return s.set(ctx, key, obj)
}

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

func (s *BadgerStore) DeleteObject(ctx context.Context, bucket string, encodedKey string) error {
	// Key format: objects/{bucket}/{etag}/{key}
	// Find the key by checking the portion after the second '/'
	prefix := append(prefixObjects, []byte(bucket+"/")...)
	keys, err := s.iterateKeys(ctx, prefix)
	if err != nil {
		return err
	}
	for _, k := range keys {
		suffix := k[len(prefix):] // "{etag}/{key}"
		slashIdx := strings.IndexByte(suffix, '/')
		if slashIdx < 0 {
			continue
		}
		objKey := suffix[slashIdx+1:]
		if objKey == encodedKey {
			return s.del(ctx, []byte(k))
		}
	}
	return nil
}

func (s *BadgerStore) ListObjectKeys(ctx context.Context, bucket string) ([]string, error) {
	prefix := append(prefixObjects, []byte(bucket+"/")...)
	return s.iterateKeys(ctx, prefix)
}

func (s *BadgerStore) GetObject(ctx context.Context, bucket string, encodedKey string) (*ObjectRecord, error) {
	// Key format: objects/{bucket}/{etag}/{key}
	// Iterate keys, find the one matching encodedKey, then fetch directly
	prefix := append(prefixObjects, []byte(bucket+"/")...)
	keys, err := s.iterateKeys(ctx, prefix)
	if err != nil {
		return nil, err
	}
	var targetKey string
	for _, k := range keys {
		suffix := k[len(prefix):]
		slashIdx := strings.IndexByte(suffix, '/')
		if slashIdx < 0 {
			continue
		}
		objKey := suffix[slashIdx+1:]
		if objKey == encodedKey {
			targetKey = k
			break
		}
	}
	if targetKey == "" {
		return nil, badger.ErrKeyNotFound
	}
	data, err := s.get(ctx, []byte(targetKey))
	if err != nil {
		return nil, err
	}
	var obj ObjectRecord
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, err
	}
	return &obj, nil
}

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

func (s *BadgerStore) AddScanToBucketIndex(ctx context.Context, bucket string, scanID string) error {
	key := append(prefixBucketIndex, []byte(bucket)...)
	return s.db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		var ids []string
		if err == nil {
			data, err := item.ValueCopy(nil)
			if err == nil {
				json.Unmarshal(data, &ids)
			}
		}
		for _, id := range ids {
			if id == scanID {
				return nil
			}
		}
		ids = append(ids, scanID)
		data, err := json.Marshal(ids)
		if err != nil {
			return err
		}
		return txn.Set(key, data)
	})
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
