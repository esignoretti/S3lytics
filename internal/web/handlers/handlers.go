package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/esignoretti/s3lytics/internal/auth"
	"github.com/esignoretti/s3lytics/internal/s3"
	"github.com/esignoretti/s3lytics/internal/scan"
	"github.com/esignoretti/s3lytics/internal/scan/deep"
	"github.com/esignoretti/s3lytics/internal/store"
	"github.com/esignoretti/s3lytics/internal/web"
)

type Handler struct {
	Store          store.Store
	SessionManager *auth.SessionManager
	ScanEngine     *scan.Engine
	S3Client       s3.S3Client
	Renderer       *web.TemplateRenderer
	DeepConfig     deep.Config
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	staticFS, _ := fs.Sub(web.Assets, "static")
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	r.Get("/login", h.GetLogin)
	r.Post("/login", h.PostLogin)
	r.Post("/logout", h.PostLogout)

	r.Get("/", h.GetDashboard)
	r.Get("/buckets", h.GetBucketsJSON)

	r.Post("/scan/start", h.PostStartScan)
	r.Get("/scan/{id}", h.GetScanReport)
	r.Get("/scan/{id}/progress", h.GetScanProgress)
	r.Get("/scan/{id}/status", h.GetScanStatus)
	r.Post("/scan/{id}/delete", h.PostDeleteScan)

	r.Get("/history", h.GetHistory)

	r.Get("/compare", h.GetComparison)

	r.Get("/settings", h.GetSettings)
	r.Post("/settings", h.PostSettings)
}

func (h *Handler) GetLogin(w http.ResponseWriter, r *http.Request) {
	renderLogin(w, h.Renderer, "")
}

func (h *Handler) PostLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		renderLogin(w, h.Renderer, "Invalid form")
		return
	}

	endpoint := r.FormValue("endpoint")
	region := r.FormValue("region")
	accessKey := r.FormValue("access_key")
	secretKey := r.FormValue("secret_key")

	if endpoint == "" || accessKey == "" || secretKey == "" {
		renderLogin(w, h.Renderer, "Endpoint, Access Key, and Secret Key are required")
		return
	}
	if region == "" {
		region = "us-east-1"
	}

	ctx := context.Background()

	s3Client, err := s3.NewCubbitS3Client(endpoint, region, accessKey, secretKey)
	if err != nil {
		log.Printf("S3 client creation failed: %v", err)
		renderLogin(w, h.Renderer, fmt.Sprintf("Failed to create S3 client: %v", err))
		return
	}

	if err := h.SessionManager.SaveLogin(ctx, endpoint, region, accessKey, secretKey); err != nil {
		log.Printf("save login failed: %v", err)
		renderLogin(w, h.Renderer, "Failed to save session")
		return
	}

	h.S3Client = s3Client
	h.ScanEngine.SetS3Client(s3Client)

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handler) PostLogout(w http.ResponseWriter, r *http.Request) {
	h.SessionManager.Logout(context.Background())
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (h *Handler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	projects, _ := h.Store.GetProjects(ctx)

	acct, _ := h.Store.GetAccount(ctx)
	email := ""
	if acct != nil {
		email = acct.Email
	}

	data := &web.PageData{
		LoggedIn:     true,
		Page:         "dashboard",
		AccountEmail: email,
		Projects:     projects,
	}
	_ = h.Renderer.Render(w, "layout.html", data)
}

func (h *Handler) GetBucketsJSON(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project")
	if projectID == "" {
		http.Error(w, "missing project", http.StatusBadRequest)
		return
	}

	buckets, err := h.Store.GetBuckets(context.Background(), projectID)
	if err != nil || len(buckets) == 0 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]store.Bucket{})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(buckets)
}

func (h *Handler) PostStartScan(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	bucket := r.FormValue("bucket")
	if bucket == "" {
		http.Error(w, "bucket required", http.StatusBadRequest)
		return
	}

	ctx := context.Background()

	var scanID string
	var err error

	if r.FormValue("incremental") == "true" {
		scanID, err = h.ScanEngine.StartIncrementalScan(ctx, bucket)
	} else {
		scanID, err = h.ScanEngine.StartFullScan(ctx, bucket)
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/scan/"+scanID+"/progress", http.StatusSeeOther)
}

func (h *Handler) GetScanProgress(w http.ResponseWriter, r *http.Request) {
	scanID := chi.URLParam(r, "id")
	progress := h.ScanEngine.GetProgress()

	data := &web.PageData{
		LoggedIn:     true,
		Page:         "scan_progress",
		ScanID:       scanID,
		Bucket:       "Loading...",
		ObjectsFound: 0,
		Elapsed:      "0s",
		ScanType:     "full",
	}

	if progress != nil {
		data.Bucket = progress.Bucket
		data.ObjectsFound = progress.ObjectsDone
		data.Elapsed = progress.Elapsed
		data.Status = progress.Status
		data.ProgressPct = 50.0
	}

	_ = h.Renderer.Render(w, "layout.html", data)
}

func (h *Handler) GetScanStatus(w http.ResponseWriter, r *http.Request) {
	scanID := chi.URLParam(r, "id")
	progress := h.ScanEngine.GetProgress()

	resp := map[string]string{
		"scan_id": scanID,
		"status":  "running",
	}
	if progress != nil {
		resp["status"] = progress.Status
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) GetScanReport(w http.ResponseWriter, r *http.Request) {
	scanID := chi.URLParam(r, "id")
	ctx := context.Background()

	result, err := h.Store.GetScanResult(ctx, scanID)
	if err != nil {
		http.Error(w, "scan not found", http.StatusNotFound)
		return
	}

	acct, _ := h.Store.GetAccount(ctx)
	email := ""
	if acct != nil {
		email = acct.Email
	}

	data := &web.PageData{
		LoggedIn:     true,
		Page:         "scan_report",
		AccountEmail: email,
		Result:       result,
	}

	_ = h.Renderer.Render(w, "layout.html", data)
}

func (h *Handler) PostDeleteScan(w http.ResponseWriter, r *http.Request) {
	scanID := chi.URLParam(r, "id")
	if err := h.Store.DeleteScan(context.Background(), scanID); err != nil {
		log.Printf("delete scan %s: %v", scanID, err)
	}
	http.Redirect(w, r, "/history", http.StatusSeeOther)
}

func (h *Handler) GetHistory(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	bucket := r.URL.Query().Get("bucket")

	records, err := h.Store.ListScans(ctx, bucket)
	if err != nil {
		records = []store.ScanRecord{}
	}

	bucketSet := make(map[string]bool)
	var items []web.ScanListItem
	for _, rec := range records {
		bucketSet[rec.Bucket] = true
		items = append(items, web.ScanListItem{
			ScanRecord: rec,
		})
	}

	for i, item := range items {
		result, err := h.Store.GetScanResult(ctx, item.ID)
		if err == nil && result != nil {
			items[i].TotalObjects = result.Summary.TotalObjects
		}
	}

	var buckets []string
	for b := range bucketSet {
		buckets = append(buckets, b)
	}

	acct, _ := h.Store.GetAccount(ctx)
	email := ""
	if acct != nil {
		email = acct.Email
	}

	data := &web.PageData{
		LoggedIn:       true,
		Page:           "history",
		AccountEmail:   email,
		Scans:          items,
		Buckets:        buckets,
		SelectedBucket: bucket,
	}

	_ = h.Renderer.Render(w, "layout.html", data)
}

func (h *Handler) GetComparison(w http.ResponseWriter, r *http.Request) {
	scans := r.URL.Query()["scans"]
	if len(scans) != 2 {
		http.Error(w, "need exactly 2 scan IDs", http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	resultA, errA := h.Store.GetScanResult(ctx, scans[0])
	resultB, errB := h.Store.GetScanResult(ctx, scans[1])

	if errA != nil || errB != nil {
		http.Error(w, "scan not found", http.StatusNotFound)
		return
	}

	objDelta := int64(resultB.Summary.TotalObjects) - int64(resultA.Summary.TotalObjects)
	sizeDelta := resultB.Summary.TotalSize - resultA.Summary.TotalSize

	acct, _ := h.Store.GetAccount(ctx)
	email := ""
	if acct != nil {
		email = acct.Email
	}

	data := &web.PageData{
		LoggedIn:     true,
		Page:         "comparison",
		AccountEmail: email,
		ScanA:        resultA,
		ScanB:        resultB,
		ObjectDelta:  objDelta,
		SizeDelta:    sizeDelta,
	}

	_ = h.Renderer.Render(w, "layout.html", data)
}

func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	settings := h.loadSettings()

	ctx := context.Background()
	acct, _ := h.Store.GetAccount(ctx)
	email := ""
	if acct != nil {
		email = acct.Email
	}

	data := &web.PageData{
		LoggedIn:     true,
		Page:         "settings",
		AccountEmail: email,
		Settings:     settings,
	}
	_ = h.Renderer.Render(w, "layout.html", data)
}

func (h *Handler) PostSettings(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	settings := h.loadSettings()
	settings.ClamdSocket = r.FormValue("clamd_socket")
	settings.DeepDuplicates = r.FormValue("deep_duplicates") == "on"
	settings.DeepMultipart = r.FormValue("deep_multipart") == "on"
	settings.DeepAccess = r.FormValue("deep_access") == "on"
	settings.DeepEncryption = r.FormValue("deep_encryption") == "on"
	settings.DeepVersioning = r.FormValue("deep_versioning") == "on"
	settings.DeepLargeFiles = r.FormValue("deep_large_files") == "on"
	settings.DeepNaming = r.FormValue("deep_naming") == "on"
	settings.DeepCost = r.FormValue("deep_cost") == "on"
	settings.NamingPattern = r.FormValue("naming_pattern")

	if threshold, err := strconv.ParseInt(r.FormValue("large_file_threshold"), 10, 64); err == nil {
		settings.LargeFileThresholdMB = threshold
	}

	for class := range settings.CostRates {
		if val := r.FormValue("cost_" + class); val != "" {
			if rate, err := strconv.ParseFloat(val, 64); err == nil {
				settings.CostRates[class] = rate
			}
		}
	}

	settingsData, _ := json.Marshal(settings)
	settingsRecord := &store.ScanResult{
		Record: store.ScanRecord{
			ID:        "__settings__",
			Bucket:    "__global__",
			Timestamp: time.Now(),
			Status:    "settings",
			ScanType:  "settings",
		},
		Summary: store.ScanSummary{
			TotalObjects: int64(len(settingsData)),
		},
	}
	_ = h.Store.SaveScanResult(context.Background(), settingsRecord)

	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (h *Handler) loadSettings() *web.SettingsData {
	return &web.SettingsData{
		ClamdSocket:         "/var/run/clamav/clamd.sock",
		DeepDuplicates:      true,
		DeepMultipart:       true,
		DeepAccess:          true,
		DeepEncryption:      true,
		DeepVersioning:      true,
		DeepLargeFiles:      true,
		DeepNaming:          true,
		DeepCost:            true,
		NamingPattern:       "",
		LargeFileThresholdMB: 100,
		CostRates: map[string]float64{
			"STANDARD":                  0.023,
			"INTELLIGENT_TIERING":       0.023,
			"STANDARD_IA":               0.0125,
			"ONEZONE_IA":                0.01,
			"GLACIER":                   0.004,
			"DEEP_ARCHIVE":              0.002,
			"GLACIER_INSTANT_RETRIEVAL": 0.004,
		},
	}
}

func renderLogin(w http.ResponseWriter, r *web.TemplateRenderer, errMsg string) {
	data := &web.PageData{
		Error: errMsg,
	}
	if err := r.Render(w, "layout.html", data); err != nil {
		log.Printf("render login error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}
