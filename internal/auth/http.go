package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type httpClient struct {
	mu         sync.Mutex
	baseURL    string
	httpClient *http.Client
	cookies    []*http.Cookie
}

func (c *httpClient) SetBaseURL(url string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.baseURL = strings.TrimRight(url, "/")
	if c.httpClient == nil {
		c.httpClient = &http.Client{
			Timeout: 30 * time.Second,
		}
	}
}

func (c *httpClient) doRequest(ctx context.Context, method, path string, body interface{}, authToken string) ([]byte, error) {
	c.mu.Lock()
	baseURL := c.baseURL
	client := c.httpClient
	cookies := append([]*http.Cookie(nil), c.cookies...)
	c.mu.Unlock()

	url := baseURL + path

	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if authToken != "" {
		req.Header.Set("Authorization", "Bearer "+authToken)
	}

	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyPreview := string(responseBody)
		if len(bodyPreview) > 200 {
			bodyPreview = bodyPreview[:200] + "..."
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, bodyPreview)
	}

	c.mu.Lock()
	c.cookies = nil
	for _, cc := range resp.Cookies() {
		c.cookies = append(c.cookies, cc)
	}
	c.mu.Unlock()

	return responseBody, nil
}
