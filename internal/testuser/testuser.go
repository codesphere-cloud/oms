// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package testuser

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"time"

	"k8s.io/client-go/rest"
)

// Default connection settings for the internal auth service.
const (
	DefaultNamespace = "codesphere"
	DefaultService   = "auth-service"
	DefaultPort      = 3010
)

// CreateTestUserOpts selects the internal auth service and optional credentials.
// Empty credentials are generated independently for each invocation.
type CreateTestUserOpts struct {
	Namespace string
	Service   string
	Port      int
	Email     string
	Password  string
}

// TestUserResult contains the reusable credentials returned by the application.
type TestUserResult struct {
	Email             string `json:"email"`
	PlaintextPassword string `json:"password"`
	PlaintextAPIToken string `json:"api_token"`
}

func (o CreateTestUserOpts) defaults() (CreateTestUserOpts, error) {
	if o.Namespace == "" {
		o.Namespace = DefaultNamespace
	}

	if o.Service == "" {
		o.Service = DefaultService
	}

	if o.Port == 0 {
		o.Port = DefaultPort
	}

	if o.Port < 1 || o.Port > 65535 {
		return o, fmt.Errorf("service port must be between 1 and 65535")
	}

	if o.Email == "" {
		o.Email = "test+" + strings.ToLower(rand.Text()) + "@codesphere.com"
	}

	addr, err := mail.ParseAddress(o.Email)
	if err != nil || addr.Address != o.Email {
		return o, fmt.Errorf("a plain email address is required")
	}

	if o.Password == "" {
		o.Password = "Aa1!" + rand.Text()
	}

	return o, nil
}

// CreateTestUser reaches auth-service through an ephemeral loopback-only Kubernetes
// port-forward. Signup and API-token creation use the application's normal rules.
func CreateTestUser(ctx context.Context, cfg *rest.Config, opts CreateTestUserOpts) (*TestUserResult, error) {
	opts, err := opts.defaults()
	if err != nil {
		return nil, err
	}

	address, closeForward, err := forwardAuthService(ctx, cfg, opts)
	if err != nil {
		return nil, err
	}
	defer closeForward()

	return createViaAPI(ctx, address, opts)
}

func createViaAPI(ctx context.Context, address string, opts CreateTestUserOpts) (*TestUserResult, error) {
	client := &http.Client{
		Timeout:       30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	credentials := map[string]string{"email": opts.Email, "password": opts.Password}

	signup := map[string]string{"email": opts.Email, "password": opts.Password, "recaptchaToken": "test"}
	if err := rpc(ctx, client, address, "Auth", "signUp", signup, "", nil); err != nil {
		return nil, fmt.Errorf("signup failed (email signup and reCAPTCHA-free signup must be enabled for this test installation): %w", err)
	}

	var session struct {
		SessionID string `json:"sessionId"`
	}
	if err := rpc(ctx, client, address, "Auth", "signIn", credentials, "", &session); err != nil {
		return nil, fmt.Errorf("sign-in failed for %s (email sign-in must be enabled and email confirmation disabled): %w", opts.Email, err)
	}

	if session.SessionID == "" {
		return nil, fmt.Errorf("sign-in returned no session ID")
	}
	// Close the temporary login session even when token creation fails. Cookies from
	// auth-service target /auth-service, so pass the session explicitly on direct calls.
	defer func() {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, address+"/session/v1/sign-out", nil)
		if err != nil {
			return
		}

		req.AddCookie(&http.Cookie{Name: "sessionId", Value: session.SessionID})

		if resp, err := client.Do(req); err == nil {
			_ = resp.Body.Close()
		}
	}()

	var access struct {
		AccessToken string `json:"accessToken"`
	}
	if err := post(ctx, client, address+"/session/v1/gen-access-token", map[string]string{"id": session.SessionID}, "", false, &access); err != nil {
		return nil, fmt.Errorf("generating access token failed: %w", err)
	}

	if access.AccessToken == "" {
		return nil, fmt.Errorf("auth service returned no access token")
	}

	var token struct {
		Token string `json:"token"`
	}
	if err := rpc(ctx, client, address, "ApiTokenService", "createToken", map[string]string{"name": "oms-test-user"}, access.AccessToken, &token); err != nil {
		return nil, fmt.Errorf("creating API token failed: %w", err)
	}

	if token.Token == "" {
		return nil, fmt.Errorf("auth service returned no API token")
	}

	return &TestUserResult{Email: opts.Email, PlaintextPassword: opts.Password, PlaintextAPIToken: token.Token}, nil
}

func rpc(ctx context.Context, client *http.Client, address, service, method string, data any, token string, result any) error {
	return post(ctx, client, address+"/internal/v1/"+service+"/"+method, map[string]any{"data": data}, token, true, result)
}

func post(ctx context.Context, client *http.Client, address string, data any, token string, envelope bool, result any) error {
	body, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("encoding internal API request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, address, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating internal API request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("internal API request failed: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()
	// Error bodies can contain credentials or internal details; never include them.
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("internal API returned HTTP %d", resp.StatusCode)
	}

	body, err = io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("reading internal API response: %w", err)
	}

	if envelope {
		var reply struct {
			Kind int             `json:"kind"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(body, &reply); err != nil {
			return fmt.Errorf("invalid internal RPC response")
		}

		if reply.Kind != 12 {
			return fmt.Errorf("internal API did not return a successful RPC response")
		}

		body = reply.Data
	}

	if result != nil {
		if err := json.Unmarshal(body, result); err != nil {
			return fmt.Errorf("invalid internal API response data")
		}
	}

	return nil
}

// WriteResultToFile persists reusable credentials with owner-only permissions.
func WriteResultToFile(result *TestUserResult, dir string) (string, error) {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("persisting test-user credentials: %w", err)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("persisting test-user credentials: %w", err)
	}
	filePath := filepath.Join(dir, "test-user.json")

	f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return "", fmt.Errorf("persisting test-user credentials: %w", err)
	}

	defer func() { _ = f.Close() }()

	if err := f.Chmod(0600); err != nil {
		return "", fmt.Errorf("persisting test-user credentials: %w", err)
	}

	if _, err := f.Write(data); err != nil {
		return "", fmt.Errorf("persisting test-user credentials: %w", err)
	}

	if err := f.Close(); err != nil {
		return "", fmt.Errorf("persisting test-user credentials: %w", err)
	}

	return filePath, nil
}
