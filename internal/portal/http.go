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
	Request(url string, method string, body io.Reader) (responseBody []byte, err error)
	Get(url string) (responseBody []byte, err error)
	Download(url string, file io.Writer, quiet bool) error
}

type HttpWrapper struct {
	HttpClient HttpClient
	// ctx is the command context. Requests are cancelled with it.
	ctx context.Context
}

// NewHTTPWrapper returns an HTTP wrapper whose requests are cancelled with ctx.
func NewHTTPWrapper(ctx context.Context) *HttpWrapper {
	return &HttpWrapper{
		HttpClient: http.DefaultClient,
		ctx:        ctx,
	}
}

// context returns the wrapper's context, or a background context for a wrapper built without one.
func (c *HttpWrapper) context() context.Context {
	if c.ctx == nil {
		return context.Background()
	}

	return c.ctx
}

func (c *HttpWrapper) Request(url string, method string, body io.Reader) (responseBody []byte, err error) {
	req, err := http.NewRequestWithContext(c.context(), method, url, body)
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

func (c *HttpWrapper) Get(url string) (responseBody []byte, err error) {
	return c.Request(url, http.MethodGet, nil)
}

func (c *HttpWrapper) Download(url string, file io.Writer, quiet bool) error {
	req, err := http.NewRequestWithContext(c.context(), http.MethodGet, url, nil)
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
