package deep

import (
	"context"
	"time"

	"github.com/esignoretti/s3lytics/internal/s3"
	"github.com/esignoretti/s3lytics/internal/store"
)

func FindMultipartUploads(ctx context.Context, client s3.S3Client, bucket string) ([]store.MultipartUpload, error) {
	uploads, err := client.ListMultipartUploads(ctx, bucket)
	if err != nil {
		return nil, err
	}

	results := make([]store.MultipartUpload, 0, len(uploads))
	for _, u := range uploads {
		key := ""
		if u.Key != nil {
			key = *u.Key
		}
		uploadID := ""
		if u.UploadId != nil {
			uploadID = *u.UploadId
		}
		initiated := time.Time{}
		if u.Initiated != nil {
			initiated = *u.Initiated
		}

		results = append(results, store.MultipartUpload{
			UploadID:  uploadID,
			Key:       key,
			Initiated: initiated,
		})
	}

	return results, nil
}
