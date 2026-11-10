// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package portal

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
)

//mockery:generate: true
type Http interface {
	Request(ctx context.Context, url string, method string, body io.Reader) (responseBody []byte, err error)
	Get(ctx context.Context, url string) (responseBody []byte, err error)
	Download(ctx context.Context, url string, file io.Writer, quiet bool) error
}

type HttpWrapper struct {
	HttpClient HttpClient
}

func NewHttpWrapper() *HttpWrapper {
	return &HttpWrapper{
		HttpClient: http.DefaultClient,
	}
}

// Request sends a request and returns the response body, failing on non-2xx statuses.
func (c *HttpWrapper) Request(ctx context.Context, url string, method string, body io.Reader) (responseBody []byte, err error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		log.Fatalf("Error creating request: %v", err)
		return
	}

	resp, err := c.HttpClient.Do(req)
	if err != nil {
		return []byte{}, fmt.Errorf("failed to send request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return []byte{}, fmt.Errorf("failed request with status: %d", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return []byte{}, fmt.Errorf("failed to read response body: %w", err)
	}

	return respBody, nil
}

// Get sends a GET request and returns the response body.
func (c *HttpWrapper) Get(ctx context.Context, url string) (responseBody []byte, err error) {
	return c.Request(ctx, url, http.MethodGet, nil)
}

// Download streams the response body of a GET request to file.
func (c *HttpWrapper) Download(ctx context.Context, url string, file io.Writer, quiet bool) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.HttpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("failed to get body: %d", resp.StatusCode)
	}

	counter := file
	if !quiet {
		counter = NewWriteCounter(file)
	}

	_, err = io.Copy(counter, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to copy response body to file: %w", err)
	}

	log.Println("Download finished successfully.")
	return nil
}
