package handlers

import (
	"context"
	"encoding/json"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
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
	AuthService    *auth.Service
	SessionManager *auth.SessionManager
	ScanEngine     *scan.Engine
	Renderer       *web.TemplateRenderer
	DeepConfig     deep.Config

	mu        sync.RWMutex
	s3Clients map[string]s3.S3Client // projectID -> client
}

func (h *Handler) setS3Client(projectID string, c s3.S3Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.s3Clients == nil {
		h.s3Clients = map[string]s3.S3Client{}
	}
	h.s3Clients[projectID] = c
}

func (h *Handler) s3ClientFor(projectID string) s3.S3Client {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.s3Clients[projectID]
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	staticFS, _ := fs.Sub(web.Assets, "static")
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	r.Get("/login", h.GetLogin)
	r.Post("/login", h.PostLogin)
	r.Post("/logout", h.PostLogout)

	r.Get("/", h.GetDashboard)
	r.Get("/buckets", h.GetBucketsOptions)

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

	loginReq := &auth.LoginRequest{
		Email:    r.FormValue("email"),
		Password: r.FormValue("password"),
		TfaCode:  r.FormValue("tfa_code"),
		TenantID: r.FormValue("tenant_id"),
		APIURL:   r.FormValue("api_url"),
	}

	ctx := context.Background()

	signinResp, err := h.AuthService.Login(ctx, loginReq)
	if err != nil {
		log.Printf("login failed: %v", err)
		renderLogin(w, h.Renderer, "Login failed. Check your credentials and try again.")
		return
	}

	account, err := h.AuthService.GetAccount(ctx, signinResp.Token)
	if err != nil {
		log.Printf("get account failed: %v", err)
		renderLogin(w, h.Renderer, "Failed to retrieve account")
		return
	}

	coordinatorURL := loginReq.APIURL
	if coordinatorURL == "" {
		coordinatorURL = "https://api.eu00wi.cubbit.services"
	}
	if err := h.SessionManager.SaveLogin(ctx, signinResp, account, coordinatorURL); err != nil {
		log.Printf("save login failed: %v", err)
		renderLogin(w, h.Renderer, "Failed to save session")
		return
	}

	projects, err := h.AuthService.GetProjects(ctx, signinResp.Token)
	if err != nil {
		log.Printf("get projects failed (will continue): %v", err)
	} else {
		storeProjects := make([]store.Project, len(projects))
		for i, p := range projects {
			storeProjects[i] = store.Project{ID: p.ID, Name: p.Name}
		}
		h.Store.SaveProjects(ctx, storeProjects)

		if len(projects) > 0 && account.EndpointGateway != "" {
			log.Printf("auth flow: starting forge/keyvault chain over %d projects", len(projects))
			for _, p := range projects {
				log.Printf("auth flow: project %q id=%s users=%d", p.Name, p.ID, len(p.Users))
			}

			// Wipe stale per-project bucket entries from previous (buggy)
			// runs so we don't serve cross-project ghosts.
			if err := h.Store.ClearAllBuckets(ctx); err != nil {
				log.Printf("auth flow: clear buckets failed: %v", err)
			}

			creds := h.loadOrCreateS3Credentials(ctx, projects)
			log.Printf("auth flow: got credentials for %d/%d projects", len(creds), len(projects))

			h.initS3ClientsForProjects(ctx, account.EndpointGateway, creds)
		}
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handler) PostLogout(w http.ResponseWriter, r *http.Request) {
	h.SessionManager.Logout(context.Background())
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// loadOrCreateS3Credentials walks every project and tries to obtain a
// usable (api_key, secret_key) pair per project. For each IAM user in a
// project it reconciles local persistence with the remote keyvault, mirroring
// Swift's DS3SDK.loadOrCreateDS3APIKeys:
//
//  1. Local cache + matching remote → reuse cache (the only source of secret).
//  2. Remote orphan with our name but no local cache → delete it.
//  3. Stale local cache with no matching remote → drop it.
//  4. Create a new key, persist it locally.
//
// Returns a map keyed by projectID.
func (h *Handler) loadOrCreateS3Credentials(ctx context.Context, projects []auth.IAMProject) map[string]*store.S3Credential {
	result := map[string]*store.S3Credential{}
	for _, p := range projects {
		log.Printf("auth flow: project %s users=%d", p.Name, len(p.Users))
		for _, u := range p.Users {
			cred := h.reconcileProjectCredential(ctx, p, u)
			if cred != nil {
				result[p.ID] = cred
				break // one credential per project is enough
			}
		}
	}
	return result
}

func (h *Handler) reconcileProjectCredential(ctx context.Context, p auth.IAMProject, u auth.IAMUser) *store.S3Credential {
	keyName := s3KeyName(u.UserName, p.Name)

	forgeResp, err := h.AuthService.ForgeJWT(ctx, u.UserID)
	_ = h.SessionManager.SyncRefreshToken(ctx)
	if err != nil {
		log.Printf("auth flow: forge for user %s failed: %v", u.UserName, err)
		return nil
	}

	remote, err := h.AuthService.ListApiKeys(ctx, u.UserID, forgeResp.Token)
	if err != nil {
		log.Printf("auth flow: list keys for %s failed: %v", u.UserName, err)
		return nil
	}

	cached, _ := h.Store.GetS3Credential(ctx, keyName)
	var remoteMatch *auth.IAMApiKey
	for i := range remote {
		if remote[i].Name == keyName {
			remoteMatch = &remote[i]
			break
		}
	}

	if cached != nil && remoteMatch != nil && cached.ApiKey == remoteMatch.ApiKey {
		log.Printf("auth flow: reusing cached key %s", keyName)
		return cached
	}

	if remoteMatch != nil {
		log.Printf("auth flow: deleting orphan remote key %s (no local secret)", keyName)
		if delErr := h.AuthService.DeleteApiKey(ctx, remoteMatch.ApiKey, u.UserID, forgeResp.Token); delErr != nil {
			log.Printf("auth flow: delete orphan failed: %v", delErr)
		}
	}
	if cached != nil && remoteMatch == nil {
		_ = h.Store.DeleteS3Credential(ctx, keyName)
	}

	var created *auth.IAMApiKey
	for attempt := 1; attempt <= 3; attempt++ {
		c, err := h.AuthService.CreateApiKey(ctx, keyName, u.UserID, forgeResp.Token)
		if err == nil && c != nil && c.SecretKey != "" {
			created = c
			break
		}
		log.Printf("auth flow: create key %s attempt %d failed: %v (secret_empty=%v)", keyName, attempt, err, c != nil && c.SecretKey == "")
		time.Sleep(time.Duration(attempt) * 400 * time.Millisecond)
	}
	if created == nil {
		return nil
	}
	cred := &store.S3Credential{
		Name:      keyName,
		ApiKey:    created.ApiKey,
		SecretKey: created.SecretKey,
		UserID:    u.UserID,
		ProjectID: p.ID,
	}
	_ = h.Store.SaveS3Credential(ctx, cred)
	log.Printf("auth flow: created and cached key %s", keyName)
	return cred
}

func s3KeyName(userName, projectName string) string {
	safe := func(s string) string {
		s = strings.ToLower(s)
		s = strings.ReplaceAll(s, " ", "_")
		return s
	}
	return "s3lytics-" + safe(userName) + "-" + safe(projectName)
}

// initS3ClientsForProjects creates one S3 client per project using that
// project's credentials, then lists each project's buckets and persists them
// under that project's ID. If a project's own credential fails, falls back
// to the first working credential from another project.
func (h *Handler) initS3ClientsForProjects(ctx context.Context, endpoint string, creds map[string]*store.S3Credential) {
	var fallbackCred *store.S3Credential
	for _, c := range creds {
		if c.ApiKey != "" && c.SecretKey != "" {
			fallbackCred = c
			break
		}
	}

	for projectID, c := range creds {
		if projectID == "" {
			continue
		}
		cred := c
		if cred.ApiKey == "" || cred.SecretKey == "" {
			cred = fallbackCred
		}
		if cred == nil || cred.ApiKey == "" || cred.SecretKey == "" {
			log.Printf("auth flow: no credentials for project %s", projectID)
			continue
		}
		client, err := s3.NewCubbitS3Client(endpoint, "us-east-1", cred.ApiKey, cred.SecretKey)
		if err != nil {
			log.Printf("create s3 client for project %s: %v", projectID, err)
			continue
		}
		h.setS3Client(projectID, client)

		buckets, err := h.listBucketsWithRetry(ctx, client, projectID)
		if err != nil {
			log.Printf("list buckets for project %s: %v", projectID, err)
			continue
		}
		storeBuckets := make([]store.Bucket, len(buckets))
		bucketNames := make([]string, 0, len(buckets))
		for i, b := range buckets {
			storeBuckets[i] = store.Bucket{Name: b.Name, CreationDate: b.CreationDate.Format(time.RFC3339)}
			if len(bucketNames) < 5 {
				bucketNames = append(bucketNames, b.Name)
			}
		}
		if err := h.Store.SaveBuckets(ctx, projectID, storeBuckets); err != nil {
			log.Printf("save buckets for project %s: %v", projectID, err)
			continue
		}
		log.Printf("auth flow: project %s -> %d buckets (sample: %v)", projectID, len(buckets), bucketNames)
	}
}

func (h *Handler) listBucketsWithRetry(ctx context.Context, client s3.S3Client, projectID string) ([]s3.BucketInfo, error) {
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		buckets, err := client.ListBuckets(ctx)
		if err == nil {
			return buckets, nil
		}
		lastErr = err
		log.Printf("auth flow: list buckets project %s attempt %d failed: %v", projectID, attempt, err)
		time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
	}
	return nil, lastErr
}

// RestoreS3Clients rebuilds per-project S3 clients from cached credentials
// after an app restart. Call once at startup.
func (h *Handler) RestoreS3Clients(ctx context.Context) {
	acct, err := h.Store.GetAccount(ctx)
	if err != nil || acct == nil || acct.EndpointGateway == "" {
		return
	}
	creds, err := h.Store.ListS3Credentials(ctx)
	if err != nil {
		return
	}
	for _, c := range creds {
		if c.ProjectID == "" || c.ApiKey == "" || c.SecretKey == "" {
			continue
		}
		client, err := s3.NewCubbitS3Client(acct.EndpointGateway, "us-east-1", c.ApiKey, c.SecretKey)
		if err != nil {
			log.Printf("restore s3 client for project %s: %v", c.ProjectID, err)
			continue
		}
		h.setS3Client(c.ProjectID, client)
	}
}

func (h *Handler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	projects, err := h.Store.GetProjects(r.Context())
	if err != nil {
		log.Printf("get projects from store: %v", err)
	}

	acct, _ := h.Store.GetAccount(r.Context())
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
	if err := h.Renderer.Render(w, "layout.html", data); err != nil {
		log.Printf("render error on %s: %v", r.URL.Path, err)
	}
}

func (h *Handler) GetBucketsOptions(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project")
	if projectID == "" {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<option value="">Select a project first...</option>`))
		return
	}

	buckets, err := h.Store.GetBuckets(r.Context(), projectID)
	if err != nil || len(buckets) == 0 {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<option value="">No buckets found</option>`))
		return
	}

	w.Header().Set("Content-Type", "text/html")
	var sb strings.Builder
	for _, b := range buckets {
		sb.WriteString(`<option value="`)
		sb.WriteString(template.HTMLEscapeString(b.Name))
		sb.WriteString(`">`)
		sb.WriteString(template.HTMLEscapeString(b.Name))
		sb.WriteString(`</option>`)
	}
	w.Write([]byte(sb.String()))
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
	projectID := r.FormValue("project")
	if projectID == "" {
		http.Error(w, "project required", http.StatusBadRequest)
		return
	}

	client := h.s3ClientFor(projectID)
	if client == nil {
		http.Error(w, "no S3 client for project (re-login may be required)", http.StatusBadRequest)
		return
	}
	h.ScanEngine.SetS3Client(client)

	ctx := r.Context()

	var scanID string
	var err error

	if r.FormValue("incremental") == "true" {
		scanID, err = h.ScanEngine.StartIncrementalScan(ctx, bucket)
	} else {
		scanID, err = h.ScanEngine.StartFullScan(ctx, bucket)
	}

	if err != nil {
		http.Error(w, "Scan failed to start", http.StatusInternalServerError)
		return
	}

	redirectURL := "/scan/" + scanID + "/progress"
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", redirectURL)
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

func (h *Handler) GetScanProgress(w http.ResponseWriter, r *http.Request) {
	scanID := chi.URLParam(r, "id")
	progress := h.ScanEngine.GetProgress()

	data := &web.PageData{
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

	// HTMX poll requests get just the fragment; browser nav gets the full layout
	if r.Header.Get("HX-Request") == "true" {
		if err := h.Renderer.Render(w, "scan_progress", data); err != nil {
			log.Printf("render error on %s: %v", r.URL.Path, err)
		}
	} else {
		data.LoggedIn = true
		data.Page = "scan_progress"
		acct, _ := h.Store.GetAccount(r.Context())
		if acct != nil {
			data.AccountEmail = acct.Email
		}
		if err := h.Renderer.Render(w, "layout.html", data); err != nil {
			log.Printf("render error on %s: %v", r.URL.Path, err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	}
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

	result, err := h.Store.GetScanResult(r.Context(), scanID)
	if err != nil {
		http.Error(w, "scan not found", http.StatusNotFound)
		return
	}

	acct, _ := h.Store.GetAccount(r.Context())
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

	if err := h.Renderer.Render(w, "layout.html", data); err != nil {
		log.Printf("render error on %s: %v", r.URL.Path, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
}

func (h *Handler) PostDeleteScan(w http.ResponseWriter, r *http.Request) {
	scanID := chi.URLParam(r, "id")
	if err := h.Store.DeleteScan(r.Context(), scanID); err != nil {
		log.Printf("delete scan %s: %v", scanID, err)
	}
	http.Redirect(w, r, "/history", http.StatusSeeOther)
}

func (h *Handler) GetHistory(w http.ResponseWriter, r *http.Request) {
	bucket := r.URL.Query().Get("bucket")

	records, err := h.Store.ListScans(r.Context(), bucket)
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
		result, err := h.Store.GetScanResult(r.Context(), item.ID)
		if err == nil && result != nil {
			items[i].TotalObjects = result.Summary.TotalObjects
		}
	}

	var buckets []string
	for b := range bucketSet {
		buckets = append(buckets, b)
	}

	acct, _ := h.Store.GetAccount(r.Context())
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

	if err := h.Renderer.Render(w, "layout.html", data); err != nil {
		log.Printf("render error on %s: %v", r.URL.Path, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
}

func (h *Handler) GetComparison(w http.ResponseWriter, r *http.Request) {
	scans := r.URL.Query()["scans"]
	if len(scans) != 2 {
		http.Error(w, "need exactly 2 scan IDs", http.StatusBadRequest)
		return
	}

	resultA, errA := h.Store.GetScanResult(r.Context(), scans[0])
	resultB, errB := h.Store.GetScanResult(r.Context(), scans[1])

	if errA != nil || errB != nil {
		http.Error(w, "scan not found", http.StatusNotFound)
		return
	}

	objDelta := int64(resultB.Summary.TotalObjects) - int64(resultA.Summary.TotalObjects)
	sizeDelta := resultB.Summary.TotalSize - resultA.Summary.TotalSize

	acct, _ := h.Store.GetAccount(r.Context())
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

	if err := h.Renderer.Render(w, "layout.html", data); err != nil {
		log.Printf("render error on %s: %v", r.URL.Path, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
}

func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	settings := h.loadSettings()

	h.ScanEngine.SetConfig(scan.Config{
		Workers:       settings.ScanWorkers,
		BatchSize:     settings.ScanBatchSize,
		PrefixTimeout: time.Duration(settings.ScanPrefixTimeoutSec) * time.Second,
	})

	acct, _ := h.Store.GetAccount(r.Context())
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
	if err := h.Renderer.Render(w, "layout.html", data); err != nil {
		log.Printf("render error on %s: %v", r.URL.Path, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
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

	if workers, err := strconv.Atoi(r.FormValue("scan_workers")); err == nil && workers >= 1 && workers <= 32 {
		settings.ScanWorkers = workers
	}
	if batchSize, err := strconv.Atoi(r.FormValue("scan_batch_size")); err == nil && batchSize >= 100 && batchSize <= 5000 {
		settings.ScanBatchSize = batchSize
	}
	if timeout, err := strconv.Atoi(r.FormValue("scan_prefix_timeout")); err == nil && timeout >= 10 && timeout <= 120 {
		settings.ScanPrefixTimeoutSec = timeout
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
	_ = h.Store.SaveScanResult(r.Context(), settingsRecord)

	h.ScanEngine.SetConfig(scan.Config{
		Workers:       settings.ScanWorkers,
		BatchSize:     settings.ScanBatchSize,
		PrefixTimeout: time.Duration(settings.ScanPrefixTimeoutSec) * time.Second,
	})

	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (h *Handler) loadSettings() *web.SettingsData {
	return &web.SettingsData{
		ClamdSocket:          "/var/run/clamav/clamd.sock",
		DeepDuplicates:       true,
		DeepMultipart:        true,
		DeepAccess:           true,
		DeepEncryption:       true,
		DeepVersioning:       true,
		DeepLargeFiles:       true,
		DeepNaming:           true,
		DeepCost:             true,
		NamingPattern:        "",
		LargeFileThresholdMB: 100,
		ScanWorkers:          4,
		ScanBatchSize:        500,
		ScanPrefixTimeoutSec: 30,
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

func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("X-XSS-Protection", "0")
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		next.ServeHTTP(w, r)
	})
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
