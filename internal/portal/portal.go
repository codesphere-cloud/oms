// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package portal

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
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
	DownloadBuildArtifact(product Product, build Build, file io.Writer, startByte int, quiet bool) error
	VerifyBuildArtifactDownload(file io.Reader, download Build) error
	RegisterAPIKey(owner string, organization string, role string, expiresAt time.Time) (*ApiKey, error)
	RevokeAPIKey(key string) error
	UpdateAPIKey(key string, expiresAt time.Time) error
	ListAPIKeys() ([]ApiKey, error)
	GetApiKeyId(oldKey string) (string, error)
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
		HttpClient: newConfiguredHttpClient(),
	}
}

// newConfiguredHttpClient creates an HTTP client with proper timeouts
func newConfiguredHttpClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Minute,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   30 * time.Second,
			ResponseHeaderTimeout: 60 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			IdleConnTimeout:       90 * time.Second,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   10,
		},
	}
}

type Product string

const (
	CodesphereProduct Product = "codesphere"
	OmsProduct        Product = "oms"
)

// AuthorizedHttpRequest sends a HTTP request with the necessary authorization headers.
func (c *PortalClient) AuthorizedHttpRequest(req *http.Request) (resp *http.Response, err error) {
	apiKey, err := c.Env.GetOmsPortalApiKey()
	if err != nil {
		err = fmt.Errorf("failed to get API Key: %w", err)
		return
	}

	req.Header.Set("X-API-Key", apiKey)

	resp, err = c.HttpClient.Do(req)
	if err != nil {
		err = fmt.Errorf("failed to send request: %w", err)
		return
	}

	if resp.StatusCode == http.StatusUnauthorized {
		log.Println("You need a valid OMS API Key, please reach out to the Codesphere support at support@codesphere.com to request a new API Key.")
		log.Println("If you already have an API Key, make sure to set it using the environment variable OMS_PORTAL_API_KEY")
	}
	var respBody []byte
	if resp.StatusCode >= 300 {
		if resp.Body != nil {
			respBody, _ = io.ReadAll(resp.Body)
		}
		log.Printf("Non-2xx response received - Status: %d, Body: %s", resp.StatusCode, string(respBody))
		err = fmt.Errorf("unexpected response status: %d - %s, %s", resp.StatusCode, http.StatusText(resp.StatusCode), string(respBody))
		return
	}

	return
}

// HttpRequest sends an unauthorized HTTP request to the portal API with the specified method, path, and body.
func (c *PortalClient) HttpRequest(method string, path string, body []byte) (resp *http.Response, err error) {
	requestBody := bytes.NewBuffer(body)
	url, err := url.JoinPath(c.Env.GetOmsPortalApi(), path)
	if err != nil {
		err = fmt.Errorf("failed to get generate URL: %w", err)
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
	return c.AuthorizedHttpRequest(req)
}

// GetBody sends a GET request to the specified path and returns the response body and status code.
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

// ListBuilds retrieves the list of available builds for the specified product.
func (c *PortalClient) ListBuilds(product Product) (availablePackages Builds, err error) {
	log.Printf("Fetching available %s packages from portal...", product)
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

// GetBuild retrieves a specific build for the given product, version, and hash.
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
		return Build{}, fmt.Errorf("version '%s' with hash '%s' not found", version, hash)
	}

	// Builds are always ordered by date, return newest build
	return matchingPackages[len(matchingPackages)-1], nil
}

// DownloadBuildArtifact downloads the build artifact for the specified product and build.
func (c *PortalClient) DownloadBuildArtifact(product Product, build Build, file io.Writer, startByte int, quiet bool) error {
	reqBody, err := json.Marshal(build)
	if err != nil {
		return fmt.Errorf("failed to generate request body: %w", err)
	}

	url, err := url.JoinPath(c.Env.GetOmsPortalApi(), fmt.Sprintf("/packages/%s/download", product))
	if err != nil {
		return fmt.Errorf("failed to get generate URL: %w", err)
	}
	bodyReader := bytes.NewBuffer(reqBody)
	req, err := http.NewRequest(http.MethodGet, url, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create GET request to download build: %w", err)
	}
	if startByte > 0 {
		log.Printf("Resuming download of existing file at byte %d\n", startByte)
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", startByte))
	}

	// Download the file from startByte to allow resuming
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.AuthorizedHttpRequest(req)
	if err != nil {
		return fmt.Errorf("GET request to download build failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if !quiet && resp.ContentLength > 0 {
		log.Printf("Starting download of %s...", byteCountToHumanReadable(resp.ContentLength))
	}

	// Create a WriteCounter to wrap the output file and report progress, unless quiet is requested.
	// Default behavior: report progress. Quiet callers should pass true for quiet.
	counter := file
	if !quiet {
		totalSize := resp.ContentLength
		if startByte > 0 && totalSize > 0 {
			totalSize = totalSize + int64(startByte)
		}
		counter = NewWriteCounterWithTotal(file, totalSize, int64(startByte))
	}

	_, err = io.Copy(counter, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to copy response body to file: %w", err)
	}

	log.Println("Download finished successfully.")
	return nil
}

func (c *PortalClient) VerifyBuildArtifactDownload(file io.Reader, download Build) error {
	// skip if oms-portal does not provide MD5Sum (older builds)
	if download.Artifacts[0].Md5Sum == "" {
		return nil
	}

	log.Println("Calculating MD5 checksum to verify download integrity...")

	hash := md5.New()

	_, err := io.Copy(hash, file)
	if err != nil {
		return fmt.Errorf("failed to compute checksum: %w", err)
	}

	md5Sum := hex.EncodeToString(hash.Sum(nil))

	if !strings.EqualFold(download.Artifacts[0].Md5Sum, md5Sum) {
		return fmt.Errorf("invalid md5Sum: expected %s, but got %s", download.Artifacts[0].Md5Sum, md5Sum)
	}

	log.Println("File checksum verified successfully.")

	return nil
}

// RegisterAPIKey registers a new API key with the specified parameters.
func (c *PortalClient) RegisterAPIKey(owner string, organization string, role string, expiresAt time.Time) (*ApiKey, error) {
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
		return nil, fmt.Errorf("failed to generate request body: %w", err)
	}

	resp, err := c.HttpRequest(http.MethodPost, "/key/register", reqBody)
	if err != nil {
		return nil, fmt.Errorf("POST request to register API key failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	responseBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, fmt.Errorf("failed to read response body: %w", readErr)
	}

	newKey := &ApiKey{}
	err = json.Unmarshal(responseBody, newKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decode response body: %w", err)
	}

	return newKey, nil
}

// RevokeAPIKey revokes the API key with the specified key ID.
func (c *PortalClient) RevokeAPIKey(keyId string) error {
	req := struct {
		KeyID string `json:"keyId"`
	}{
		KeyID: keyId,
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

	log.Println("API key revoked successfully!")

	return nil
}

// UpdateAPIKey updates the expiration date of the specified API key.
func (c *PortalClient) UpdateAPIKey(key string, expiresAt time.Time) error {
	req := struct {
		Key       string    `json:"keyId"`
		ExpiresAt time.Time `json:"expiresAt"`
	}{
		Key:       key,
		ExpiresAt: expiresAt,
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to generate request body: %w", err)
	}

	resp, err := c.HttpRequest(http.MethodPost, "/key/update", reqBody)
	if err != nil {
		return fmt.Errorf("POST request to update API key failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	log.Println("API key updated successfully")
	return nil
}

// ListAPIKeys retrieves the list of API keys.
func (c *PortalClient) ListAPIKeys() ([]ApiKey, error) {
	res, _, err := c.GetBody("/keys")
	if err != nil {
		return nil, fmt.Errorf("failed to list api keys: %w", err)
	}

	var keys []ApiKey
	if err := json.Unmarshal(res, &keys); err != nil {
		return nil, fmt.Errorf("failed to parse api keys response: %w", err)
	}

	return keys, nil
}

// GetApiKeyId retrieves the key ID by sending the old key in the request header.
func (c *PortalClient) GetApiKeyId(oldKey string) (string, error) {
	url, err := url.JoinPath(c.Env.GetOmsPortalApi(), "/key")
	if err != nil {
		return "", fmt.Errorf("failed to generate URL: %w", err)
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-API-Key", oldKey)

	resp, err := c.HttpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("unexpected response status: %d - %s, %s", resp.StatusCode, http.StatusText(resp.StatusCode), string(respBody))
	}

	var result struct {
		KeyID string `json:"keyId"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	return result.KeyID, nil
}
