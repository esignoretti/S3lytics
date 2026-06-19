package deep

import (
	"context"
	"sort"

	"github.com/esignoretti/s3lytics/internal/s3"
	"github.com/esignoretti/s3lytics/internal/store"
)

func FindDuplicates(ctx context.Context, client s3.S3Client, bucket string) ([]store.DuplicateGroup, error) {
	etagGroups := make(map[string]*store.DuplicateGroup)
	var continuationToken *string

	for {
		result, err := client.ListObjectsPage(ctx, bucket, "", continuationToken)
		if err != nil {
			return nil, err
		}

		for _, obj := range result.Objects {
			if obj.ETag == "" {
				continue
			}
			if _, ok := etagGroups[obj.ETag]; !ok {
				etagGroups[obj.ETag] = &store.DuplicateGroup{
					ETag: obj.ETag,
					Keys: []string{},
				}
			}
			g := etagGroups[obj.ETag]
			g.Count++
			g.TotalSize += obj.Size
			g.Keys = append(g.Keys, obj.Key)
		}

		if !result.IsTruncated {
			break
		}
		continuationToken = result.ContinuationToken
	}

	var groups []store.DuplicateGroup
	for _, g := range etagGroups {
		if g.Count > 1 {
			groups = append(groups, *g)
		}
	}

	sort.Slice(groups, func(i, j int) bool {
		return groups[i].TotalSize > groups[j].TotalSize
	})

	return groups, nil
}
