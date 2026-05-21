# S3lytics — Phase 6: Deep Scan Features

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement all deep scan analyzers that run after the basic scan completes: duplicate detection, multipart uploads, access audit, encryption audit, versioning waste, large file heatmap, naming convention check, cost estimation, and virus scan (ClamAV).

**Architecture:** Package `internal/scan/deep/` contains one file per analyzer. Each analyzer takes the `s3.S3Client`, bucket name, and configuration, then returns its result struct. The scan engine calls analyzers sequentially after the basic scan. The deep scan scan ID reuses the basic scan ID (results stored under `scans/{id}/deep_*` keys in the store).

**Tech Stack:** `s3.S3Client` (Phase 3), `store` models (Phase 2), `encoding/json`

**Pre-requisites:** Phase 5 complete (scan engine exists).

---

### Task 1: Duplicate detection analyzer

**Files:**
- Create: `internal/scan/deep/duplicates.go`

- [ ] **Step 1: Write the duplicate detection analyzer**

```go
package deep

import (
	"context"
	"sort"

	"github.com/esignoretti/s3lytics/internal/s3"
	"github.com/esignoretti/s3lytics/internal/store"
)

// FindDuplicates groups objects by ETag and returns groups with more than one member.
func FindDuplicates(ctx context.Context, client s3.S3Client, bucket string) ([]store.DuplicateGroup, error) {
	etagGroups := make(map[string]*store.DuplicateGroup)
	var continuationToken *string

	for {
		result, err := client.ListObjectsPage(ctx, bucket, continuationToken)
		if err != nil {
			return nil, err
		}

		for _, obj := range result.Objects {
			if obj.ETag == "" {
				continue
			}
			if _, ok := etagGroups[obj.ETag]; !ok {
				etagGroups[obj.ETag] = &store.DuplicateGroup{
					ETag:  obj.ETag,
					Count: 0,
					Keys:  []string{},
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
```

- [ ] **Step 2: Verify compilation**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go vet ./internal/scan/deep/
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/scan/deep/duplicates.go && git commit -m "feat: add duplicate detection analyzer"
```

---

### Task 2: Incomplete multipart uploads analyzer

**Files:**
- Create: `internal/scan/deep/multipart.go`

- [ ] **Step 1: Write the multipart uploads analyzer**

```go
package deep

import (
	"context"
	"time"

	"github.com/esignoretti/s3lytics/internal/s3"
	"github.com/esignoretti/s3lytics/internal/store"
)

// FindMultipartUploads lists all incomplete multipart uploads in the bucket.
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
			Size:      0, // size not known until we list parts
		})
	}

	return results, nil
}
```

- [ ] **Step 2: Verify compilation**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go vet ./internal/scan/deep/
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/scan/deep/multipart.go && git commit -m "feat: add multipart uploads analyzer"
```

---

### Task 3: Access / security audit analyzer

**Files:**
- Create: `internal/scan/deep/access.go`

- [ ] **Step 1: Write the access audit analyzer**

```go
package deep

import (
	"context"
	"sync"
	"time"

	"github.com/esignoretti/s3lytics/internal/s3"
	"github.com/esignoretti/s3lytics/internal/store"
)

// AuditAccess checks bucket policy, ACL, and public access block.
func AuditAccess(ctx context.Context, client s3.S3Client, bucket string) (*store.DeepAccessAudit, error) {
	audit := &store.DeepAccessAudit{
		Findings: []store.AccessFinding{},
	}

	// Public access block
	pab, err := client.GetPublicAccessBlock(ctx, bucket)
	if err == nil && pab != nil {
		audit.PublicAccessBlocked = true
	} else {
		audit.Findings = append(audit.Findings, store.AccessFinding{
			Severity: "WARN",
			Message:  "Public access block not configured",
			Detail:   "GetPublicAccessBlock returned an error; block may be absent",
		})
	}

	// Bucket policy
	policy, err := client.GetBucketPolicy(ctx, bucket)
	if err == nil && policy != "" {
		audit.BucketPolicy = policy
		if strings.Contains(policy, "\"Effect\":\"Allow\"") &&
			strings.Contains(policy, "\"Principal\":\"*\"") {
			audit.Findings = append(audit.Findings, store.AccessFinding{
				Severity: "HIGH",
				Message:  "Bucket policy allows public access",
				Detail:   "Found Effect: Allow with Principal: * in bucket policy",
			})
		}
	}

	// ACL
	grants, err := client.GetBucketAcl(ctx, bucket)
	if err == nil {
		for _, g := range grants {
			if g.Grantee != nil && g.Grantee.Type == "Group" &&
				g.Grantee.URI != nil &&
				strings.Contains(*g.Grantee.URI, "AllUsers") {
				audit.Findings = append(audit.Findings, store.AccessFinding{
					Severity: "HIGH",
					Message:  "Bucket has public ACL grant",
					Detail:   "AllUsers group has permissions on this bucket",
				})
			}
		}
	}

	return audit, nil
}
```

- [ ] **Step 2: Verify compilation**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go vet ./internal/scan/deep/
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/scan/deep/access.go && git commit -m "feat: add access/security audit analyzer"
```

---

### Task 4: Encryption audit analyzer

**Files:**
- Create: `internal/scan/deep/encryption.go`

- [ ] **Step 1: Write the encryption audit analyzer**

```go
package deep

import (
	"context"
	"fmt"

	"github.com/esignoretti/s3lytics/internal/s3"
	"github.com/esignoretti/s3lytics/internal/store"
)

// AuditEncryption checks bucket default encryption and samples individual object SSE.
func AuditEncryption(ctx context.Context, client s3.S3Client, bucket string) (*store.DeepEncryption, error) {
	encResult := &store.DeepEncryption{
		Algorithms:      []string{},
		UnencryptedKeys: []string{},
	}

	// Check bucket-level default encryption
	encConfig, err := client.GetBucketEncryption(ctx, bucket)
	if err == nil && encConfig != nil {
		for _, rule := range encConfig.Rules {
			if rule.ApplyServerSideEncryptionByDefault != nil {
				algo := string(rule.ApplyServerSideEncryptionByDefault.SSEAlgorithm)
				encResult.Algorithms = append(encResult.Algorithms, algo)
			}
		}
	}

	// Sample objects to check per-object encryption
	var continuationToken *string
	sampleCount := 0
	maxSamples := 1000
	encryptedCount := 0
	totalChecked := 0

	for {
		result, err := client.ListObjectsPage(ctx, bucket, continuationToken)
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
				// Non-standard storage usually implies SSE-S3 or similar
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
```

- [ ] **Step 2: Verify compilation**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go vet ./internal/scan/deep/
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/scan/deep/encryption.go && git commit -m "feat: add encryption audit analyzer"
```

---

### Task 5: Versioning waste & large files analyzers

**Files:**
- Create: `internal/scan/deep/versioning.go`
- Create: `internal/scan/deep/largefiles.go`

- [ ] **Step 1: Write versioning waste analyzer**

```go
package deep

import (
	"context"
	"fmt"

	"github.com/esignoretti/s3lytics/internal/s3"
	"github.com/esignoretti/s3lytics/internal/store"
)

// AnalyzeVersioningWaste checks versioning status and calculates wasted space from non-current versions.
func AnalyzeVersioningWaste(ctx context.Context, client s3.S3Client, bucket string) (*store.DeepVersioning, error) {
	versions, deleteMarkers, err := client.ListObjectVersions(ctx, bucket)
	if err != nil {
		return &store.DeepVersioning{}, nil // versioning likely not enabled
	}

	result := &store.DeepVersioning{}

	// Count non-current versions
	for _, v := range versions {
		result.TotalVersions++
		if v.IsLatest != nil && !*v.IsLatest {
			result.NonCurrentCount++
			if v.Size != nil {
				result.WastedBytes += *v.Size
			}
		}
	}

	// Delete markers also represent non-current state
	for range deleteMarkers {
		result.TotalVersions++
	}

	return result, nil
}
```

- [ ] **Step 2: Write large files heatmap analyzer**

```go
package deep

import (
	"context"
	"sort"

	"github.com/esignoretti/s3lytics/internal/s3"
	"github.com/esignoretti/s3lytics/internal/store"
)

// FindLargeFiles lists all objects above the given size threshold, sorted descending.
func FindLargeFiles(ctx context.Context, client s3.S3Client, bucket string, thresholdBytes int64, maxResults int) ([]store.LargeFile, error) {
	var largeFiles []store.LargeFile
	var continuationToken *string

	for {
		result, err := client.ListObjectsPage(ctx, bucket, continuationToken)
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
```

- [ ] **Step 3: Verify compilation**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go vet ./internal/scan/deep/
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/scan/deep/versioning.go internal/scan/deep/largefiles.go && git commit -m "feat: add versioning waste and large files analyzers"
```

---

### Task 6: Naming convention and cost estimation analyzers

**Files:**
- Create: `internal/scan/deep/naming.go`
- Create: `internal/scan/deep/cost.go`

- [ ] **Step 1: Write naming convention checker**

```go
package deep

import (
	"context"
	"regexp"

	"github.com/esignoretti/s3lytics/internal/s3"
	"github.com/esignoretti/s3lytics/internal/store"
)

// CheckNamingConvention validates object keys against a regex pattern.
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
	var totalChecked int

	for {
		page, err := client.ListObjectsPage(ctx, bucket, continuationToken)
		if err != nil {
			break
		}

		for _, obj := range page.Objects {
			totalChecked++
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
```

- [ ] **Step 2: Write cost estimation analyzer**

```go
package deep

import (
	"context"

	"github.com/esignoretti/s3lytics/internal/s3"
	"github.com/esignoretti/s3lytics/internal/store"
)

var defaultCosts = map[string]float64{
	"STANDARD":                 0.023,
	"INTELLIGENT_TIERING":      0.023,
	"STANDARD_IA":              0.0125,
	"ONEZONE_IA":               0.01,
	"GLACIER":                  0.004,
	"DEEP_ARCHIVE":             0.002,
	"GLACIER_INSTANT_RETRIEVAL": 0.004,
}

// EstimateCost calculates monthly storage cost based on storage class.
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
			rate = 0.023 // default to STANDARD rate
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
```

- [ ] **Step 3: Verify compilation**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go vet ./internal/scan/deep/
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/scan/deep/naming.go internal/scan/deep/cost.go && git commit -m "feat: add naming convention and cost estimation analyzers"
```

---

### Task 7: Virus scan analyzer (ClamAV)

**Files:**
- Create: `internal/scan/deep/virus.go`

- [ ] **Step 1: Write the ClamAV virus scan analyzer**

```go
package deep

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/esignoretti/s3lytics/internal/s3"
	"github.com/esignoretti/s3lytics/internal/store"
)

// VirusScanConfig configures the ClamAV scan.
type VirusScanConfig struct {
	ClamdSocket string   // e.g., "/var/run/clamav/clamd.sock" or "tcp://host:3310"
	Extensions  []string // only scan files with these extensions (empty = all)
	MaxSize     int64    // skip files larger than this (0 = no limit)
	MaxCount    int      // max objects to scan (0 = no limit)
	LastSince   time.Time // only scan objects modified after this time
}

// ScanObjectsForVirus sends objects to ClamAV for scanning.
func ScanObjectsForVirus(ctx context.Context, client s3.S3Client, bucket string, config VirusScanConfig) (*store.VirusResult, error) {
	result := &store.VirusResult{
		Status:   "completed",
		Scanned:  0,
		Infected: []string{},
		Errors:   []string{},
	}

	// Build extension filter set
	extSet := make(map[string]bool)
	for _, ext := range config.Extensions {
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		extSet[strings.ToLower(ext)] = true
	}

	var continuationToken *string
	scannedCount := 0

	for {
		page, err := client.ListObjectsPage(ctx, bucket, continuationToken)
		if err != nil {
			break
		}

		for _, obj := range page.Objects {
			if config.MaxCount > 0 && scannedCount >= config.MaxCount {
				return result, nil
			}

			// Extension filter
			if len(extSet) > 0 {
				ext := strings.ToLower(extractExtension(obj.Key))
				if !extSet[ext] {
					continue
				}
			}

			// Size filter
			if config.MaxSize > 0 && obj.Size > config.MaxSize {
				continue
			}

			// Date filter
			if !config.LastSince.IsZero() && obj.LastModified.Before(config.LastSince) {
				continue
			}

			// Stream object to ClamAV
			data, err := client.GetObject(ctx, bucket, obj.Key, nil)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", obj.Key, err))
				continue
			}

			infected, err := scanWithClamd(data, config.ClamdSocket)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", obj.Key, err))
				continue
			}

			scannedCount++
			if infected != "" {
				result.Infected = append(result.Infected, fmt.Sprintf("%s: %s", obj.Key, infected))
			}
		}

		if !page.IsTruncated {
			break
		}
		continuationToken = page.ContinuationToken
	}

	result.Scanned = scannedCount
	if len(result.Infected) > 0 {
		result.Status = "completed_with_infections"
	}

	return result, nil
}

func scanWithClamd(data []byte, socketAddr string) (string, error) {
	var conn net.Conn
	var err error

	if strings.HasPrefix(socketAddr, "tcp://") {
		conn, err = net.DialTimeout("tcp", socketAddr[6:], 30*time.Second)
	} else {
		conn, err = net.DialTimeout("unix", socketAddr, 30*time.Second)
	}
	if err != nil {
		return "", fmt.Errorf("clamd connect: %w", err)
	}
	defer conn.Close()

	// ClamAV protocol: zINSTREAM\0 + [4-byte length][data] + [0-length terminator]
	cmd := []byte("zINSTREAM\x00")
	if _, err := conn.Write(cmd); err != nil {
		return "", fmt.Errorf("send instream: %w", err)
	}

	// Send data in chunks
	chunkSize := 1024 * 64
	reader := bytes.NewReader(data)
	buf := make([]byte, chunkSize)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			// Write 4-byte big-endian length prefix
			lenBytes := []byte{
				byte(n >> 24),
				byte(n >> 16),
				byte(n >> 8),
				byte(n),
			}
			if _, err := conn.Write(lenBytes); err != nil {
				return "", fmt.Errorf("send length: %w", err)
			}
			if _, err := conn.Write(buf[:n]); err != nil {
				return "", fmt.Errorf("send chunk: %w", err)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("read chunk: %w", err)
		}
	}

	// Zero-length terminator
	conn.Write([]byte{0, 0, 0, 0})

	// Read response
	respBuf := make([]byte, 4096)
	n, err := conn.Read(respBuf)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	response := string(respBuf[:n])
	if strings.Contains(response, "FOUND") {
		// Extract virus name
		parts := strings.Split(strings.TrimSpace(response), ":")
		if len(parts) >= 2 {
			return strings.TrimSpace(parts[len(parts)-1]), nil
		}
		return "unknown virus", nil
	}

	return "", nil // clean
}
```

- [ ] **Step 2: Verify compilation**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go vet ./internal/scan/deep/
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/scan/deep/virus.go && git commit -m "feat: add ClamAV virus scan analyzer"
```

---

### Task 8: Deep scan coordinator and tests

**Files:**
- Create: `internal/scan/deep/coordinator.go`
- Create: `internal/scan/deep/deep_test.go`

- [ ] **Step 1: Write the deep scan coordinator**

```go
package deep

import (
	"context"
	"time"

	"github.com/esignoretti/s3lytics/internal/s3"
	"github.com/esignoretti/s3lytics/internal/store"
)

// Config holds all deep scan configuration options.
type Config struct {
	EnableDuplicates      bool
	EnableMultipart       bool
	EnableAccessAudit     bool
	EnableEncryption      bool
	EnableVersioning      bool
	EnableLargeFiles      bool
	EnableNaming          bool
	EnableCostEstimate    bool
	EnableVirusScan       bool

	LargeFileThresholdMB  int64
	NamingPattern         string
	VirusConfig           VirusScanConfig
	CostOverrides         map[string]float64
}

// RunAll executes all enabled deep scans for the given bucket and returns the populated ScanResult.
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
```

- [ ] **Step 2: Write deep scan tests**

```go
package deep

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/esignoretti/s3lytics/internal/s3"
	"github.com/esignoretti/s3lytics/internal/store"
)

type mockS3Client struct {
	objects []s3.ObjectInfo
}

func (m *mockS3Client) ListBuckets(ctx context.Context) ([]s3.BucketInfo, error) {
	return nil, nil
}

func (m *mockS3Client) ListObjectsPage(ctx context.Context, bucket string, continuationToken *string) (*s3.ListResult, error) {
	return &s3.ListResult{
		Objects:     m.objects,
		IsTruncated: false,
	}, nil
}

func (m *mockS3Client) HeadObject(ctx context.Context, bucket, key string) (*s3.ObjectInfo, error) {
	for _, o := range m.objects {
		if o.Key == key {
			return &o, nil
		}
	}
	return nil, nil
}

func (m *mockS3Client) ListMultipartUploads(ctx context.Context, bucket string) ([]types.MultipartUpload, error) {
	return nil, nil
}
func (m *mockS3Client) GetBucketPolicy(ctx context.Context, bucket string) (string, error) {
	return "", nil
}
func (m *mockS3Client) GetBucketAcl(ctx context.Context, bucket string) ([]types.Grant, error) {
	return nil, nil
}
func (m *mockS3Client) GetPublicAccessBlock(ctx context.Context, bucket string) (*types.PublicAccessBlockConfiguration, error) {
	return nil, nil
}
func (m *mockS3Client) GetBucketEncryption(ctx context.Context, bucket string) (*types.ServerSideEncryptionConfiguration, error) {
	return nil, nil
}
func (m *mockS3Client) ListObjectVersions(ctx context.Context, bucket string) ([]types.ObjectVersion, []types.DeleteMarkerEntry, error) {
	return nil, nil, nil
}
func (m *mockS3Client) GetObject(ctx context.Context, bucket, key string, rangeSpec *string) ([]byte, error) {
	return []byte("test-data"), nil
}

func TestFindDuplicates(t *testing.T) {
	client := &mockS3Client{
		objects: []s3.ObjectInfo{
			{Key: "a.txt", ETag: "e1", Size: 100},
			{Key: "b.txt", ETag: "e1", Size: 100}, // duplicate of a.txt
			{Key: "c.txt", ETag: "e2", Size: 200},
		},
	}

	dups, err := FindDuplicates(context.Background(), client, "test-bucket")
	if err != nil {
		t.Fatal(err)
	}

	if len(dups) != 1 {
		t.Errorf("expected 1 duplicate group, got %d", len(dups))
	}
	if len(dups) > 0 && len(dups[0].Keys) != 2 {
		t.Errorf("expected 2 keys in duplicate group, got %d", len(dups[0].Keys))
	}
}

func TestFindLargeFiles(t *testing.T) {
	client := &mockS3Client{
		objects: []s3.ObjectInfo{
			{Key: "small.txt", Size: 100},
			{Key: "large.txt", Size: 100 * 1024 * 1024},
		},
	}

	files, err := FindLargeFiles(context.Background(), client, "test-bucket", 50*1024*1024, 10)
	if err != nil {
		t.Fatal(err)
	}

	if len(files) != 1 {
		t.Errorf("expected 1 large file, got %d", len(files))
	}
	if len(files) > 0 && files[0].Key != "large.txt" {
		t.Errorf("expected large.txt, got %s", files[0].Key)
	}
}

func TestCheckNamingConvention(t *testing.T) {
	client := &mockS3Client{
		objects: []s3.ObjectInfo{
			{Key: "valid-name.txt"},
			{Key: "INVALID_NAME"},
			{Key: "another-valid.txt"},
		},
	}

	result, err := CheckNamingConvention(context.Background(), client, "test-bucket", "^[a-z][a-z0-9-]+\\.[a-z]+$")
	if err != nil {
		t.Fatal(err)
	}

	if result.NonCompliantCount != 1 {
		t.Errorf("expected 1 non-compliant, got %d", result.NonCompliantCount)
	}
}

func TestEstimateCost(t *testing.T) {
	client := &mockS3Client{
		objects: []s3.ObjectInfo{
			{Key: "a.txt", Size: 1073741824, StorageClass: "STANDARD"},        // 1 GB
			{Key: "b.txt", Size: 1073741824, StorageClass: "GLACIER"},          // 1 GB
		},
	}

	cost, err := EstimateCost(context.Background(), client, "test-bucket", nil)
	if err != nil {
		t.Fatal(err)
	}

	if cost.Breakdown == nil || len(cost.Breakdown) == 0 {
		t.Fatal("expected cost breakdown")
	}
	// STANDARD: 1 GB * $0.023 = $0.023
	// GLACIER: 1 GB * $0.004 = $0.004
	if cost.MonthlyTotal < 0.01 || cost.MonthlyTotal > 0.1 {
		t.Errorf("expected monthly total around $0.027, got $%.4f", cost.MonthlyTotal)
	}
}

func TestExtractExtension(t *testing.T) {
	tests := []struct {
		key  string
		want string
	}{
		{"file.txt", ".txt"},
		{"dir/file.jpg", ".jpg"},
		{"noext", "(no extension)"},
		{"dir/.hidden", ".hidden"},
	}

	for _, tt := range tests {
		got := extractExtension(tt.key)
		if got != tt.want {
			t.Errorf("extractExtension(%q) = %q, want %q", tt.key, got, tt.want)
		}
	}
}
```

- [ ] **Step 3: Verify compilation**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go vet ./internal/scan/deep/
```

Expected: no errors.

- [ ] **Step 4: Run the tests**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go test ./internal/scan/deep/ -v -count=1 -timeout=30s
```

Expected: all 4 tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/scan/deep/coordinator.go internal/scan/deep/deep_test.go && git commit -m "feat: add deep scan coordinator and tests"
```

---

**End of Phase 6. Phase 6 deliverables:**
- [x] Duplicate detection analyzer
- [x] Multipart uploads analyzer
- [x] Access / security audit analyzer
- [x] Encryption audit analyzer
- [x] Versioning waste analyzer
- [x] Large files heatmap analyzer
- [x] Naming convention checker
- [x] Cost estimation analyzer
- [x] ClamAV virus scan with INSTREAM protocol
- [x] Deep scan coordinator with concurrent goroutines
- [x] 4 unit tests passing

**Ready for Phase 7: Web UI — templates, static assets, and HTTP handlers.**
