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
		if !*resp.IsTruncated {
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
		if !*resp.IsTruncated {
			break
		}
		keyMarker = resp.NextKeyMarker
		versionIDMarker = resp.NextVersionIdMarker
	}
	return allVersions, allMarkers, nil
}
