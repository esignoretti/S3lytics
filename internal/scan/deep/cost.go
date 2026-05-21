package deep

import (
	"context"

	"github.com/esignoretti/s3lytics/internal/s3"
	"github.com/esignoretti/s3lytics/internal/store"
)

var defaultCosts = map[string]float64{
	"STANDARD":                  0.023,
	"INTELLIGENT_TIERING":       0.023,
	"STANDARD_IA":               0.0125,
	"ONEZONE_IA":                0.01,
	"GLACIER":                   0.004,
	"DEEP_ARCHIVE":              0.002,
	"GLACIER_INSTANT_RETRIEVAL": 0.004,
}

func EstimateCost(ctx context.Context, client s3.S3Client, bucket string, costOverrides map[string]float64) (*store.DeepCostEstimate, error) {
	costs := make(map[string]float64)
	for k, v := range defaultCosts {
		costs[k] = v
	}
	for k, v := range costOverrides {
		costs[k] = v
	}

	classTotals := make(map[string]int64)
	var continuationToken *string

	for {
		result, err := client.ListObjectsPage(ctx, bucket, continuationToken)
		if err != nil {
			return nil, err
		}

		for _, obj := range result.Objects {
			sc := obj.StorageClass
			if sc == "" {
				sc = "STANDARD"
			}
			classTotals[sc] += obj.Size
		}

		if !result.IsTruncated {
			break
		}
		continuationToken = result.ContinuationToken
	}

	estimate := &store.DeepCostEstimate{
		Breakdown: []store.CostBreakdown{},
	}

	var total float64
	for class, bytes := range classTotals {
		gb := float64(bytes) / (1024 * 1024 * 1024)
		rate, ok := costs[class]
		if !ok {
			rate = 0.023
		}
		monthly := gb * rate
		total += monthly
		estimate.Breakdown = append(estimate.Breakdown, store.CostBreakdown{
			Class:       class,
			MonthlyCost: monthly,
		})
	}

	estimate.MonthlyTotal = total
	return estimate, nil
}
