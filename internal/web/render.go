package web

import (
	"fmt"
	"html/template"
	"io"
	"path/filepath"
	"strings"

	"github.com/esignoretti/s3lytics/internal/store"
)

type PageData struct {
	LoggedIn     bool
	Page         string
	AccountEmail string
	Error        string

	Projects []store.Project

	Result *store.ScanResult

	ScanID       string
	Bucket       string
	ProgressPct  float64
	ObjectsFound int64
	Elapsed      string
	ScanType     string
	Status       string

	Scans          []ScanListItem
	Buckets        []string
	SelectedBucket string

	ScanA       *store.ScanResult
	ScanB       *store.ScanResult
	ObjectDelta int64
	SizeDelta   int64
	TrendData   interface{}

	Settings *SettingsData
}

type ScanListItem struct {
	store.ScanRecord
	TotalObjects int64
}

type SettingsData struct {
	ClamdSocket          string
	DeepDuplicates       bool
	DeepMultipart        bool
	DeepAccess           bool
	DeepEncryption       bool
	DeepVersioning       bool
	DeepLargeFiles       bool
	DeepNaming           bool
	DeepCost             bool
	NamingPattern        string
	LargeFileThresholdMB int64
	CostRates            map[string]float64
	ScanWorkers          int     `json:"scan_workers"`
	ScanBatchSize        int     `json:"scan_batch_size"`
	ScanPrefixTimeoutSec int     `json:"scan_prefix_timeout"`
}

type TemplateRenderer struct {
	templates *template.Template
}

func NewTemplateRenderer() (*TemplateRenderer, error) {
	funcMap := template.FuncMap{
		"formatBytes": formatBytes,
		"join":        strings.Join,
	}

	patterns := []string{"templates/*.html"}
	var allFiles []string
	for _, pattern := range patterns {
		files, err := Assets.ReadDir(filepath.Dir(pattern))
		if err != nil {
			return nil, fmt.Errorf("read dir: %w", err)
		}
		for _, f := range files {
			if !f.IsDir() && strings.HasSuffix(f.Name(), ".html") {
				allFiles = append(allFiles, "templates/"+f.Name())
			}
		}
	}

	tmpl := template.New("layout.html").Funcs(funcMap)
	for _, name := range allFiles {
		content, err := Assets.ReadFile(name)
		if err != nil {
			return nil, fmt.Errorf("read template %s: %w", name, err)
		}
		_, err = tmpl.Parse(string(content))
		if err != nil {
			return nil, fmt.Errorf("parse template %s: %w", name, err)
		}
	}

	return &TemplateRenderer{templates: tmpl}, nil
}

func (r *TemplateRenderer) Render(w io.Writer, name string, data *PageData) error {
	return r.templates.ExecuteTemplate(w, name, data)
}

func formatBytes(v interface{}) string {
	var b int64
	switch val := v.(type) {
	case int64:
		b = val
	case float64:
		b = int64(val)
	default:
		return "0 B"
	}
	if b == 0 {
		return "0 B"
	}
	if b < 0 {
		b = -b
	}
	if b < 1024 {
		return fmt.Sprintf("%d B", b)
	}
	if b < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	}
	if b < 1024*1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(b)/(1024*1024))
	}
	return fmt.Sprintf("%.2f GB", float64(b)/(1024*1024*1024))
}
