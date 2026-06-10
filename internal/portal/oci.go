// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package portal

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/codesphere-cloud/oms/internal/env"
)

// OCIClient handles pulling OCI container images through the OMS portal registry proxy.
type OCIClient struct {
	Env        env.Env
	HttpClient HttpClient
}

// NewOCIClient creates a new OCIClient with environment-based configuration.
func NewOCIClient() *OCIClient {
	return &OCIClient{
		Env:        env.NewEnv(),
		HttpClient: http.DefaultClient,
	}
}

// OCIImageIndex represents an OCI image index (multi-arch manifest list).
type OCIImageIndex struct {
	Manifests []OCIImageIndexEntry `json:"manifests"`
}

// OCIImageIndexEntry represents a single entry in an OCI image index.
type OCIImageIndexEntry struct {
	MediaType string       `json:"mediaType"`
	Digest    string       `json:"digest"`
	Platform  *OCIPlatform `json:"platform"`
}

// OCIPlatform represents the platform information in an OCI manifest.
type OCIPlatform struct {
	Architecture string `json:"architecture"`
	OS           string `json:"os"`
}

// OCIManifest represents an OCI image manifest.
type OCIManifest struct {
	Layers []OCILayer `json:"layers"`
	Config OCIConfig  `json:"config"`
}

// OCILayer represents a single layer in an OCI image manifest.
type OCILayer struct {
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
}

// OCIConfig represents the config blob reference in an OCI image manifest.
type OCIConfig struct {
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
}

// ImageRef holds the parsed components of an OCI image reference (org/repo:tag).
type ImageRef struct {
	Org  string
	Repo string
	Tag  string
}

// ParseImageRef parses an image reference string like "org/repo:tag".
func ParseImageRef(ref string) (ImageRef, error) {
	parts := strings.SplitN(ref, ":", 2)
	if len(parts) != 2 {
		return ImageRef{}, fmt.Errorf("invalid image reference %q: expected org/repo:tag format", ref)
	}
	tag := parts[1]

	slashParts := strings.SplitN(parts[0], "/", 2)
	if len(slashParts) != 2 {
		return ImageRef{}, fmt.Errorf("invalid image reference %q: expected org/repo:tag format", ref)
	}

	return ImageRef{
		Org:  slashParts[0],
		Repo: slashParts[1],
		Tag:  tag,
	}, nil
}

// PullImage pulls an OCI container image through the OMS portal registry proxy
// and saves all layers and config to the specified output directory.
func (c *OCIClient) PullImage(imageRef ImageRef, outputDir string) error {
	registryBase := c.Env.GetOmsRegistry()

	// Remove output dir and recreate it
	if err := os.RemoveAll(outputDir); err != nil {
		return fmt.Errorf("failed to clean output directory %q: %w", outputDir, err)
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory %q: %w", outputDir, err)
	}

	// Step 1: Fetch the manifest (may be an image index or a single manifest)
	manifestURL := fmt.Sprintf("%s/v2/%s/%s/manifests/%s", registryBase, imageRef.Org, imageRef.Repo, imageRef.Tag)
	log.Printf("Fetching manifest from %s", manifestURL)

	manifestBody, err := c.registryGet(manifestURL,
		"application/vnd.oci.image.index.v1+json, application/vnd.oci.image.manifest.v1+json")
	if err != nil {
		return fmt.Errorf("failed to fetch manifest: %w", err)
	}

	// Step 2: Check if it's an image index (multi-arch) and resolve to linux/amd64 manifest
	var manifestData json.RawMessage
	if err := json.Unmarshal(manifestBody, &manifestData); err != nil {
		return fmt.Errorf("failed to parse manifest JSON: %w", err)
	}

	// Try to parse as image index first
	var imageIndex OCIImageIndex
	if err := json.Unmarshal(manifestBody, &imageIndex); err == nil && len(imageIndex.Manifests) > 0 {
		log.Printf("Image index found with %d entries, selecting linux/amd64", len(imageIndex.Manifests))

		manifestDigest, err := c.resolveLinuxAmd64Manifest(imageIndex, imageRef)
		if err != nil {
			return fmt.Errorf("failed to resolve linux/amd64 manifest: %w", err)
		}

		// Fetch the resolved manifest
		manifestBody, err = c.registryGet(
			fmt.Sprintf("%s/v2/%s/%s/manifests/%s", registryBase, imageRef.Org, imageRef.Repo, manifestDigest),
			"application/vnd.oci.image.manifest.v1+json")
		if err != nil {
			return fmt.Errorf("failed to fetch resolved manifest: %w", err)
		}
	}

	// Step 3: Parse the single manifest
	var manifest OCIManifest
	if err := json.Unmarshal(manifestBody, &manifest); err != nil {
		return fmt.Errorf("failed to parse image manifest: %w", err)
	}

	// Step 4: Collect all blob digests (layers + config)
	blobs := make([]string, 0, len(manifest.Layers)+1)
	for _, layer := range manifest.Layers {
		blobs = append(blobs, layer.Digest)
	}
	blobs = append(blobs, manifest.Config.Digest)

	log.Printf("Downloading %d blobs...", len(blobs))

	// Step 5: Download each blob
	for _, digest := range blobs {
		if err := c.downloadBlob(registryBase, imageRef, digest, outputDir); err != nil {
			return fmt.Errorf("failed to download blob %s: %w", digest, err)
		}
	}

	return nil
}

// resolveLinuxAmd64Manifest finds the linux/amd64 manifest digest from an image index.
func (c *OCIClient) resolveLinuxAmd64Manifest(index OCIImageIndex, imageRef ImageRef) (string, error) {
	for _, entry := range index.Manifests {
		if entry.Platform != nil &&
			entry.Platform.OS == "linux" &&
			entry.Platform.Architecture == "amd64" {
			return entry.Digest, nil
		}
	}

	return "", fmt.Errorf("no linux manifest found in image index for %s/%s:%s", imageRef.Org, imageRef.Repo, imageRef.Tag)
}

// registryGet performs an authenticated GET request to the OCI registry endpoint.
func (c *OCIClient) registryGet(url, acceptHeader string) ([]byte, error) {
	apiKey, err := c.Env.GetOmsPortalApiKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get API key: %w", err)
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// OCI registry uses Basic auth: username "anything", password = API key
	auth := base64.StdEncoding.EncodeToString([]byte("anything:" + apiKey))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Accept", acceptHeader)

	resp, err := c.HttpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("registry returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return body, nil
}

// downloadBlob downloads a single blob and saves it to the output directory.
func (c *OCIClient) downloadBlob(registryBase string, imageRef ImageRef, digest, outputDir string) error {
	blobURL := fmt.Sprintf("%s/v2/%s/%s/blobs/%s", registryBase, imageRef.Org, imageRef.Repo, digest)

	apiKey, err := c.Env.GetOmsPortalApiKey()
	if err != nil {
		return fmt.Errorf("failed to get API key: %w", err)
	}

	req, err := http.NewRequest(http.MethodGet, blobURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create blob request: %w", err)
	}

	auth := base64.StdEncoding.EncodeToString([]byte("anything:" + apiKey))
	req.Header.Set("Authorization", "Basic "+auth)

	resp, err := c.HttpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download blob: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("blob download returned status %d: %s", resp.StatusCode, string(body))
	}

	// Replace colons with dashes in filename (Docker convention)
	filename := strings.ReplaceAll(digest, ":", "-")
	filePath := outputDir + "/" + filename

	out, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create file %q: %w", filePath, err)
	}
	defer out.Close()

	written, err := io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write blob to file: %w", err)
	}

	shortDigest := digest[7:19] // sha256:abcdef1234... -> abcdef1234
	if len(digest) > 19 {
		shortDigest = digest[7:19]
	} else if len(digest) > 7 {
		shortDigest = digest[7:]
	}
	log.Printf("  %s... %.1f MB", shortDigest, float64(written)/1048576.0)

	return nil
}
