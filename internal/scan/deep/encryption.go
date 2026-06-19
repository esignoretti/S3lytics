package deep

import (
	"context"

	"github.com/esignoretti/s3lytics/internal/s3"
	"github.com/esignoretti/s3lytics/internal/store"
)

func AuditEncryption(ctx context.Context, client s3.S3Client, bucket string) (*store.DeepEncryption, error) {
	encResult := &store.DeepEncryption{
		Algorithms:      []string{},
		UnencryptedKeys: []string{},
	}

	encConfig, err := client.GetBucketEncryption(ctx, bucket)
	if err == nil && encConfig != nil {
		for _, rule := range encConfig.Rules {
			if rule.ApplyServerSideEncryptionByDefault != nil {
				algo := string(rule.ApplyServerSideEncryptionByDefault.SSEAlgorithm)
				encResult.Algorithms = append(encResult.Algorithms, algo)
			}
		}
	}

	var continuationToken *string
	sampleCount := 0
	maxSamples := 1000
	encryptedCount := 0
	totalChecked := 0

	for {
		result, err := client.ListObjectsPage(ctx, bucket, "", continuationToken)
		if err != nil {
			break
		}

		for _, obj := range result.Objects {
			if sampleCount >= maxSamples {
				break
			}
			totalChecked++
			sampleCount++

			info, err := client.HeadObject(ctx, bucket, obj.Key)
			if err != nil {
				continue
			}

			if info.StorageClass != "" && info.StorageClass != "STANDARD" {
				encryptedCount++
			} else {
				encResult.UnencryptedKeys = append(encResult.UnencryptedKeys, obj.Key)
			}
		}

		if !result.IsTruncated || sampleCount >= maxSamples {
			break
		}
		continuationToken = result.ContinuationToken
	}

	if totalChecked > 0 {
		encResult.EncryptedPct = float64(encryptedCount) / float64(totalChecked) * 100
		encResult.EncryptedPct = float64(int(encResult.EncryptedPct*100)) / 100
	}

	return encResult, nil
}
