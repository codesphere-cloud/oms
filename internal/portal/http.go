package portal

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"slices"

	"github.com/codesphere-cloud/oms/internal/env"
)

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

func (c *PortalClient) Get(path string) (body []byte, status int, err error) {
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

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Fatalf("Error creating request: %v", err)
		return
	}

	req.Header.Set("X-API-Key", apiKey)

	resp, err := c.HttpClient.Do(req)
	if err != nil {
		err = fmt.Errorf("failed to send request: %w", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	status = resp.StatusCode
	if resp.StatusCode == http.StatusUnauthorized {
		fmt.Println("You need a valid OMS API Key, please reach out to the Codesphere support at support@codesphere.com to request a new API Key.")
		fmt.Println("If you already have an API Key, make sure to set it using the environment variable OMS_PORTAL_API_KEY")
	}
	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("unexpected response status: %d - %s", resp.StatusCode, http.StatusText(status))
		return
	}

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		err = fmt.Errorf("failed to read response body: %w", err)
		return
	}

	return
}

func (c *PortalClient) ListCodespherePackages() (availablePackages CodesphereBuilds, err error) {
	res, _, err := c.Get("/packages/codesphere")
	if err != nil {
		err = fmt.Errorf("failed to list packages: %w", err)
		return
	}
	err = json.Unmarshal(res, &availablePackages)
	if err != nil {
		err = fmt.Errorf("failed to parse list packages response: %w", err)
		return
	}
	slices.SortFunc(availablePackages.Builds, compareBuilds)

	return
}

func compareBuilds(l, r CodesphereBuild) int {
	if l.Date.Before(r.Date) {
		return -1
	}
	if l.Date.Equal(r.Date) && l.Internal == r.Internal {
		return 0
	}
	return 1
}
