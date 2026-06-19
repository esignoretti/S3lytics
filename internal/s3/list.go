package s3

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
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

func (c *CubbitS3Client) ListObjectsPage(ctx context.Context, bucket, prefix string, continuationToken *string) (*ListResult, error) {
	input := &s3.ListObjectsV2Input{
		Bucket:  &bucket,
		Prefix:  &prefix,
		MaxKeys: aws.Int32(2000),
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
			Size:         *obj.Size,
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
		IsTruncated:       *resp.IsTruncated,
	}, nil
}

func (c *CubbitS3Client) ListPrefixes(ctx context.Context, bucket, prefix string) ([]string, error) {
	input := &s3.ListObjectsV2Input{
		Bucket:    &bucket,
		Prefix:    &prefix,
		Delimiter: aws.String("/"),
		MaxKeys:   aws.Int32(1000),
	}
	resp, err := c.client.ListObjectsV2(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("list prefixes: %w", err)
	}
	prefixes := make([]string, 0, len(resp.CommonPrefixes))
	for _, cp := range resp.CommonPrefixes {
		prefixes = append(prefixes, *cp.Prefix)
	}
	return prefixes, nil
}
