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
		Size:         *resp.ContentLength,
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
