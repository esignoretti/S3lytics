# S3lytics — Phase 7: Web UI (Templates, Static Assets, HTTP Handlers)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the complete web UI: Go templates rendered server-side with HTMX for partial updates, Chart.js for visualizations, and a sidebar-based layout. Pages: Login, Dashboard, Scan Report, Scan Progress, History, Comparison, and Settings.

**Architecture:** Package `internal/web/` contains `handlers/` (HTTP handlers), `templates/` (Go html/template files), and `static/` (CSS, JS). Templates use `embed.FS` for single-binary deployment. HTMX handles dynamic updates via partial template rendering. Chart.js is loaded from CDN.

**Tech Stack:** `html/template`, `embed`, `github.com/go-chi/chi/v5`, HTMX (CDN), Chart.js (CDN), plain CSS

**Pre-requisites:** Phases 1-6 complete.

---

### Task 1: Embed templates and static assets with embed.FS

**Files:**
- Create: `internal/web/templates/layout.html`
- Create: `internal/web/templates/login.html`
- Create: `internal/web/templates/dashboard.html`
- Create: `internal/web/templates/scan_report.html`
- Create: `internal/web/templates/scan_progress.html`
- Create: `internal/web/templates/history.html`
- Create: `internal/web/templates/comparison.html`
- Create: `internal/web/templates/settings.html`
- Create: `internal/web/static/style.css`
- Create: `internal/web/embed.go`

- [ ] **Step 1: Create embed.go with embed.FS**

```go
package web

import "embed"

//go:embed templates/*.html static/*.css
var Assets embed.FS
```

- [ ] **Step 2: Write the layout template**

```html
<!-- internal/web/templates/layout.html -->
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>S3lytics</title>
    <script src="https://cdn.jsdelivr.net/npm/chart.js@4.4.1/dist/chart.umd.min.js"></script>
    <script src="https://unpkg.com/htmx.org@1.9.10"></script>
    <link rel="stylesheet" href="/static/style.css">
</head>
<body>
    <div class="app">
        {{if .LoggedIn}}
        <nav class="sidebar">
            <div class="sidebar-header">
                <h1>S3lytics</h1>
                <span class="version">v0.1.0</span>
            </div>
            <ul class="sidebar-nav">
                <li><a href="/" class="{{if eq .Page "dashboard"}}active{{end}}">Dashboard</a></li>
                <li><a href="/history" class="{{if eq .Page "history"}}active{{end}}">Scan History</a></li>
                <li><a href="/settings" class="{{if eq .Page "settings"}}active{{end}}">Settings</a></li>
            </ul>
            <div class="sidebar-footer">
                <span class="account-email">{{.AccountEmail}}</span>
                <form action="/logout" method="POST" style="display:inline;">
                    <button type="submit" class="btn-link">Logout</button>
                </form>
            </div>
        </nav>
        <main class="main-content">
            {{block "content" .}}{{end}}
        </main>
        {{else}}
        <main class="main-content login-page">
            {{block "content" .}}{{end}}
        </main>
        {{end}}
    </div>
</body>
</html>
```

- [ ] **Step 3: Write style.css**

```css
/* internal/web/static/style.css */
*, *::before, *::after {
    box-sizing: border-box;
    margin: 0;
    padding: 0;
}

:root {
    --bg: #0f1117;
    --surface: #1a1d27;
    --surface-2: #242736;
    --border: #2e3245;
    --text: #e1e4ed;
    --text-muted: #8b8fa3;
    --primary: #4f8cff;
    --primary-hover: #3a7aff;
    --green: #34d399;
    --red: #f87171;
    --yellow: #fbbf24;
    --radius: 8px;
}

body {
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
    background: var(--bg);
    color: var(--text);
    line-height: 1.5;
}

.app {
    display: flex;
    min-height: 100vh;
}

/* Sidebar */
.sidebar {
    width: 240px;
    background: var(--surface);
    border-right: 1px solid var(--border);
    padding: 1.5rem;
    display: flex;
    flex-direction: column;
    flex-shrink: 0;
}

.sidebar-header h1 {
    font-size: 1.25rem;
    font-weight: 700;
    color: var(--primary);
}

.version {
    font-size: 0.75rem;
    color: var(--text-muted);
    display: block;
    margin-top: 0.25rem;
}

.sidebar-nav {
    list-style: none;
    margin-top: 2rem;
    flex: 1;
}

.sidebar-nav li {
    margin-bottom: 0.5rem;
}

.sidebar-nav a {
    display: block;
    padding: 0.625rem 0.75rem;
    color: var(--text-muted);
    text-decoration: none;
    border-radius: var(--radius);
    transition: all 0.15s;
}

.sidebar-nav a:hover, .sidebar-nav a.active {
    background: var(--surface-2);
    color: var(--text);
}

.sidebar-footer {
    border-top: 1px solid var(--border);
    padding-top: 1rem;
    font-size: 0.875rem;
}

.account-email {
    display: block;
    margin-bottom: 0.5rem;
    color: var(--text-muted);
}

/* Main content */
.main-content {
    flex: 1;
    padding: 2rem;
    overflow-y: auto;
}

.login-page {
    display: flex;
    align-items: center;
    justify-content: center;
}

/* Cards */
.card {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 1.5rem;
    margin-bottom: 1.5rem;
}

.card h2 {
    font-size: 1.125rem;
    margin-bottom: 1rem;
    color: var(--text);
}

.card-row {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
    gap: 1rem;
    margin-bottom: 1.5rem;
}

.stat-card {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 1.25rem;
    text-align: center;
}

.stat-card .stat-value {
    font-size: 1.75rem;
    font-weight: 700;
    color: var(--primary);
}

.stat-card .stat-label {
    font-size: 0.875rem;
    color: var(--text-muted);
    margin-top: 0.25rem;
}

/* Forms */
.form-group {
    margin-bottom: 1rem;
}

.form-group label {
    display: block;
    margin-bottom: 0.375rem;
    font-size: 0.875rem;
    color: var(--text-muted);
}

.form-group input, .form-group select {
    width: 100%;
    padding: 0.625rem 0.75rem;
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    color: var(--text);
    font-size: 0.875rem;
}

.form-group input:focus {
    outline: none;
    border-color: var(--primary);
}

.login-box {
    width: 400px;
    max-width: 90vw;
}

.login-box h1 {
    margin-bottom: 1.5rem;
    font-size: 1.5rem;
    color: var(--primary);
}

/* Buttons */
.btn, button[type="submit"] {
    padding: 0.625rem 1.25rem;
    background: var(--primary);
    color: white;
    border: none;
    border-radius: var(--radius);
    font-size: 0.875rem;
    cursor: pointer;
    transition: background 0.15s;
}

.btn:hover, button[type="submit"]:hover {
    background: var(--primary-hover);
}

.btn-secondary {
    background: var(--surface-2);
    border: 1px solid var(--border);
    color: var(--text);
}

.btn-secondary:hover {
    background: var(--border);
}

.btn-danger {
    background: var(--red);
}

.btn-danger:hover {
    background: #ef4444;
}

.btn-link {
    background: none;
    border: none;
    color: var(--primary);
    cursor: pointer;
    font-size: 0.875rem;
    padding: 0;
}

.btn-link:hover {
    text-decoration: underline;
}

/* Tables */
table {
    width: 100%;
    border-collapse: collapse;
    font-size: 0.875rem;
}

th, td {
    padding: 0.75rem 1rem;
    text-align: left;
    border-bottom: 1px solid var(--border);
}

th {
    color: var(--text-muted);
    font-weight: 600;
}

tbody tr:hover {
    background: var(--surface-2);
}

/* Badges */
.badge {
    display: inline-block;
    padding: 0.125rem 0.5rem;
    border-radius: 999px;
    font-size: 0.75rem;
    font-weight: 600;
}

.badge-green { background: #065f46; color: var(--green); }
.badge-yellow { background: #78350f; color: var(--yellow); }
.badge-red { background: #7f1d1d; color: var(--red); }

/* Chart containers */
.chart-container {
    position: relative;
    height: 300px;
    margin-bottom: 1.5rem;
}

.chart-container canvas {
    max-height: 300px;
}

/* Accordion */
.accordion-section {
    border: 1px solid var(--border);
    border-radius: var(--radius);
    margin-bottom: 0.75rem;
}

.accordion-header {
    padding: 1rem;
    background: var(--surface-2);
    cursor: pointer;
    font-weight: 600;
    display: flex;
    justify-content: space-between;
    align-items: center;
}

.accordion-body {
    padding: 1rem;
    display: none;
}

.accordion-section.open .accordion-body {
    display: block;
}

/* Progress bar */
.progress-bar {
    width: 100%;
    height: 0.75rem;
    background: var(--surface-2);
    border-radius: 999px;
    overflow: hidden;
}

.progress-bar-fill {
    height: 100%;
    background: var(--primary);
    border-radius: 999px;
    transition: width 0.5s ease;
}

/* Delta indicators */
.delta-up { color: var(--green); }
.delta-down { color: var(--red); }

/* Compare layout */
.compare-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 1.5rem;
}

/* Settings */
.settings-section {
    margin-bottom: 2rem;
}

.settings-section h2 {
    border-bottom: 1px solid var(--border);
    padding-bottom: 0.75rem;
    margin-bottom: 1.25rem;
}

/* Tooltip */
.tooltip {
    position: relative;
    cursor: help;
}

/* Empty state */
.empty-state {
    text-align: center;
    padding: 3rem;
    color: var(--text-muted);
}

.empty-state h3 {
    margin-bottom: 0.5rem;
    color: var(--text);
}

/* Checkbox groups */
.checkbox-group {
    display: flex;
    flex-wrap: wrap;
    gap: 1rem;
    margin-bottom: 1rem;
}

.checkbox-group label {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.875rem;
    cursor: pointer;
}

/* Login form extras */
.form-help {
    font-size: 0.75rem;
    color: var(--text-muted);
    margin-top: 0.25rem;
}

.error-message {
    background: #7f1d1d;
    border: 1px solid var(--red);
    color: var(--red);
    padding: 0.75rem;
    border-radius: var(--radius);
    margin-bottom: 1rem;
    font-size: 0.875rem;
}
```

- [ ] **Step 4: Write login template**

```html
<!-- internal/web/templates/login.html -->
{{define "content"}}
<div class="card login-box">
    <h1>S3lytics</h1>
    <p style="color: var(--text-muted); margin-bottom: 1.5rem;">Sign in with your Cubbit IAM account</p>

    {{if .Error}}
    <div class="error-message">{{.Error}}</div>
    {{end}}

    <form action="/login" method="POST">
        <div class="form-group">
            <label for="email">Email</label>
            <input type="email" id="email" name="email" required placeholder="user@example.com">
        </div>

        <div class="form-group">
            <label for="password">Password</label>
            <input type="password" id="password" name="password" required>
        </div>

        <div class="form-group">
            <label for="tfa_code">2FA Code <span class="form-help">(optional)</span></label>
            <input type="text" id="tfa_code" name="tfa_code" placeholder="000000">
        </div>

        <div class="form-group">
            <label for="tenant_id">Tenant ID <span class="form-help">(optional)</span></label>
            <input type="text" id="tenant_id" name="tenant_id" placeholder="tenant-id">
        </div>

        <div class="form-group">
            <label for="api_url">API Server URL <span class="form-help">(optional, defaults to Cubbit IAM)</span></label>
            <input type="url" id="api_url" name="api_url" placeholder="https://iam.cubbit.eu">
        </div>

        <button type="submit" class="btn" style="width:100%;">Sign In</button>
    </form>
</div>
{{end}}
```

- [ ] **Step 5: Write dashboard template**

```html
<!-- internal/web/templates/dashboard.html -->
{{define "content"}}
<h2 style="margin-bottom: 1.5rem;">Dashboard</h2>

<div class="card">
    <h2>Select Bucket</h2>
    <form hx-post="/scan/start" hx-target="#scan-result" hx-swap="innerHTML">
        <div class="form-group">
            <label for="project">Project</label>
            <select id="project" name="project" hx-get="/buckets" hx-target="#bucket" hx-trigger="change">
                <option value="">Select a project...</option>
                {{range .Projects}}
                <option value="{{.ID}}">{{.Name}}</option>
                {{end}}
            </select>
        </div>

        <div class="form-group">
            <label for="bucket">Bucket</label>
            <select id="bucket" name="bucket">
                <option value="">Select a project first...</option>
            </select>
        </div>

        <div class="form-group">
            <label>Deep Scan Features</label>
            <div class="checkbox-group">
                <label><input type="checkbox" name="deep_duplicates" checked> Duplicates</label>
                <label><input type="checkbox" name="deep_multipart" checked> Multipart Uploads</label>
                <label><input type="checkbox" name="deep_access" checked> Access Audit</label>
                <label><input type="checkbox" name="deep_encryption" checked> Encryption</label>
                <label><input type="checkbox" name="deep_versioning" checked> Versioning</label>
                <label><input type="checkbox" name="deep_large_files" checked> Large Files</label>
                <label><input type="checkbox" name="deep_naming" checked> Naming</label>
                <label><input type="checkbox" name="deep_cost" checked> Cost Estimate</label>
                <label><input type="checkbox" name="deep_virus"> Virus Scan</label>
            </div>
        </div>

        <div class="form-group" style="display:flex; gap:0.75rem;">
            <button type="submit" class="btn">Start Full Scan</button>
            <button type="submit" name="incremental" value="true" class="btn btn-secondary">Start Incremental Scan</button>
        </div>

        <div style="margin-top:0.75rem; display:flex; gap:0.75rem;">
            <a href="/history" class="btn btn-secondary">View History</a>
        </div>
    </form>
</div>

<div id="scan-result"></div>
{{end}}
```

- [ ] **Step 6: Write scan progress template**

```html
<!-- internal/web/templates/scan_progress.html -->
{{define "content"}}
<h2 style="margin-bottom: 1.5rem;">Scan in Progress</h2>

<div class="card" id="progress-container" hx-get="/scan/{{.ScanID}}/progress" hx-trigger="every 2s" hx-swap="outerHTML">
    <h2>Scanning {{.Bucket}}</h2>

    <div style="margin: 1.5rem 0;">
        <div class="progress-bar">
            <div class="progress-bar-fill" style="width: {{.ProgressPct}}%;"></div>
        </div>
    </div>

    <div style="display: grid; grid-template-columns: 1fr 1fr 1fr; gap: 1rem; text-align: center;">
        <div>
            <div style="font-size: 1.5rem; font-weight: 700; color: var(--primary);">{{.ObjectsFound}}</div>
            <div style="font-size: 0.875rem; color: var(--text-muted);">Objects Found</div>
        </div>
        <div>
            <div style="font-size: 1.5rem; font-weight: 700;">{{.Elapsed}}</div>
            <div style="font-size: 0.875rem; color: var(--text-muted);">Elapsed</div>
        </div>
        <div>
            <div style="font-size: 1.5rem; font-weight: 700;">{{.ScanType}}</div>
            <div style="font-size: 0.875rem; color: var(--text-muted);">Scan Type</div>
        </div>
    </div>

    <div style="margin-top: 1rem; text-align: center;">
        <span style="color: var(--text-muted); font-size: 0.875rem;">
            {{if .Status}}{{.Status}}{{else}}Running...{{end}}
        </span>
    </div>
</div>

<script>
    // Auto-redirect to report when complete (fallback for HTMX)
    setInterval(function() {
        fetch('/scan/{{.ScanID}}/status')
            .then(r => r.json())
            .then(data => {
                if (data.status === 'completed') {
                    window.location.href = '/scan/' + data.scan_id;
                }
            })
            .catch(() => {});
    }, 3000);
</script>
{{end}}
```

- [ ] **Step 7: Write scan report template**

```html
<!-- internal/web/templates/scan_report.html -->
{{define "content"}}
<h2 style="margin-bottom: 1.5rem;">Scan Report: {{.Result.Record.Bucket}}</h2>

<div style="margin-bottom: 1rem; display:flex; gap:0.75rem; align-items:center;">
    <span class="badge {{if eq .Result.Record.Status "completed"}}badge-green{{else}}badge-yellow{{end}}">
        {{.Result.Record.Status}}
    </span>
    <span style="font-size:0.875rem; color:var(--text-muted);">
        {{.Result.Record.ScanType}} scan &middot; {{.Result.Record.Timestamp}} &middot; {{.Result.Record.Duration}}
    </span>
    <button class="btn-link" style="margin-left:auto;" onclick="window.print()">Download Report</button>
    <form action="/scan/{{.Result.Record.ID}}/delete" method="POST" style="display:inline;"
          onsubmit="return confirm('Delete this scan?')">
        <button type="submit" class="btn-link" style="color:var(--red);">Delete</button>
    </form>
</div>

<div class="card-row">
    <div class="stat-card">
        <div class="stat-value">{{.Result.Summary.TotalObjects}}</div>
        <div class="stat-label">Total Objects</div>
    </div>
    <div class="stat-card">
        <div class="stat-value">{{formatBytes .Result.Summary.TotalSize}}</div>
        <div class="stat-label">Total Size</div>
    </div>
    <div class="stat-card">
        <div class="stat-value">{{formatBytes .Result.Summary.AvgSize}}</div>
        <div class="stat-label">Avg Size</div>
    </div>
    <div class="stat-card">
        <div class="stat-value">{{formatBytes .Result.Summary.MedianSize}}</div>
        <div class="stat-label">Median Size</div>
    </div>
    <div class="stat-card">
        <div class="stat-value">{{formatBytes .Result.Summary.MaxSize}}</div>
        <div class="stat-label">Max Size</div>
    </div>
    <div class="stat-card">
        <div class="stat-value">{{.Result.Summary.EmptyObjects}}</div>
        <div class="stat-label">Empty Objects</div>
    </div>
</div>

{{if .Result.Delta}}
<div class="card">
    <h2>Delta Report</h2>
    <div style="display:grid; grid-template-columns: 1fr 1fr 1fr 1fr; gap:1rem; text-align:center;">
        <div><span style="font-size:1.25rem; font-weight:700; color:var(--green);">+{{.Result.Delta.New}}</span><br><span style="font-size:0.75rem;color:var(--text-muted);">New</span></div>
        <div><span style="font-size:1.25rem; font-weight:700; color:var(--yellow);">{{.Result.Delta.Modified}}</span><br><span style="font-size:0.75rem;color:var(--text-muted);">Modified</span></div>
        <div><span style="font-size:1.25rem; font-weight:700; color:var(--red);">-{{.Result.Delta.Deleted}}</span><br><span style="font-size:0.75rem;color:var(--text-muted);">Deleted</span></div>
        <div><span style="font-size:1.25rem; font-weight:700;">{{.Result.Delta.Unchanged}}</span><br><span style="font-size:0.75rem;color:var(--text-muted);">Unchanged</span></div>
    </div>
</div>
{{end}}

<div class="card">
    <h2>File Types</h2>
    <div class="chart-container">
        <canvas id="typesChart"></canvas>
    </div>
    <table>
        <thead><tr><th>Extension</th><th>Count</th><th>Size</th><th>%</th></tr></thead>
        <tbody>
        {{range .Result.Types}}
        <tr><td>{{.Ext}}</td><td>{{.Count}}</td><td>{{formatBytes .TotalSize}}</td><td>{{.Pct}}%</td></tr>
        {{end}}
        </tbody>
    </table>
</div>

<div class="card">
    <h2>Age Distribution</h2>
    <div class="chart-container">
        <canvas id="agesChart"></canvas>
    </div>
    <table>
        <thead><tr><th>Age</th><th>Count</th><th>Size</th></tr></thead>
        <tbody>
        {{range .Result.Ages}}
        <tr><td>{{.Label}}</td><td>{{.Count}}</td><td>{{formatBytes .Size}}</td></tr>
        {{end}}
        </tbody>
    </table>
</div>

<div class="card">
    <h2>Storage Classes</h2>
    <table>
        <thead><tr><th>Class</th><th>Count</th><th>Size</th></tr></thead>
        <tbody>
        {{range .Result.Storage}}
        <tr><td>{{.Class}}</td><td>{{.Count}}</td><td>{{formatBytes .Size}}</td></tr>
        {{end}}
        </tbody>
    </table>
</div>

<div class="card">
    <h2>Top-Level Prefixes</h2>
    <table>
        <thead><tr><th>Prefix</th><th>Count</th><th>Size</th></tr></thead>
        <tbody>
        {{range .Result.Prefixes}}
        <tr><td>{{.Prefix}}</td><td>{{.Count}}</td><td>{{formatBytes .Size}}</td></tr>
        {{end}}
        </tbody>
    </table>
</div>

<!-- Deep scan sections -->
{{if .Result.Duplicates}}
<div class="accordion-section open">
    <div class="accordion-header" onclick="this.parentElement.classList.toggle('open')">
        <span>Duplicate Detection ({{len .Result.Duplicates}} groups)</span>
        <span>&#9660;</span>
    </div>
    <div class="accordion-body">
        <table>
            <thead><tr><th>ETag</th><th>Count</th><th>Wasted</th><th>Keys</th></tr></thead>
            <tbody>
            {{range .Result.Duplicates}}
            <tr><td>{{.ETag}}</td><td>{{.Count}}</td><td>{{formatBytes .TotalSize}}</td><td>{{join .Keys ", "}}</td></tr>
            {{end}}
            </tbody>
        </table>
    </div>
</div>
{{end}}

{{if .Result.Multiparts}}
<div class="accordion-section open">
    <div class="accordion-header" onclick="this.parentElement.classList.toggle('open')">
        <span>Incomplete Multipart Uploads ({{len .Result.Multiparts}})</span>
        <span>&#9660;</span>
    </div>
    <div class="accordion-body">
        <table>
            <thead><tr><th>Key</th><th>Upload ID</th><th>Initiated</th></tr></thead>
            <tbody>
            {{range .Result.Multiparts}}
            <tr><td>{{.Key}}</td><td>{{.UploadID}}</td><td>{{.Initiated}}</td></tr>
            {{end}}
            </tbody>
        </table>
    </div>
</div>
{{end}}

{{if .Result.AccessAudit}}
<div class="accordion-section open">
    <div class="accordion-header" onclick="this.parentElement.classList.toggle('open')">
        <span>Access Audit{{if .Result.AccessAudit.Findings}} <span class="badge badge-red">{{len .Result.AccessAudit.Findings}} findings{{end}}</span>
        <span>&#9660;</span>
    </div>
    <div class="accordion-body">
        {{if .Result.AccessAudit.PublicAccessBlocked}}
        <p style="color:var(--green);">&#10003; Public access block is configured</p>
        {{else}}
        <p style="color:var(--red);">&#10007; Public access block not configured</p>
        {{end}}
        {{range .Result.AccessAudit.Findings}}
        <div style="margin-top:0.75rem; padding:0.75rem; background:var(--surface-2); border-radius:var(--radius);">
            <span class="badge {{if eq .Severity "HIGH"}}badge-red{{else}}badge-yellow{{end}}">{{.Severity}}</span>
            <strong>{{.Message}}</strong>
            <p style="font-size:0.875rem; color:var(--text-muted); margin-top:0.25rem;">{{.Detail}}</p>
        </div>
        {{end}}
    </div>
</div>
{{end}}

{{if .Result.Encryption}}
<div class="accordion-section open">
    <div class="accordion-header" onclick="this.parentElement.classList.toggle('open')">
        <span>Encryption Audit &mdash; {{printf "%.1f" .Result.Encryption.EncryptedPct}}% encrypted</span>
        <span>&#9660;</span>
    </div>
    <div class="accordion-body">
        <p>Algorithms: {{join .Result.Encryption.Algorithms ", "}}</p>
        {{if .Result.Encryption.UnencryptedKeys}}
        <p style="color:var(--red); margin-top:0.75rem;">{{len .Result.Encryption.UnencryptedKeys}} unencrypted object samples</p>
        <ul style="font-size:0.875rem; margin-top:0.5rem;">
            {{range .Result.Encryption.UnencryptedKeys}}
            <li>{{.}}</li>
            {{end}}
        </ul>
        {{end}}
    </div>
</div>
{{end}}

{{if .Result.Versioning}}
<div class="accordion-section open">
    <div class="accordion-header" onclick="this.parentElement.classList.toggle('open')">
        <span>Versioning Waste</span>
        <span>&#9660;</span>
    </div>
    <div class="accordion-body">
        <div style="display:grid; grid-template-columns:1fr 1fr 1fr; gap:1rem;">
            <div><strong>{{.Result.Versioning.TotalVersions}}</strong><br><span style="font-size:0.875rem;color:var(--text-muted);">Total Versions</span></div>
            <div><strong>{{.Result.Versioning.NonCurrentCount}}</strong><br><span style="font-size:0.875rem;color:var(--text-muted);">Non-Current</span></div>
            <div><strong>{{formatBytes .Result.Versioning.WastedBytes}}</strong><br><span style="font-size:0.875rem;color:var(--text-muted);">Wasted Space</span></div>
        </div>
    </div>
</div>
{{end}}

{{if .Result.LargeFiles}}
<div class="accordion-section open">
    <div class="accordion-header" onclick="this.parentElement.classList.toggle('open')">
        <span>Large Files ({{len .Result.LargeFiles}})</span>
        <span>&#9660;</span>
    </div>
    <div class="accordion-body">
        <table>
            <thead><tr><th>Key</th><th>Size</th><th>Last Modified</th></tr></thead>
            <tbody>
            {{range .Result.LargeFiles}}
            <tr><td>{{.Key}}</td><td>{{formatBytes .Size}}</td><td>{{.LastModified}}</td></tr>
            {{end}}
            </tbody>
        </table>
    </div>
</div>
{{end}}

{{if .Result.Naming}}
<div class="accordion-section open">
    <div class="accordion-header" onclick="this.parentElement.classList.toggle('open')">
        <span>Naming Convention &mdash; {{.Result.Naming.NonCompliantCount}} non-compliant</span>
        <span>&#9660;</span>
    </div>
    <div class="accordion-body">
        <p>Pattern: <code>{{.Result.Naming.Pattern}}</code></p>
        {{if .Result.Naming.Examples}}
        <ul style="font-size:0.875rem; margin-top:0.5rem; color:var(--red);">
            {{range .Result.Naming.Examples}}<li>{{.}}</li>{{end}}
        </ul>
        {{end}}
    </div>
</div>
{{end}}

{{if .Result.CostEstimate}}
<div class="accordion-section open">
    <div class="accordion-header" onclick="this.parentElement.classList.toggle('open')">
        <span>Cost Estimate &mdash; ${{printf "%.2f" .Result.CostEstimate.MonthlyTotal}}/month</span>
        <span>&#9660;</span>
    </div>
    <div class="accordion-body">
        <table>
            <thead><tr><th>Storage Class</th><th>Monthly Cost</th></tr></thead>
            <tbody>
            {{range .Result.CostEstimate.Breakdown}}
            <tr><td>{{.Class}}</td><td>${{printf "%.4f" .MonthlyCost}}</td></tr>
            {{end}}
            <tr style="font-weight:700;"><td>Total</td><td>${{printf "%.2f" .Result.CostEstimate.MonthlyTotal}}</td></tr>
            </tbody>
        </table>
    </div>
</div>
{{end}}

{{if .Result.Virus}}
<div class="accordion-section open">
    <div class="accordion-header" onclick="this.parentElement.classList.toggle('open')">
        <span>Virus Scan &mdash; {{.Result.Virus.Scanned}} scanned{{if .Result.Virus.Infected}}, {{len .Result.Virus.Infected}} infected{{end}}</span>
        <span>&#9660;</span>
    </div>
    <div class="accordion-body">
        {{if .Result.Virus.Infected}}
        <div style="color:var(--red);">
            <strong>Infected files:</strong>
            <ul>{{range .Result.Virus.Infected}}<li>{{.}}</li>{{end}}</ul>
        </div>
        {{end}}
        {{if .Result.Virus.Errors}}
        <div style="color:var(--yellow); margin-top:0.75rem;">
            <strong>Errors:</strong>
            <ul>{{range .Result.Virus.Errors}}<li>{{.}}</li>{{end}}</ul>
        </div>
        {{end}}
    </div>
</div>
{{end}}

<script>
{{range $i, $t := .Result.Types}}
typesLabels.push("{{$t.Ext}}");
typesData.push({{$t.Count}});
{{end}}
{{range $i, $a := .Result.Ages}}
agesLabels.push("{{$a.Label}}");
agesCounts.push({{$a.Count}});
{{end}}
</script>
{{end}}
```

- [ ] **Step 8: Write history template**

```html
<!-- internal/web/templates/history.html -->
{{define "content"}}
<h2 style="margin-bottom: 1.5rem;">Scan History</h2>

{{if .Buckets}}
<div class="form-group" style="max-width: 300px;">
    <label for="bucket-filter">Filter by Bucket</label>
    <select id="bucket-filter" onchange="window.location.href='?bucket='+this.value">
        <option value="">All buckets</option>
        {{range .Buckets}}
        <option value="{{.}}" {{if eq $.SelectedBucket .}}selected{{end}}>{{.}}</option>
        {{end}}
    </select>
</div>
{{end}}

{{if .Scans}}
<form action="/compare" method="GET" id="compare-form">
    <table>
        <thead>
            <tr>
                <th style="width:40px;">Compare</th>
                <th>Date</th>
                <th>Bucket</th>
                <th>Type</th>
                <th>Status</th>
                <th>Duration</th>
                <th>Objects</th>
                <th>Actions</th>
            </tr>
        </thead>
        <tbody>
        {{range .Scans}}
            <tr>
                <td><input type="checkbox" name="scans" value="{{.ID}}" onchange="updateCompareBtn()"></td>
                <td>{{.Timestamp}}</td>
                <td>{{.Bucket}}</td>
                <td><span class="badge {{if eq .ScanType "full"}}badge-yellow{{else}}badge-green{{end}}">{{.ScanType}}</span></td>
                <td><span class="badge {{if eq .Status "completed"}}badge-green{{else}}badge-red{{end}}">{{.Status}}</span></td>
                <td>{{.Duration}}</td>
                <td>{{.TotalObjects}}</td>
                <td>
                    <a href="/scan/{{.ID}}" class="btn-link">View</a>
                    <form action="/scan/{{.ID}}/delete" method="POST" style="display:inline;"
                          onsubmit="return confirm('Delete this scan?')">
                        <button type="submit" class="btn-link" style="color:var(--red);">Delete</button>
                    </form>
                </td>
            </tr>
        {{end}}
        </tbody>
    </table>
</form>

<div style="margin-top:1rem;">
    <button id="compare-btn" class="btn btn-secondary" disabled onclick="document.getElementById('compare-form').submit()">
        Compare Selected
    </button>
    <span style="font-size:0.875rem; color:var(--text-muted); margin-left:0.75rem;">Select exactly 2 scans to compare</span>
</div>

<script>
function updateCompareBtn() {
    var checked = document.querySelectorAll('input[name="scans"]:checked');
    document.getElementById('compare-btn').disabled = checked.length !== 2;
}
</script>
{{else}}
<div class="empty-state">
    <h3>No scans yet</h3>
    <p>Run a scan from the dashboard to see history here.</p>
    <a href="/" class="btn" style="margin-top:1rem;">Go to Dashboard</a>
</div>
{{end}}
{{end}}
```

- [ ] **Step 9: Write comparison template**

```html
<!-- internal/web/templates/comparison.html -->
{{define "content"}}
<h2 style="margin-bottom: 1.5rem;">Scan Comparison</h2>

<div class="compare-grid">
    <div>
        <h3 style="margin-bottom:0.75rem;">Scan A: {{.ScanA.Record.Timestamp}}</h3>
        <div class="stat-card">
            <div class="stat-value">{{.ScanA.Summary.TotalObjects}}</div>
            <div class="stat-label">Objects</div>
        </div>
        <div class="stat-card">
            <div class="stat-value">{{formatBytes .ScanA.Summary.TotalSize}}</div>
            <div class="stat-label">Size</div>
        </div>
    </div>
    <div>
        <h3 style="margin-bottom:0.75rem;">Scan B: {{.ScanB.Record.Timestamp}}</h3>
        <div class="stat-card">
            <div class="stat-value">{{.ScanB.Summary.TotalObjects}}</div>
            <div class="stat-label">Objects</div>
        </div>
        <div class="stat-card">
            <div class="stat-value">{{formatBytes .ScanB.Summary.TotalSize}}</div>
            <div class="stat-label">Size</div>
        </div>
    </div>
</div>

<div style="margin-top:1.5rem;">
    <div class="card">
        <h2>Changes</h2>
        <div style="display:grid; grid-template-columns:1fr 1fr; gap:1rem; text-align:center;">
            <div>
                <div style="font-size:1.5rem; font-weight:700; {{if ge .ObjectDelta 0}}color:var(--green){{else}}color:var(--red){{end}}">
                    {{if ge .ObjectDelta 0}}+{{end}}{{.ObjectDelta}}
                </div>
                <div style="font-size:0.875rem; color:var(--text-muted);">Object Count Change</div>
            </div>
            <div>
                <div style="font-size:1.5rem; font-weight:700; {{if ge .SizeDelta 0}}color:var(--green){{else}}color:var(--red){{end}}">
                    {{formatBytes .SizeDelta}}
                </div>
                <div style="font-size:0.875rem; color:var(--text-muted);">Size Change</div>
            </div>
        </div>
    </div>
</div>

{{if .TrendData}}
<div class="card">
    <h2>Trends (3+ scans required for time series)</h2>
    <div class="chart-container">
        <canvas id="trendChart"></canvas>
    </div>
</div>
{{end}}

<div style="margin-top:1.5rem;">
    <div class="card">
        <h2>File Type Comparison</h2>
        <div class="compare-grid">
            <div>
                <h4>Scan A</h4>
                <table>
                    <thead><tr><th>Ext</th><th>Count</th></tr></thead>
                    <tbody>{{range .ScanA.Types}}<tr><td>{{.Ext}}</td><td>{{.Count}}</td></tr>{{end}}</tbody>
                </table>
            </div>
            <div>
                <h4>Scan B</h4>
                <table>
                    <thead><tr><th>Ext</th><th>Count</th></tr></thead>
                    <tbody>{{range .ScanB.Types}}<tr><td>{{.Ext}}</td><td>{{.Count}}</td></tr>{{end}}</tbody>
                </table>
            </div>
        </div>
    </div>
</div>
{{end}}
```

- [ ] **Step 10: Write settings template**

```html
<!-- internal/web/templates/settings.html -->
{{define "content"}}
<h2 style="margin-bottom: 1.5rem;">Settings</h2>

<form action="/settings" method="POST">
    <div class="card settings-section">
        <h2>ClamAV</h2>
        <div class="form-group">
            <label for="clamd_socket">ClamAV Socket Path</label>
            <input type="text" id="clamd_socket" name="clamd_socket" value="{{.Settings.ClamdSocket}}" placeholder="/var/run/clamav/clamd.sock">
            <span class="form-help">Unix socket path or tcp://host:3310</span>
        </div>
    </div>

    <div class="card settings-section">
        <h2>Deep Scan Defaults</h2>
        <div class="checkbox-group">
            <label><input type="checkbox" name="deep_duplicates" {{if .Settings.DeepDuplicates}}checked{{end}}> Duplicates</label>
            <label><input type="checkbox" name="deep_multipart" {{if .Settings.DeepMultipart}}checked{{end}}> Multipart</label>
            <label><input type="checkbox" name="deep_access" {{if .Settings.DeepAccess}}checked{{end}}> Access Audit</label>
            <label><input type="checkbox" name="deep_encryption" {{if .Settings.DeepEncryption}}checked{{end}}> Encryption</label>
            <label><input type="checkbox" name="deep_versioning" {{if .Settings.DeepVersioning}}checked{{end}}> Versioning</label>
            <label><input type="checkbox" name="deep_large_files" {{if .Settings.DeepLargeFiles}}checked{{end}}> Large Files</label>
            <label><input type="checkbox" name="deep_naming" {{if .Settings.DeepNaming}}checked{{end}}> Naming</label>
            <label><input type="checkbox" name="deep_cost" {{if .Settings.DeepCost}}checked{{end}}> Cost Estimate</label>
        </div>
    </div>

    <div class="card settings-section">
        <h2>Naming Convention</h2>
        <div class="form-group">
            <label for="naming_pattern">Regex Pattern</label>
            <input type="text" id="naming_pattern" name="naming_pattern" value="{{.Settings.NamingPattern}}" placeholder="^[a-z0-9-/]+$">
            <span class="form-help">Leave empty to skip naming check</span>
        </div>
    </div>

    <div class="card settings-section">
        <h2>Large File Threshold</h2>
        <div class="form-group">
            <label for="large_file_threshold">Threshold (MB)</label>
            <input type="number" id="large_file_threshold" name="large_file_threshold" value="{{.Settings.LargeFileThresholdMB}}" min="1">
        </div>
    </div>

    <div class="card settings-section">
        <h2>Cost Estimation ($/GB/month)</h2>
        {{range $class, $cost := .Settings.CostRates}}
        <div class="form-group">
            <label for="cost_{{$class}}">{{$class}}</label>
            <input type="number" step="0.001" id="cost_{{$class}}" name="cost_{{$class}}" value="{{$cost}}">
        </div>
        {{end}}
    </div>

    <button type="submit" class="btn">Save Settings</button>
</form>
{{end}}
```

- [ ] **Step 11: Verify compilation**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go vet ./internal/web/
```

Expected: no errors (may get "no Go files" — that's fine, embed.go has a build tag issue because templates are HTML; we need to ensure the package compiles).

- [ ] **Step 12: Commit**

```bash
git add internal/web/ && git commit -m "feat: add embedded templates, CSS, and static assets"
```

---

### Task 2: Template renderer with helper functions

**Files:**
- Create: `internal/web/render.go`

- [ ] **Step 1: Write template renderer with formatBytes helper**

```go
package web

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/esignoretti/s3lytics/internal/store"
)

// PageData is the common data passed to all templates.
type PageData struct {
	LoggedIn     bool
	Page         string
	AccountEmail string
	Error        string

	// Dashboard
	Projects []store.Project

	// Scan report
	Result *store.ScanResult

	// Scan progress
	ScanID       string
	Bucket       string
	ProgressPct  float64
	ObjectsFound int64
	Elapsed      string
	ScanType     string
	Status       string

	// History
	Scans          []ScanListItem
	Buckets        []string
	SelectedBucket string

	// Comparison
	ScanA       *store.ScanResult
	ScanB       *store.ScanResult
	ObjectDelta int64
	SizeDelta   int64
	TrendData   interface{}

	// Settings
	Settings *SettingsData
}

// ScanListItem is a scan record with total objects for display.
type ScanListItem struct {
	store.ScanRecord
	TotalObjects int64
}

// SettingsData holds application settings.
type SettingsData struct {
	ClamdSocket         string
	DeepDuplicates      bool
	DeepMultipart       bool
	DeepAccess          bool
	DeepEncryption      bool
	DeepVersioning      bool
	DeepLargeFiles      bool
	DeepNaming          bool
	DeepCost            bool
	NamingPattern       string
	LargeFileThresholdMB int64
	CostRates           map[string]float64
}

// TemplateRenderer renders Go templates from the embedded filesystem.
type TemplateRenderer struct {
	templates *template.Template
}

// NewTemplateRenderer parses all templates from the embedded filesystem.
func NewTemplateRenderer() (*TemplateRenderer, error) {
	funcMap := template.FuncMap{
		"formatBytes": formatBytes,
		"join":        strings.Join,
	}

	// Parse all HTML files from the embedded templates directory
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

	tmpl := template.New("").Funcs(funcMap)
	for _, name := range allFiles {
		content, err := Assets.ReadFile(name)
		if err != nil {
			return nil, fmt.Errorf("read template %s: %w", name, err)
		}
		// Parse as sub-template
		_, err = tmpl.Parse(string(content))
		if err != nil {
			return nil, fmt.Errorf("parse template %s: %w", name, err)
		}
	}

	return &TemplateRenderer{templates: tmpl}, nil
}

// Render executes the named template with the given data.
func (r *TemplateRenderer) Render(w io.Writer, name string, data *PageData) error {
	return r.templates.ExecuteTemplate(w, name, data)
}

// RenderString executes a template and returns the result as a string.
func (r *TemplateRenderer) RenderString(name string, data *PageData) (string, error) {
	var buf bytes.Buffer
	if err := r.Render(&buf, name, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func formatBytes(b int64) string {
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
```

- [ ] **Step 2: Verify compilation**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go vet ./internal/web/
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/web/render.go && git commit -m "feat: add template renderer with formatBytes helper"
```

---

### Task 3: HTTP handlers — static, auth, dashboard

**Files:**
- Create: `internal/web/handlers/handlers.go`

- [ ] **Step 1: Write all HTTP handlers**

```go
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
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

// Handler holds dependencies for all HTTP handlers.
type Handler struct {
	Store           store.Store
	AuthService     *auth.Service
	SessionManager  *auth.SessionManager
	ScanEngine      *scan.Engine
	S3Client        s3.S3Client
	TemplateRenderer *web.TemplateRenderer
	DeepConfig      deep.Config
}

// RegisterRoutes mounts all routes on the chi router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	// Static files — use fs.Sub to serve files from the embedded "static" directory
	staticFS, _ := fs.Sub(web.Assets, "static")
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	// Auth routes
	r.Get("/login", h.GetLogin)
	r.Post("/login", h.PostLogin)
	r.Post("/logout", h.PostLogout)

	// Dashboard
	r.Get("/", h.GetDashboard)
	r.Get("/buckets", h.GetBucketsJSON)

	// Scan
	r.Post("/scan/start", h.PostStartScan)
	r.Get("/scan/{id}", h.GetScanReport)
	r.Get("/scan/{id}/progress", h.GetScanProgress)
	r.Get("/scan/{id}/status", h.GetScanStatus)
	r.Post("/scan/{id}/delete", h.PostDeleteScan)

	// History
	r.Get("/history", h.GetHistory)

	// Comparison
	r.Get("/compare", h.GetComparison)

	// Settings
	r.Get("/settings", h.GetSettings)
	r.Post("/settings", h.PostSettings)
}

// --- Auth ---

func (h *Handler) GetLogin(w http.ResponseWriter, r *http.Request) {
	renderLogin(w, h.TemplateRenderer, "")
}

func (h *Handler) PostLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		renderLogin(w, h.TemplateRenderer, "Invalid form")
		return
	}

	loginReq := &auth.LoginRequest{
		Email:    r.FormValue("email"),
		Password: r.FormValue("password"),
		TfaCode:  r.FormValue("tfa_code"),
		TenantID: r.FormValue("tenant_id"),
		APIURL:   r.FormValue("api_url"),
	}

	// Step 1-3: Challenge-response signin
	signinResp, err := h.AuthService.Login(loginReq)
	if err != nil {
		log.Printf("login failed: %v", err)
		renderLogin(w, h.TemplateRenderer, fmt.Sprintf("Login failed: %v", err))
		return
	}

	// Step 4: Get account
	account, err := h.AuthService.GetAccount(signinResp.JWT)
	if err != nil {
		log.Printf("get account failed: %v", err)
		renderLogin(w, h.TemplateRenderer, "Failed to retrieve account")
		return
	}

	// Save session and account
	ctx := context.Background()
	if err := h.SessionManager.SaveLogin(ctx, signinResp, account); err != nil {
		log.Printf("save login failed: %v", err)
		renderLogin(w, h.TemplateRenderer, "Failed to save session")
		return
	}

	// Steps 5-7: Get projects and API keys
	projects, err := h.AuthService.GetProjects(signinResp.JWT)
	if err == nil {
		storeProjects := make([]store.Project, len(projects))
		for i, p := range projects {
			storeProjects[i] = store.Project{ID: p.ID, Name: p.Name}
		}
		h.Store.SaveProjects(ctx, storeProjects)

		// Forge JWT and get API key for first project
		if len(projects) > 0 && account.EndpointGateway != "" {
			forgeResp, err := h.AuthService.ForgeJWT(account.ID)
			if err == nil {
				keyResp, err := h.AuthService.CreateApiKey("s3lytics", account.ID, forgeResp.JWT)
				if err == nil && keyResp != nil {
					log.Printf("S3 API key obtained for bucket access")

					// Create S3 client with the obtained credentials
					s3Client, err := s3.NewCubbitS3Client(
						account.EndpointGateway,
						"us-east-1", // default region
						keyResp.ApiKey,
						keyResp.SecretKey,
					)
					if err == nil {
						h.S3Client = s3Client
						h.ScanEngine.SetS3Client(s3Client)
					}
				}
			}
		}
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handler) PostLogout(w http.ResponseWriter, r *http.Request) {
	h.SessionManager.Logout(context.Background())
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// --- Dashboard ---

func (h *Handler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	projects, _ := h.Store.GetProjects(ctx)

	data := &web.PageData{
		LoggedIn: true,
		Page:     "dashboard",
		Projects: projects,
	}
	_ = h.TemplateRenderer.Render(w, "layout.html", data)
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

// --- Scan ---

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
		Page:         "scan",
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
		data.ProgressPct = 50.0 // estimated
	}

	_ = h.TemplateRenderer.Render(w, "layout.html", data)
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

	data := &web.PageData{
		LoggedIn: true,
		Page:     "scan",
		Result:   result,
	}

	_ = h.TemplateRenderer.Render(w, "layout.html", data)
}

func (h *Handler) PostDeleteScan(w http.ResponseWriter, r *http.Request) {
	scanID := chi.URLParam(r, "id")
	if err := h.Store.DeleteScan(context.Background(), scanID); err != nil {
		log.Printf("delete scan %s: %v", scanID, err)
	}
	http.Redirect(w, r, "/history", http.StatusSeeOther)
}

// --- History ---

func (h *Handler) GetHistory(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	bucket := r.URL.Query().Get("bucket")

	records, err := h.Store.ListScans(ctx, bucket)
	if err != nil {
		records = []store.ScanRecord{}
	}

	// Collect unique bucket names
	bucketSet := make(map[string]bool)
	var items []web.ScanListItem
	for _, rec := range records {
		bucketSet[rec.Bucket] = true
		items = append(items, web.ScanListItem{
			ScanRecord: rec,
		})
	}

	// Enrich with total objects from ScanResult
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

	data := &web.PageData{
		LoggedIn:       true,
		Page:           "history",
		Scans:          items,
		Buckets:        buckets,
		SelectedBucket: bucket,
	}

	_ = h.TemplateRenderer.Render(w, "layout.html", data)
}

// --- Comparison ---

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

	data := &web.PageData{
		LoggedIn:    true,
		Page:        "comparison",
		ScanA:       resultA,
		ScanB:       resultB,
		ObjectDelta: objDelta,
		SizeDelta:   sizeDelta,
	}

	_ = h.TemplateRenderer.Render(w, "layout.html", data)
}

// --- Settings ---

func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	settings := h.loadSettings()
	data := &web.PageData{
		LoggedIn: true,
		Page:     "settings",
		Settings: settings,
	}
	_ = h.TemplateRenderer.Render(w, "layout.html", data)
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

	// Parse cost rates
	for class := range settings.CostRates {
		if val := r.FormValue("cost_" + class); val != "" {
			if rate, err := strconv.ParseFloat(val, 64); err == nil {
				settings.CostRates[class] = rate
			}
		}
	}

	// Save settings to store with a dedicated key
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

// --- Helpers ---

func renderLogin(w http.ResponseWriter, r *web.TemplateRenderer, errMsg string) {
	data := &web.PageData{
		Error: errMsg,
	}
	_ = r.Render(w, "layout.html", data)
}
```

- [ ] **Step 2: Verify compilation**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go vet ./internal/web/handlers/
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/web/handlers/handlers.go && git commit -m "feat: add all HTTP handlers (auth, dashboard, scan, history, comparison, settings)"
```

---

**End of Phase 7. Phase 7 deliverables:**
- [x] 8 HTML templates (layout, login, dashboard, scan report, scan progress, history, comparison, settings)
- [x] CSS stylesheet with dark theme
- [x] `embed.go` for single-binary deployment
- [x] Template renderer with `formatBytes` helper
- [x] All HTTP handlers (auth flow, dashboard, scan start/progress/report, history, comparison, settings)
- [x] HTMX integration for dynamic updates
- [x] Chart.js for visualizations

**Ready for Phase 8: Wire everything together in main.go and integration test.**
