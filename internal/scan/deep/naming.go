package deep

import (
	"context"
	"log"
	"regexp"

	"github.com/esignoretti/s3lytics/internal/s3"
	"github.com/esignoretti/s3lytics/internal/store"
)

func CheckNamingConvention(ctx context.Context, client s3.S3Client, bucket string, pattern string) (*store.DeepNaming, error) {
	result := &store.DeepNaming{
		Pattern:  pattern,
		Examples: []string{},
	}

	if pattern == "" {
		result.NonCompliantCount = 0
		return result, nil
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	var continuationToken *string

	for {
		page, err := client.ListObjectsPage(ctx, bucket, "", continuationToken)
		if err != nil {
			log.Printf("deep/naming: pagination error after %d objects, returning partial results: %v", result.NonCompliantCount, err)
			return result, nil
		}

		for _, obj := range page.Objects {
			if !re.MatchString(obj.Key) {
				result.NonCompliantCount++
				if len(result.Examples) < 10 {
					result.Examples = append(result.Examples, obj.Key)
				}
			}
		}

		if !page.IsTruncated {
			break
		}
		continuationToken = page.ContinuationToken
	}

	return result, nil
}
