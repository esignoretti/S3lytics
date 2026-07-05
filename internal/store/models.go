package store

import "time"

// --- Auth ---

type SessionData struct {
	JWT            string    `json:"jwt"`
	RefreshToken   string    `json:"refresh_token"`
	CoordinatorURL string    `json:"coordinator_url,omitempty"`
	ExpiresAt      time.Time `json:"expires_at"`
}

type AccountData struct {
	EndpointGateway string `json:"endpoint_gateway"`
	Email           string `json:"email"`
	UserID          string `json:"user_id"`
}

// S3Credential persists a Cubbit-issued S3 key pair. The secret is only
// returned by the keyvault API at creation time, so we must cache it.
type S3Credential struct {
	Name      string `json:"name"`
	ApiKey    string `json:"api_key"`
	SecretKey string `json:"secret_key"`
	UserID    string `json:"user_id"`
	ProjectID string `json:"project_id"`
}

// --- Projects & Buckets ---

type Project struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Bucket struct {
	Name         string `json:"name"`
	CreationDate string `json:"creation_date,omitempty"`
}

// --- Objects ---

type ObjectRecord struct {
	Key          string    `json:"key"`
	ETag         string    `json:"etag"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"last_modified"`
	StorageClass string    `json:"storage_class"`
	ScanID       string    `json:"scan_id"`
}

// --- Scans ---

type ScanRecord struct {
	ID        string    `json:"id"`
	Bucket    string    `json:"bucket"`
	Project   string    `json:"project"`
	Timestamp time.Time `json:"timestamp"`
	Duration  string    `json:"duration"`
	Status    string    `json:"status"`
	ScanType  string    `json:"scan_type"`
}

type ScanSummary struct {
	TotalObjects int64   `json:"total_objects"`
	TotalSize    int64   `json:"total_size"`
	AvgSize      float64 `json:"avg_size"`
	MedianSize   int64   `json:"median_size"`
	MaxSize      int64   `json:"max_size"`
	EmptyObjects int64   `json:"empty_objects"`
}

type TypeBreakdown struct {
	Ext       string  `json:"ext"`
	Count     int64   `json:"count"`
	TotalSize int64   `json:"total_size"`
	Pct       float64 `json:"pct"`
}

type AgeBucket struct {
	Label string `json:"label"`
	Count int64  `json:"count"`
	Size  int64  `json:"size"`
}

type StorageBreakdown struct {
	Class string `json:"class"`
	Count int64  `json:"count"`
	Size  int64  `json:"size"`
}

type PrefixBreakdown struct {
	Prefix string `json:"prefix"`
	Count  int64  `json:"count"`
	Size   int64  `json:"size"`
}

type DeltaReport struct {
	New       int64 `json:"new"`
	Modified  int64 `json:"modified"`
	Deleted   int64 `json:"deleted"`
	Unchanged int64 `json:"unchanged"`
}

// --- Deep Scan Results ---

type DuplicateGroup struct {
	ETag      string   `json:"etag"`
	Count     int64    `json:"count"`
	TotalSize int64    `json:"total_size"`
	Keys      []string `json:"keys"`
}

type MultipartUpload struct {
	UploadID  string    `json:"upload_id"`
	Key       string    `json:"key"`
	Initiated time.Time `json:"initiated"`
	Size      int64     `json:"size"`
}

type AccessFinding struct {
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Detail   string `json:"detail"`
}

type DeepAccessAudit struct {
	PublicAccessBlocked bool            `json:"public_access_blocked"`
	BucketPolicy        string          `json:"bucket_policy"`
	Findings            []AccessFinding `json:"findings"`
}

type DeepEncryption struct {
	EncryptedPct    float64  `json:"encrypted_pct"`
	Algorithms      []string `json:"algorithms"`
	UnencryptedKeys []string `json:"unencrypted_keys"`
}

type DeepVersioning struct {
	TotalVersions    int64 `json:"total_versions"`
	NonCurrentCount  int64 `json:"non_current_count"`
	WastedBytes      int64 `json:"wasted_bytes"`
}

type LargeFile struct {
	Key          string    `json:"key"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"last_modified"`
}

type DeepNaming struct {
	Pattern           string   `json:"pattern"`
	NonCompliantCount int      `json:"non_compliant_count"`
	Examples          []string `json:"examples"`
}

type CostBreakdown struct {
	Class       string  `json:"class"`
	MonthlyCost float64 `json:"monthly_cost"`
}

type DeepCostEstimate struct {
	MonthlyTotal float64         `json:"monthly_total"`
	Breakdown    []CostBreakdown `json:"breakdown"`
}

type VirusResult struct {
	Status   string   `json:"status"`
	Scanned  int      `json:"scanned"`
	Infected []string `json:"infected"`
	Errors   []string `json:"errors"`
}

// --- Scan Result (top-level container for one scan) ---

type ScanResult struct {
	Record  ScanRecord       `json:"record"`
	Summary ScanSummary      `json:"summary"`
	Types   []TypeBreakdown  `json:"types,omitempty"`
	Ages    []AgeBucket      `json:"ages,omitempty"`
	Storage []StorageBreakdown `json:"storage,omitempty"`
	Prefixes []PrefixBreakdown `json:"prefixes,omitempty"`
	Delta   *DeltaReport     `json:"delta,omitempty"`

	Duplicates   []DuplicateGroup  `json:"duplicates,omitempty"`
	Multiparts   []MultipartUpload `json:"multiparts,omitempty"`
	AccessAudit  *DeepAccessAudit  `json:"access_audit,omitempty"`
	Encryption   *DeepEncryption   `json:"encryption,omitempty"`
	Versioning   *DeepVersioning   `json:"versioning,omitempty"`
	LargeFiles   []LargeFile       `json:"large_files,omitempty"`
	Naming       *DeepNaming       `json:"naming,omitempty"`
	CostEstimate *DeepCostEstimate `json:"cost_estimate,omitempty"`
	Virus        *VirusResult      `json:"virus,omitempty"`
}
