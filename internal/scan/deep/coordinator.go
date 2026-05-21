package deep

import (
	"context"
	"sync"

	"github.com/esignoretti/s3lytics/internal/s3"
	"github.com/esignoretti/s3lytics/internal/store"
)

type Config struct {
	EnableDuplicates   bool
	EnableMultipart    bool
	EnableAccessAudit  bool
	EnableEncryption   bool
	EnableVersioning   bool
	EnableLargeFiles   bool
	EnableNaming       bool
	EnableCostEstimate bool
	EnableVirusScan    bool

	LargeFileThresholdMB int64
	NamingPattern        string
	VirusConfig          VirusScanConfig
	CostOverrides        map[string]float64
}

func RunAll(ctx context.Context, client s3.S3Client, bucket string, cfg Config) *store.ScanResult {
	result := &store.ScanResult{}

	var wg sync.WaitGroup
	var mu sync.Mutex

	if cfg.EnableDuplicates {
		wg.Add(1)
		go func() {
			defer wg.Done()
			dups, err := FindDuplicates(ctx, client, bucket)
			if err == nil {
				mu.Lock()
				result.Duplicates = dups
				mu.Unlock()
			}
		}()
	}

	if cfg.EnableMultipart {
		wg.Add(1)
		go func() {
			defer wg.Done()
			uploads, err := FindMultipartUploads(ctx, client, bucket)
			if err == nil {
				mu.Lock()
				result.Multiparts = uploads
				mu.Unlock()
			}
		}()
	}

	if cfg.EnableAccessAudit {
		wg.Add(1)
		go func() {
			defer wg.Done()
			audit, err := AuditAccess(ctx, client, bucket)
			if err == nil {
				mu.Lock()
				result.AccessAudit = audit
				mu.Unlock()
			}
		}()
	}

	if cfg.EnableEncryption {
		wg.Add(1)
		go func() {
			defer wg.Done()
			enc, err := AuditEncryption(ctx, client, bucket)
			if err == nil {
				mu.Lock()
				result.Encryption = enc
				mu.Unlock()
			}
		}()
	}

	if cfg.EnableVersioning {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ver, err := AnalyzeVersioningWaste(ctx, client, bucket)
			if err == nil {
				mu.Lock()
				result.Versioning = ver
				mu.Unlock()
			}
		}()
	}

	if cfg.EnableLargeFiles {
		wg.Add(1)
		go func() {
			defer wg.Done()
			threshold := cfg.LargeFileThresholdMB * 1024 * 1024
			files, err := FindLargeFiles(ctx, client, bucket, threshold, 100)
			if err == nil {
				mu.Lock()
				result.LargeFiles = files
				mu.Unlock()
			}
		}()
	}

	if cfg.EnableNaming {
		wg.Add(1)
		go func() {
			defer wg.Done()
			naming, err := CheckNamingConvention(ctx, client, bucket, cfg.NamingPattern)
			if err == nil {
				mu.Lock()
				result.Naming = naming
				mu.Unlock()
			}
		}()
	}

	if cfg.EnableCostEstimate {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cost, err := EstimateCost(ctx, client, bucket, cfg.CostOverrides)
			if err == nil {
				mu.Lock()
				result.CostEstimate = cost
				mu.Unlock()
			}
		}()
	}

	if cfg.EnableVirusScan {
		wg.Add(1)
		go func() {
			defer wg.Done()
			virus, err := ScanObjectsForVirus(ctx, client, bucket, cfg.VirusConfig)
			if err == nil {
				mu.Lock()
				result.Virus = virus
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	return result
}
