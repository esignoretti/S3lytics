package s3

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type ObjectInfo struct {
	Key          string
	ETag         string
	Size         int64
	LastModified time.Time
	StorageClass string
}

type ListResult struct {
	Objects           []ObjectInfo
	ContinuationToken *string
	IsTruncated       bool
}

type BucketInfo struct {
	Name         string
	CreationDate time.Time
}

type S3Client interface {
	ListBuckets(ctx context.Context) ([]BucketInfo, error)
	ListObjectsPage(ctx context.Context, bucket, prefix string, continuationToken *string) (*ListResult, error)
	HeadObject(ctx context.Context, bucket, key string) (*ObjectInfo, error)
	ListMultipartUploads(ctx context.Context, bucket string) ([]types.MultipartUpload, error)
	GetBucketPolicy(ctx context.Context, bucket string) (string, error)
	GetBucketAcl(ctx context.Context, bucket string) ([]types.Grant, error)
	GetPublicAccessBlock(ctx context.Context, bucket string) (*types.PublicAccessBlockConfiguration, error)
	GetBucketEncryption(ctx context.Context, bucket string) (*types.ServerSideEncryptionConfiguration, error)
	ListObjectVersions(ctx context.Context, bucket string) ([]types.ObjectVersion, []types.DeleteMarkerEntry, error)
	GetObject(ctx context.Context, bucket, key string, rangeSpec *string) ([]byte, error)
	ListPrefixes(ctx context.Context, bucket, prefix string) ([]string, error)
}

type CubbitS3Client struct {
	client *s3.Client
}

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

	// Tune HTTP transport for high-concurrency S3 access
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			MaxConnsPerHost:     100,
			IdleConnTimeout:     90 * time.Second,
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2: true,
		},
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = true
		o.HTTPClient = httpClient
	})

	return &CubbitS3Client{client: client}, nil
}
