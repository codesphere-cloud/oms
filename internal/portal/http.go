// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package portal

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/codesphere-cloud/oms/internal/env"
)

type Portal interface {
	ListBuilds(product Product) (availablePackages Builds, err error)
	GetBuild(product Product, version string, hash string) (Build, error)
	DownloadBuildArtifact(product Product, build Build, file io.Writer) error
	RegisterAPIKey(owner string, organization string, role string, expiresAt time.Time) error
	RevokeAPIKey(key string) error
}

type PortalClient struct {
	Env        env.Env
	HttpClient HttpClient
}

type HttpClient interface {
	Do(*http.Request) (*http.Response, error)
}

func NewPortalClient() *PortalClient {
	return &PortalClient{
		Env:        env.NewEnv(),
		HttpClient: http.DefaultClient,
	}
}

type Product string

const (
	CodesphereProduct Product = "codesphere"
	OmsProduct        Product = "oms"
)

func (c *PortalClient) HttpRequest(method string, path string, body []byte) (resp *http.Response, err error) {
	requestBody := bytes.NewBuffer(body)
	url, err := url.JoinPath(c.Env.GetOmsPortalApi(), path)
	if err != nil {
		err = fmt.Errorf("failed to get generate URL: %w", err)
		return
	}
	apiKey, err := c.Env.GetOmsPortalApiKey()
	if err != nil {
		err = fmt.Errorf("failed to get API Key: %w", err)
		return
	}

	req, err := http.NewRequest(method, url, requestBody)
	if err != nil {
		log.Fatalf("Error creating request: %v", err)
		return
	}

	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}

	req.Header.Set("X-API-Key", apiKey)

	resp, err = c.HttpClient.Do(req)
	if err != nil {
		err = fmt.Errorf("failed to send request: %w", err)
		return
	}

	if resp.StatusCode == http.StatusUnauthorized {
		fmt.Println("You need a valid OMS API Key, please reach out to the Codesphere support at support@codesphere.com to request a new API Key.")
		fmt.Println("If you already have an API Key, make sure to set it using the environment variable OMS_PORTAL_API_KEY")
	}
	var respBody []byte
	if resp.StatusCode >= 300 {
		if resp.Body != nil {
			respBody, _ = io.ReadAll(resp.Body)
		}
		err = fmt.Errorf("unexpected response status: %d - %s, %s", resp.StatusCode, http.StatusText(resp.StatusCode), string(respBody))
		return
	}

	return
}

func (c *PortalClient) GetBody(path string) (body []byte, status int, err error) {
	resp, err := c.HttpRequest(http.MethodGet, path, []byte{})
	if err != nil || resp == nil {
		err = fmt.Errorf("GET failed: %w", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	status = resp.StatusCode

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		err = fmt.Errorf("failed to read response body: %w", err)
		return
	}

	return
}

func (c *PortalClient) ListBuilds(product Product) (availablePackages Builds, err error) {
	res, _, err := c.GetBody(fmt.Sprintf("/packages/%s", product))
	if err != nil {
		err = fmt.Errorf("failed to list packages: %w", err)
		return
	}

	err = json.Unmarshal(res, &availablePackages)
	if err != nil {
		err = fmt.Errorf("failed to parse list packages response: %w", err)
		return
	}

	compareBuilds := func(l, r Build) int {
		if l.Date.Before(r.Date) {
			return -1
		}
		if l.Date.Equal(r.Date) && l.Internal == r.Internal {
			return 0
		}
		return 1
	}
	slices.SortFunc(availablePackages.Builds, compareBuilds)

	return
}

func (c *PortalClient) GetBuild(product Product, version string, hash string) (Build, error) {
	packages, err := c.ListBuilds(product)
	if err != nil {
		return Build{}, fmt.Errorf("failed to list %s packages: %w", product, err)
	}

	if len(packages.Builds) == 0 {
		return Build{}, errors.New("no builds returned")
	}

	if version == "" || version == "latest" {
		// Builds are always ordered by date, newest build is latest version
		return packages.Builds[len(packages.Builds)-1], nil
	}

	matchingPackages := []Build{}
	for _, build := range packages.Builds {
		if build.Version == version {
			if len(hash) == 0 || strings.HasPrefix(hash, build.Hash) {
				matchingPackages = append(matchingPackages, build)
			}
		}
	}

	if len(matchingPackages) == 0 {
		return Build{}, fmt.Errorf("version %s not found", version)
	}

	// Builds are always ordered by date, return newest build
	return matchingPackages[len(matchingPackages)-1], nil
}

func (c *PortalClient) DownloadBuildArtifact(product Product, build Build, file io.Writer) error {
	reqBody, err := json.Marshal(build)
	if err != nil {
		return fmt.Errorf("failed to generate request body: %w", err)
	}

	resp, err := c.HttpRequest(http.MethodGet, fmt.Sprintf("/packages/%s/download", product), reqBody)
	if err != nil {
		return fmt.Errorf("GET request to download build failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Create a WriteCounter to wrap the output file and report progress.
	counter := NewWriteCounter(file)

	_, err = io.Copy(counter, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to copy response body to file: %w", err)
	}

	fmt.Println("Download finished successfully.")
	return nil
}

func (c *PortalClient) RegisterAPIKey(owner string, organization string, role string, expiresAt time.Time) error {
	req := struct {
		Owner        string    `json:"owner"`
		Organization string    `json:"organization"`
		Role         string    `json:"role"`
		ExpiresAt    time.Time `json:"expires_at"`
	}{
		Owner:        owner,
		Organization: organization,
		Role:         role,
		ExpiresAt:    expiresAt,
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to generate request body: %w", err)
	}

	resp, err := c.HttpRequest(http.MethodPost, "/key/register", reqBody)
	if err != nil {
		return fmt.Errorf("POST request to register API key failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var newKey string
	err = json.NewDecoder(resp.Body).Decode(&newKey)
	if err != nil {
		return fmt.Errorf("failed to decode response body: %w", err)
	}

	fmt.Printf("API key for owner %s registered successfully: %s\n", owner, newKey)
	return nil
}

func (c *PortalClient) RevokeAPIKey(key string) error {
	req := struct {
		Key string `json:"key"`
	}{
		Key: key,
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to generate request body: %w", err)
	}

	resp, err := c.HttpRequest(http.MethodPost, "/key/revoke", reqBody)
	if err != nil {
		return fmt.Errorf("POST request to revoke API key failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	fmt.Println("API key revoked successfully")
	return nil
}
