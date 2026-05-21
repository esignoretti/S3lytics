package deep

import (
	"context"

	"github.com/esignoretti/s3lytics/internal/s3"
	"github.com/esignoretti/s3lytics/internal/store"
)

func AnalyzeVersioningWaste(ctx context.Context, client s3.S3Client, bucket string) (*store.DeepVersioning, error) {
	versions, deleteMarkers, err := client.ListObjectVersions(ctx, bucket)
	if err != nil {
		return &store.DeepVersioning{}, nil
	}

	result := &store.DeepVersioning{}

	for _, v := range versions {
		result.TotalVersions++
		if v.IsLatest != nil && !*v.IsLatest {
			result.NonCurrentCount++
			if v.Size != nil {
				result.WastedBytes += *v.Size
			}
		}
	}

	for range deleteMarkers {
		result.TotalVersions++
	}

	return result, nil
}
