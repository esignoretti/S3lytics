package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"strings"
	"time"
)

var coordinatorURL string

func main() {
	email := flag.String("email", "", "Cubbit IAM email")
	password := flag.String("password", "", "Cubbit IAM password")
	apiURL := flag.String("api-url", "https://api.eu00wi.cubbit.services", "IAM API base URL")
	flag.Parse()

	if *email == "" || *password == "" {
		fmt.Println("Usage: debug --email=user@example.com --password=...")
		os.Exit(1)
	}

	coordinatorURL = *apiURL
	ctx := context.Background()

	// Use Go's real cookie jar
	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Timeout: 30 * time.Second,
		Jar:     jar,
	}

	baseURL := strings.TrimRight(coordinatorURL, "/")

	do := func(method, path string, body interface{}, authToken string) (int, map[string]string, []byte) {
		var reqBody io.Reader
		if body != nil {
			data, _ := json.Marshal(body)
			reqBody = bytes.NewReader(data)
		}
		req, _ := http.NewRequestWithContext(ctx, method, baseURL+path, reqBody)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if authToken != "" {
			req.Header.Set("Authorization", "Bearer "+authToken)
		}
		// Print request
		fmt.Printf(">>> %s %s\n", method, path)
		for k, v := range req.Header {
			for _, vv := range v {
				val := vv
				if len(val) > 60 {
					val = val[:60] + "..."
				}
				fmt.Printf("    %s: %s\n", k, val)
			}
		}
		// Also print cookies from jar
		cookies := jar.Cookies(req.URL)
		if len(cookies) > 0 {
			for _, c := range cookies {
				v := c.Value
				if len(v) > 30 {
					v = v[:30] + "..."
				}
				fmt.Printf("    (jar cookie) %s=%s\n", c.Name, v)
			}
		}

		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("<<< ERROR: %v\n", err)
			return 0, nil, nil
		}
		defer resp.Body.Close()

		headers := make(map[string]string)
		for k, v := range resp.Header {
			headers[k] = strings.Join(v, ", ")
		}
		bodyBytes, _ := io.ReadAll(resp.Body)

		fmt.Printf("<<< %d\n", resp.StatusCode)
		for _, c := range resp.Cookies() {
			v := c.Value
			if len(v) > 30 {
				v = v[:30] + "..."
			}
			fmt.Printf("    Set-Cookie: %s=%s (domain=%s, path=%s)\n", c.Name, v, c.Domain, c.Path)
		}

		return resp.StatusCode, headers, bodyBytes
	}

	// Read cookies helper
	getJarCookie := func(name string) string {
		u, _ := http.NewRequest("GET", baseURL+"/", nil)
		for _, c := range jar.Cookies(u.URL) {
			if c.Name == name {
				return c.Value
			}
		}
		return ""
	}

	// === STEP 1: Challenge ===
	fmt.Println("\n=== STEP 1: Challenge ===")
	status, _, body := do("POST", "/iam/v1/auth/signin/challenge", map[string]string{"email": *email}, "")
	must(status == 200, body)

	var challengeResp struct {
		Salt      string `json:"salt"`
		Challenge string `json:"challenge"`
	}
	json.Unmarshal(body, &challengeResp)
	fmt.Printf("Salt: %q\n", challengeResp.Salt)
	fmt.Printf("Challenge: %s...\n", safePrefix(challengeResp.Challenge, 30))

	// === STEP 2: Key Derivation ===
	fmt.Println("\n=== STEP 2: Key Derivation ===")
	saltBytes := []byte(challengeResp.Salt)
	seed := sha256.Sum256(append([]byte(*password), saltBytes...))
	privateKey := ed25519.NewKeyFromSeed(seed[:])
	signature := ed25519.Sign(privateKey, []byte(challengeResp.Challenge))
	signedChallenge := base64.StdEncoding.EncodeToString(signature)
	fmt.Printf("Signature: %s...\n", safePrefix(signedChallenge, 40))

	// === STEP 3: Signin ===
	fmt.Println("\n=== STEP 3: Signin ===")
	signinBody := map[string]string{
		"email":            *email,
		"signed_challenge": signedChallenge,
	}
	status, _, body = do("POST", "/iam/v1/auth/signin", signinBody, "")
	must(status == 200, body)

	var signinResp struct {
		Token string `json:"token"`
	}
	json.Unmarshal(body, &signinResp)
	fmt.Printf("Token: %s...\n", safePrefix(signinResp.Token, 30))
	refreshCookie := getJarCookie("_refresh")
	fmt.Printf("_refresh from jar: %s...\n", safePrefix(refreshCookie, 30))

	// === STEP 4: Get Account ===
	fmt.Println("\n=== STEP 4: Get Account ===")
	status, _, body = do("GET", "/iam/v1/accounts/me", nil, signinResp.Token)
	must(status == 200, body)

	var account struct {
		ID              string `json:"id"`
		EndpointGateway string `json:"endpoint_gateway"`
	}
	json.Unmarshal(body, &account)
	fmt.Printf("Account ID: %s\n", account.ID)
	fmt.Printf("Endpoint: %s\n", account.EndpointGateway)

	// === STEP 5: Get Projects ===
	fmt.Println("\n=== STEP 5: Get Projects ===")
	status, _, body = do("GET", "/composer-hub/v1/projects", nil, signinResp.Token)
	must(status == 200, body)

	var projects []struct {
		ID    string `json:"project_id"`
		Name  string `json:"project_name"`
		Users []struct {
			UserID string `json:"user_id"`
			Name   string `json:"user_name"`
		} `json:"users"`
	}
	json.Unmarshal(body, &projects)
	fmt.Printf("Projects (%d):\n", len(projects))
	for _, p := range projects {
		fmt.Printf("  - %s (%s)\n", p.Name, p.ID)
		for _, u := range p.Users {
			fmt.Printf("      user: %s (%s)\n", u.Name, u.UserID)
		}
	}

	// === STEP 6: Token Refresh ===
	fmt.Println("\n=== STEP 6: Token Refresh ===")
	status, _, body = do("GET", "/iam/v1/auth/refresh/access", nil, "")
	must(status == 200, body)

	var refreshResp struct {
		Token string `json:"token"`
	}
	json.Unmarshal(body, &refreshResp)
	fmt.Printf("Refreshed token: %s...\n", safePrefix(refreshResp.Token, 30))
	newRefresh := getJarCookie("_refresh")
	fmt.Printf("New _refresh from jar: %s...\n", safePrefix(newRefresh, 30))

	// === STEP 7: Try Forge JWT ===
	fmt.Println("\n=== STEP 7: Forge JWT ===")
	allUsers := []struct{ proj, uid, uname string }{}
	for _, p := range projects {
		for _, u := range p.Users {
			allUsers = append(allUsers, struct{ proj, uid, uname string }{p.Name, u.UserID, u.Name})
		}
	}
	if len(allUsers) == 0 {
		allUsers = append(allUsers, struct{ proj, uid, uname string }{"account", account.ID, "account"})
	}

	for _, u := range allUsers {
		fmt.Printf("\n--- Forge user=%s (%s) ---\n", u.uname, u.uid)
		path := "/iam/v1/auth/forge/access?user_id=" + u.uid

		// Try with cookie jar (has _refresh from signin or refresh)
		req, _ := http.NewRequestWithContext(ctx, "GET", baseURL+path, nil)
		req.Header.Set("Content-Type", "application/json")
		// Manually add Cookie header like DS3 Drive does
		r := getJarCookie("_refresh")
		if r != "" {
			req.Header.Set("Cookie", "_refresh="+r)
		}

		fmt.Printf(">>> GET %s\n", path)
		fmt.Printf("    Cookie: _refresh=%s...\n", safePrefix(r, 30))
		// Print jar cookies too
		for _, c := range jar.Cookies(req.URL) {
			v := c.Value
			if len(v) > 30 { v = v[:30] + "..." }
			fmt.Printf("    (jar) %s=%s\n", c.Name, v)
		}

		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("<<< ERROR: %v\n", err)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		fmt.Printf("<<< %d\n", resp.StatusCode)
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			fmt.Printf("    BODY: %s\n", string(body))
		} else {
			fmt.Printf("    ERROR: %s\n", string(body))
		}
	}

	// === STEP 8: Keyvault with forge token ===
	fmt.Println("\n=== STEP 8: Keyvault ===")

	// Forge first, then try keyvault with forge token
	for _, u := range allUsers {
		path := "/iam/v1/auth/forge/access?user_id=" + u.uid
		req, _ := http.NewRequestWithContext(ctx, "GET", baseURL+path, nil)
		r := getJarCookie("_refresh")
		if r != "" {
			req.Header.Set("Cookie", "_refresh="+r)
		}
		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("  Forge %s: ERROR: %v\n", u.uid, err)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			fmt.Printf("  Forge %s: HTTP %d\n", u.uid, resp.StatusCode)
			continue
		}
		var forgeResp struct {
			Token string `json:"token"`
		}
		json.Unmarshal(body, &forgeResp)
		if forgeResp.Token == "" {
			continue
		}
		fmt.Printf("  Forge %s/%s: got token, trying keyvault...\n", u.proj, u.uname)

		// Try keyvault with forge token
		status, _, kvBody := do("GET", "/keyvault/api/v3/keys?user_id="+u.uid, nil, forgeResp.Token)
		if status == 200 {
			fmt.Printf("  KEYVAULT WORKS! Keys: %s\n", string(kvBody))
		} else {
			fmt.Printf("  Keyvault: HTTP %d: %s\n", status, string(kvBody))
		}

		// Also try creating key with forge token
		status, _, createBody := do("POST", "/keyvault/api/v3/keys/s3lytics-debug?user_id="+u.uid, map[string]string{"name": "s3lytics-debug"}, forgeResp.Token)
		if status >= 200 && status < 300 {
			fmt.Printf("  CREATE KEY WORKS! Response: %s\n", string(createBody))
			var apiKey struct {
				ApiKey    string `json:"api_key"`
				SecretKey string `json:"secret_key"`
			}
			json.Unmarshal(createBody, &apiKey)
			if apiKey.ApiKey != "" {
				fmt.Printf("  AccessKey: %s\n", apiKey.ApiKey)
				fmt.Printf("  SecretKey: %s...\n", safePrefix(apiKey.SecretKey, 20))
			}
		} else {
			fmt.Printf("  Create key: HTTP %d: %s\n", status, string(createBody))
		}
	}

	fmt.Println("\n=== DONE ===")
}

func must(ok bool, body []byte) {
	if !ok {
		fmt.Fprintf(os.Stderr, "FATAL: unexpected status. Body: %s\n", string(body))
		os.Exit(1)
	}
}

func safePrefix(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
