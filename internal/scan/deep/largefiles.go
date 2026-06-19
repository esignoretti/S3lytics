package deep

import (
	"context"
	"sort"

	"github.com/esignoretti/s3lytics/internal/s3"
	"github.com/esignoretti/s3lytics/internal/store"
)

func FindLargeFiles(ctx context.Context, client s3.S3Client, bucket string, thresholdBytes int64, maxResults int) ([]store.LargeFile, error) {
	var largeFiles []store.LargeFile
	var continuationToken *string

	for {
		result, err := client.ListObjectsPage(ctx, bucket, "", continuationToken)
		if err != nil {
			return nil, err
		}

		for _, obj := range result.Objects {
			if obj.Size >= thresholdBytes {
				largeFiles = append(largeFiles, store.LargeFile{
					Key:          obj.Key,
					Size:         obj.Size,
					LastModified: obj.LastModified,
				})
			}
		}

		if !result.IsTruncated {
			break
		}
		continuationToken = result.ContinuationToken
	}

	sort.Slice(largeFiles, func(i, j int) bool {
		return largeFiles[i].Size > largeFiles[j].Size
	})

	if maxResults > 0 && len(largeFiles) > maxResults {
		largeFiles = largeFiles[:maxResults]
	}

	return largeFiles, nil
}
