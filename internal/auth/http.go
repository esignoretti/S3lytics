package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"
)

const defaultCoordinatorURL = "https://api.eu00wi.cubbit.services"

type httpClient struct {
	mu           sync.Mutex
	baseURL      string
	client       *http.Client
	refreshToken string
}

func newHTTPClient() *httpClient {
	jar, _ := cookiejar.New(nil)
	return &httpClient{
		baseURL: defaultCoordinatorURL,
		client: &http.Client{
			Timeout: 30 * time.Second,
			Jar:     jar,
		},
	}
}

func (c *httpClient) SetBaseURL(u string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.baseURL = strings.TrimRight(u, "/")
}

func (c *httpClient) BaseURL() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.baseURL
}

// GetRefreshCookie returns the last observed _refresh cookie value.
func (c *httpClient) GetRefreshCookie() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.refreshToken
}

// RestoreRefreshCookie seeds the jar (and internal tracker) with a previously
// persisted _refresh token, so requests after app restart still authenticate.
func (c *httpClient) RestoreRefreshCookie(value string) {
	if value == "" {
		return
	}
	c.mu.Lock()
	c.refreshToken = value
	base := c.baseURL
	jar := c.client.Jar
	c.mu.Unlock()

	u, err := url.Parse(base)
	if err != nil || jar == nil {
		return
	}
	jar.SetCookies(u, []*http.Cookie{{
		Name:  "_refresh",
		Value: value,
		Path:  "/",
	}})
}

func (c *httpClient) doRequest(ctx context.Context, method, path string, body interface{}, authToken string) ([]byte, error) {
	c.mu.Lock()
	baseURL := c.baseURL
	client := c.client
	c.mu.Unlock()

	if baseURL == "" {
		return nil, fmt.Errorf("base URL not set")
	}

	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if authToken != "" {
		req.Header.Set("Authorization", "Bearer "+authToken)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	for _, ck := range resp.Cookies() {
		if ck.Name == "_refresh" && ck.Value != "" {
			c.mu.Lock()
			c.refreshToken = ck.Value
			c.mu.Unlock()
		}
	}

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		preview := string(responseBody)
		if len(preview) > 300 {
			preview = preview[:300] + "..."
		}
		return nil, fmt.Errorf("HTTP %d %s: %s", resp.StatusCode, path, preview)
	}

	return responseBody, nil
}
