package deep

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/esignoretti/s3lytics/internal/s3"
)

type mockS3Client struct {
	objects []s3.ObjectInfo
}

func (m *mockS3Client) ListBuckets(ctx context.Context) ([]s3.BucketInfo, error) {
	return nil, nil
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
	return []byte("test-data"), nil
}

func TestFindDuplicates(t *testing.T) {
	client := &mockS3Client{
		objects: []s3.ObjectInfo{
			{Key: "a.txt", ETag: "e1", Size: 100},
			{Key: "b.txt", ETag: "e1", Size: 100},
			{Key: "c.txt", ETag: "e2", Size: 200},
		},
	}

	dups, err := FindDuplicates(context.Background(), client, "test-bucket")
	if err != nil {
		t.Fatal(err)
	}

	if len(dups) != 1 {
		t.Errorf("expected 1 duplicate group, got %d", len(dups))
	}
	if len(dups) > 0 && len(dups[0].Keys) != 2 {
		t.Errorf("expected 2 keys in duplicate group, got %d", len(dups[0].Keys))
	}
}

func TestFindLargeFiles(t *testing.T) {
	client := &mockS3Client{
		objects: []s3.ObjectInfo{
			{Key: "small.txt", Size: 100},
			{Key: "large.txt", Size: 100 * 1024 * 1024},
		},
	}

	files, err := FindLargeFiles(context.Background(), client, "test-bucket", 50*1024*1024, 10)
	if err != nil {
		t.Fatal(err)
	}

	if len(files) != 1 {
		t.Errorf("expected 1 large file, got %d", len(files))
	}
	if len(files) > 0 && files[0].Key != "large.txt" {
		t.Errorf("expected large.txt, got %s", files[0].Key)
	}
}

func TestCheckNamingConvention(t *testing.T) {
	client := &mockS3Client{
		objects: []s3.ObjectInfo{
			{Key: "valid-name.txt"},
			{Key: "INVALID_NAME"},
			{Key: "another-valid.txt"},
		},
	}

	result, err := CheckNamingConvention(context.Background(), client, "test-bucket", "^[a-z][a-z0-9-]+\\.[a-z]+$")
	if err != nil {
		t.Fatal(err)
	}

	if result.NonCompliantCount != 1 {
		t.Errorf("expected 1 non-compliant, got %d", result.NonCompliantCount)
	}
}

func TestEstimateCost(t *testing.T) {
	client := &mockS3Client{
		objects: []s3.ObjectInfo{
			{Key: "a.txt", Size: 1073741824, StorageClass: "STANDARD"},
			{Key: "b.txt", Size: 1073741824, StorageClass: "GLACIER"},
		},
	}

	cost, err := EstimateCost(context.Background(), client, "test-bucket", nil)
	if err != nil {
		t.Fatal(err)
	}

	if cost.Breakdown == nil || len(cost.Breakdown) == 0 {
		t.Fatal("expected cost breakdown")
	}
	if cost.MonthlyTotal < 0.01 || cost.MonthlyTotal > 0.1 {
		t.Errorf("expected monthly total around $0.027, got $%.4f", cost.MonthlyTotal)
	}
}

func TestExtractExtension(t *testing.T) {
	tests := []struct {
		key  string
		want string
	}{
		{"file.txt", ".txt"},
		{"dir/file.jpg", ".jpg"},
		{"noext", "(no extension)"},
		{"dir/.hidden", ".hidden"},
	}

	for _, tt := range tests {
		got := extractExtension(tt.key)
		if got != tt.want {
			t.Errorf("extractExtension(%q) = %q, want %q", tt.key, got, tt.want)
		}
	}
}
