# S3lytics — Phase 3: S3 Client Abstraction & Operations

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the S3 client abstraction that wraps `aws-sdk-go-v2` with a custom endpoint resolver for Cubbit, supporting all S3 operations needed by the scan engine.

**Architecture:** Package `internal/s3/` provides an `S3Client` interface and a `CubbitS3Client` implementation. The client uses `aws-sdk-go-v2` with `s3.NewFromConfig` and a custom endpoint resolver. Pagination uses `ListObjectsV2` continuation tokens.

**Tech Stack:** `github.com/aws/aws-sdk-go-v2`, `github.com/aws/aws-sdk-go-v2/config`, `github.com/aws/aws-sdk-go-v2/service/s3`

**Pre-requisites:** Phase 2 complete (models exist in `internal/store/`).

---

### Task 1: Add AWS SDK dependencies

- [ ] **Step 1: Add aws-sdk-go-v2 modules**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && \
go get github.com/aws/aws-sdk-go-v2 && \
go get github.com/aws/aws-sdk-go-v2/config && \
go get github.com/aws/aws-sdk-go-v2/service/s3 && \
go get github.com/aws/aws-sdk-go-v2/credentials
```

Expected: dependencies added to `go.mod` and `go.sum`.

- [ ] **Step 2: Commit**

```bash
git add go.mod go.sum && git commit -m "chore: add aws-sdk-go-v2 dependencies"
```

---

### Task 2: S3Client interface

**Files:**
- Create: `internal/s3/client.go`

- [ ] **Step 1: Write the interface and CubbitS3Client**

```go
package s3

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// ObjectInfo is a simplified representation of an S3 object.
type ObjectInfo struct {
	Key          string
	ETag         string
	Size         int64
	LastModified time.Time
	StorageClass string
}

// ListResult holds one page of listing results.
type ListResult struct {
	Objects           []ObjectInfo
	ContinuationToken *string
	IsTruncated       bool
}

// BucketInfo holds bucket-level metadata.
type BucketInfo struct {
	Name         string
	CreationDate time.Time
}

// S3Client defines the S3 operations needed by the scan engine.
type S3Client interface {
	ListBuckets(ctx context.Context) ([]BucketInfo, error)
	ListObjectsPage(ctx context.Context, bucket string, continuationToken *string) (*ListResult, error)
	HeadObject(ctx context.Context, bucket, key string) (*ObjectInfo, error)
	ListMultipartUploads(ctx context.Context, bucket string) ([]types.MultipartUpload, error)
	GetBucketPolicy(ctx context.Context, bucket string) (string, error)
	GetBucketAcl(ctx context.Context, bucket string) ([]types.Grant, error)
	GetPublicAccessBlock(ctx context.Context, bucket string) (*types.PublicAccessBlockConfiguration, error)
	GetBucketEncryption(ctx context.Context, bucket string) (*types.ServerSideEncryptionConfiguration, error)
	ListObjectVersions(ctx context.Context, bucket string) ([]types.ObjectVersion, []types.DeleteMarkerEntry, error)
	GetObject(ctx context.Context, bucket, key string, rangeSpec *string) ([]byte, error)
}

// CubbitS3Client implements S3Client using aws-sdk-go-v2.
type CubbitS3Client struct {
	client *s3.Client
}

// NewCubbitS3Client creates a new client with the given endpoint, region, access key, and secret key.
func NewCubbitS3Client(endpoint, region, accessKey, secretKey string) (*CubbitS3Client, error) {
	customResolver := aws.EndpointResolverWithOptionsFunc(
		func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{
				URL:               endpoint,
				HostnameImmutable: true,
				SigningRegion:     region,
			}, nil
		},
	)

	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(region),
		config.WithEndpointResolverWithOptions(customResolver),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	)
	if err != nil {
		return nil, err
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = true
	})

	return &CubbitS3Client{client: client}, nil
}
```

- [ ] **Step 2: Verify compilation**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go vet ./internal/s3/
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/s3/client.go && git commit -m "feat: add S3Client interface and CubbitS3Client constructor"
```

---

### Task 3: List operations (buckets and objects with pagination)

**Files:**
- Create: `internal/s3/list.go`

- [ ] **Step 1: Write ListBuckets and ListObjectsPage**

```go
package s3

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func (c *CubbitS3Client) ListBuckets(ctx context.Context) ([]BucketInfo, error) {
	resp, err := c.client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return nil, fmt.Errorf("list buckets: %w", err)
	}

	buckets := make([]BucketInfo, 0, len(resp.Buckets))
	for _, b := range resp.Buckets {
		buckets = append(buckets, BucketInfo{
			Name:         *b.Name,
			CreationDate: *b.CreationDate,
		})
	}
	return buckets, nil
}

func (c *CubbitS3Client) ListObjectsPage(ctx context.Context, bucket string, continuationToken *string) (*ListResult, error) {
	input := &s3.ListObjectsV2Input{
		Bucket: &bucket,
	}
	if continuationToken != nil {
		input.ContinuationToken = continuationToken
	}

	resp, err := c.client.ListObjectsV2(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("list objects v2: %w", err)
	}

	objects := make([]ObjectInfo, 0, len(resp.Contents))
	for _, obj := range resp.Contents {
		info := ObjectInfo{
			Key:          *obj.Key,
			Size:         obj.Size,
			LastModified: *obj.LastModified,
		}
		if obj.ETag != nil {
			info.ETag = *obj.ETag
		}
		if obj.StorageClass != "" {
			info.StorageClass = string(obj.StorageClass)
		}
		objects = append(objects, info)
	}

	return &ListResult{
		Objects:           objects,
		ContinuationToken: resp.NextContinuationToken,
		IsTruncated:       resp.IsTruncated,
	}, nil
}
```

- [ ] **Step 2: Verify compilation**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go vet ./internal/s3/
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/s3/list.go && git commit -m "feat: add ListBuckets and ListObjectsPage with pagination"
```

---

### Task 4: Object metadata and data retrieval

**Files:**
- Create: `internal/s3/objects.go`

- [ ] **Step 1: Write HeadObject and GetObject**

```go
package s3

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func (c *CubbitS3Client) HeadObject(ctx context.Context, bucket, key string) (*ObjectInfo, error) {
	resp, err := c.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: &bucket,
		Key:    &key,
	})
	if err != nil {
		return nil, fmt.Errorf("head object: %w", err)
	}

	info := &ObjectInfo{
		Key:          key,
		Size:         resp.ContentLength,
		LastModified: *resp.LastModified,
	}
	if resp.ETag != nil {
		info.ETag = *resp.ETag
	}
	if resp.StorageClass != "" {
		info.StorageClass = string(resp.StorageClass)
	}
	return info, nil
}

func (c *CubbitS3Client) GetObject(ctx context.Context, bucket, key string, rangeSpec *string) ([]byte, error) {
	input := &s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &key,
	}
	if rangeSpec != nil {
		input.Range = rangeSpec
	}

	resp, err := c.client.GetObject(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("get object: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read object body: %w", err)
	}
	return data, nil
}
```

- [ ] **Step 2: Verify compilation**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go vet ./internal/s3/
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/s3/objects.go && git commit -m "feat: add HeadObject and GetObject"
```

---

### Task 5: Deep scan S3 operations (multipart, policy, ACL, encryption, versioning)

**Files:**
- Create: `internal/s3/deep.go`

- [ ] **Step 1: Write deep scan S3 operations**

```go
package s3

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func (c *CubbitS3Client) ListMultipartUploads(ctx context.Context, bucket string) ([]types.MultipartUpload, error) {
	var allUploads []types.MultipartUpload
	var keyMarker *string
	var uploadIDMarker *string

	for {
		resp, err := c.client.ListMultipartUploads(ctx, &s3.ListMultipartUploadsInput{
			Bucket:         &bucket,
			KeyMarker:      keyMarker,
			UploadIdMarker: uploadIDMarker,
		})
		if err != nil {
			return nil, fmt.Errorf("list multipart uploads: %w", err)
		}
		allUploads = append(allUploads, resp.Uploads...)
		if !resp.IsTruncated {
			break
		}
		keyMarker = resp.NextKeyMarker
		uploadIDMarker = resp.NextUploadIdMarker
	}
	return allUploads, nil
}

func (c *CubbitS3Client) GetBucketPolicy(ctx context.Context, bucket string) (string, error) {
	resp, err := c.client.GetBucketPolicy(ctx, &s3.GetBucketPolicyInput{
		Bucket: &bucket,
	})
	if err != nil {
		return "", fmt.Errorf("get bucket policy: %w", err)
	}
	return *resp.Policy, nil
}

func (c *CubbitS3Client) GetBucketAcl(ctx context.Context, bucket string) ([]types.Grant, error) {
	resp, err := c.client.GetBucketAcl(ctx, &s3.GetBucketAclInput{
		Bucket: &bucket,
	})
	if err != nil {
		return nil, fmt.Errorf("get bucket acl: %w", err)
	}
	return resp.Grants, nil
}

func (c *CubbitS3Client) GetPublicAccessBlock(ctx context.Context, bucket string) (*types.PublicAccessBlockConfiguration, error) {
	resp, err := c.client.GetPublicAccessBlock(ctx, &s3.GetPublicAccessBlockInput{
		Bucket: &bucket,
	})
	if err != nil {
		return nil, fmt.Errorf("get public access block: %w", err)
	}
	return resp.PublicAccessBlockConfiguration, nil
}

func (c *CubbitS3Client) GetBucketEncryption(ctx context.Context, bucket string) (*types.ServerSideEncryptionConfiguration, error) {
	resp, err := c.client.GetBucketEncryption(ctx, &s3.GetBucketEncryptionInput{
		Bucket: &bucket,
	})
	if err != nil {
		return nil, fmt.Errorf("get bucket encryption: %w", err)
	}
	return resp.ServerSideEncryptionConfiguration, nil
}

func (c *CubbitS3Client) ListObjectVersions(ctx context.Context, bucket string) ([]types.ObjectVersion, []types.DeleteMarkerEntry, error) {
	var allVersions []types.ObjectVersion
	var allMarkers []types.DeleteMarkerEntry
	var keyMarker *string
	var versionIDMarker *string

	for {
		resp, err := c.client.ListObjectVersions(ctx, &s3.ListObjectVersionsInput{
			Bucket:          &bucket,
			KeyMarker:       keyMarker,
			VersionIdMarker: versionIDMarker,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("list object versions: %w", err)
		}
		allVersions = append(allVersions, resp.Versions...)
		allMarkers = append(allMarkers, resp.DeleteMarkers...)
		if !resp.IsTruncated {
			break
		}
		keyMarker = resp.NextKeyMarker
		versionIDMarker = resp.NextVersionIdMarker
	}
	return allVersions, allMarkers, nil
}
```

- [ ] **Step 2: Verify compilation**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go vet ./internal/s3/
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/s3/deep.go && git commit -m "feat: add deep scan S3 operations (multipart, policy, ACL, encryption, versioning)"
```

---

**End of Phase 3. Phase 3 deliverables:**
- [x] `S3Client` interface with all required operations
- [x] `CubbitS3Client` implementation with custom endpoint resolver
- [x] List buckets and paginated list objects
- [x] HeadObject and GetObject
- [x] Deep scan operations: multipart uploads, bucket policy, ACL, public access block, encryption, versioning
- [x] AWS SDK dependencies added

**Ready for Phase 4: Auth service (Cubbit IAM reimplementation).**
