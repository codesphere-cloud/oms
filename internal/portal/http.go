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
	"time"

	"github.com/codesphere-cloud/oms/internal/env"
)

type Portal interface {
	DownloadBuildArtifact(product Product, build Build, file io.Writer) error
	GetLatestBuild(product Product, version string) (Build, error)
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

func (c *PortalClient) Get(path string, body []byte) (resp *http.Response, err error) {
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

	req, err := http.NewRequest("GET", url, requestBody)
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
	if resp.StatusCode != http.StatusOK {
		if resp.Body != nil {
			respBody, _ = io.ReadAll(resp.Body)
		}
		err = fmt.Errorf("unexpected response status: %d - %s, %s", resp.StatusCode, http.StatusText(resp.StatusCode), string(respBody))
		return
	}

	return
}

func (c *PortalClient) GetBody(path string) (body []byte, status int, err error) {
	resp, err := c.Get(path, []byte{})
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
	slices.SortFunc(availablePackages.Builds, compareBuilds)

	return
}

func (c *PortalClient) GetCodesphereBuildByVersion(version string) (Build, error) {
	latestBuild, err := c.GetLatestBuild(CodesphereProduct, version)
	if err != nil {
		return Build{}, fmt.Errorf("failed to get latest build for version %s: %w", version, err)
	}

	return latestBuild, nil
}

func compareBuilds(l, r Build) int {
	if l.Date.Before(r.Date) {
		return -1
	}
	if l.Date.Equal(r.Date) && l.Internal == r.Internal {
		return 0
	}
	return 1
}

func (c *PortalClient) DownloadBuildArtifact(product Product, build Build, file io.Writer) error {
	reqBody, err := json.Marshal(build)
	if err != nil {
		return fmt.Errorf("failed to generate request body: %w", err)
	}

	resp, err := c.Get(fmt.Sprintf("/packages/%s/download", product), reqBody)
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

func (c *PortalClient) GetLatestBuild(product Product, version string) (Build, error) {
	packages, err := c.ListBuilds(product)
	if err != nil {
		return Build{}, fmt.Errorf("failed to list %s packages: %w", product, err)
	}

	if len(packages.Builds) == 0 {
		return Build{}, errors.New("no builds returned")
	}

	if version == "" {
		return packages.Builds[len(packages.Builds)-1], nil
	}

	matchingPackages := []Build{}
	for _, build := range packages.Builds {
		if build.Version == version {
			// Builds are always ordered by date, newest build is latest version
			matchingPackages = append(matchingPackages, build)
		}
	}

	if len(matchingPackages) == 0 {
		return Build{}, fmt.Errorf("version %s not found", version)
	}

	// Builds are always ordered by date, return newest build
	return matchingPackages[len(matchingPackages)-1], nil
}

// WriteCounter is a custom io.Writer that counts bytes written and logs progress.
type WriteCounter struct {
	Written     int64
	LastUpdate  time.Time
	Writer      io.Writer
	currentAnim int
}

// NewWriteCounter creates a new WriteCounter.
func NewWriteCounter(writer io.Writer) *WriteCounter {
	return &WriteCounter{
		Writer:     writer,
		LastUpdate: time.Now(), // Initialize last update time
	}
}

// Write implements the io.Writer interface for WriteCounter.
func (wc *WriteCounter) Write(p []byte) (int, error) {
	// Write the bytes to the underlying writer
	n, err := wc.Writer.Write(p)
	if err != nil {
		return n, err
	}

	wc.Written += int64(n)

	if time.Since(wc.LastUpdate) >= 100*time.Millisecond {
		fmt.Printf("\rDownloading... %s transferred %c \033[K", byteCountToHumanReadable(wc.Written), wc.animate())
		wc.LastUpdate = time.Now()
	}

	return n, nil
}

// byteCountToHumanReadable converts a byte count to a human-readable format (e.g., KB, MB, GB).
func byteCountToHumanReadable(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func (wc *WriteCounter) animate() byte {
	anim := "/-\\|"
	wc.currentAnim = (wc.currentAnim + 1) % len(anim)
	return anim[wc.currentAnim]
}
