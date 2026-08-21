// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package portal

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// stubClient returns queued responses in order, one per Do call.
type stubClient struct {
	responses []stubResponse
	calls     int
}

type stubResponse struct {
	resp *http.Response
	err  error
}

func (s *stubClient) Do(*http.Request) (*http.Response, error) {
	i := s.calls
	s.calls++
	if i >= len(s.responses) {
		return nil, errors.New("unexpected extra call")
	}
	return s.responses[i].resp, s.responses[i].err
}

func okResp(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func newTestRetryingClient(inner HttpClient) *retryingClient {
	return &retryingClient{
		inner:    inner,
		attempts: maxHttpAttempts,
		baseWait: 0,
		sleep:    func(time.Duration) {},
	}
}

func mustGetReq(t *testing.T) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, "https://portal.example.com/api/packages", nil)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	return req
}

func TestRetryingClient_RetriesTransportErrorThenSucceeds(t *testing.T) {
	stub := &stubClient{responses: []stubResponse{
		{nil, errors.New("unexpected EOF")},
		{okResp(http.StatusOK, "ok"), nil},
	}}
	c := newTestRetryingClient(stub)

	resp, err := c.Do(mustGetReq(t))
	if err != nil {
		t.Fatalf("expected success after retry, got error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if stub.calls != 2 {
		t.Fatalf("expected 2 calls, got %d", stub.calls)
	}
}

func TestRetryingClient_RetriesRetryable5xxThenSucceeds(t *testing.T) {
	stub := &stubClient{responses: []stubResponse{
		{okResp(http.StatusBadGateway, "bad gateway"), nil},
		{okResp(http.StatusOK, "ok"), nil},
	}}
	c := newTestRetryingClient(stub)

	resp, err := c.Do(mustGetReq(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if stub.calls != 2 {
		t.Fatalf("expected 2 calls, got %d", stub.calls)
	}
}

func TestRetryingClient_DoesNotRetryNon5xxStatus(t *testing.T) {
	stub := &stubClient{responses: []stubResponse{
		{okResp(http.StatusBadRequest, "bad request"), nil},
	}}
	c := newTestRetryingClient(stub)

	resp, err := c.Do(mustGetReq(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 to pass through, got %d", resp.StatusCode)
	}
	if stub.calls != 1 {
		t.Fatalf("expected 1 call (no retry), got %d", stub.calls)
	}
}

func TestRetryingClient_DoesNotRetryNonIdempotentMethod(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "https://portal.example.com/keys", strings.NewReader("body"))
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	stub := &stubClient{responses: []stubResponse{
		{nil, errors.New("unexpected EOF")},
	}}
	c := newTestRetryingClient(stub)

	_, doErr := c.Do(req)
	if doErr == nil {
		t.Fatal("expected the transport error to propagate for a POST")
	}
	if stub.calls != 1 {
		t.Fatalf("expected 1 call (POST not retried), got %d", stub.calls)
	}
}

func TestRetryingClient_ExhaustsAttemptsAndReturnsLastError(t *testing.T) {
	stub := &stubClient{responses: []stubResponse{
		{nil, errors.New("unexpected EOF")},
		{nil, errors.New("unexpected EOF")},
		{nil, errors.New("unexpected EOF")},
	}}
	c := newTestRetryingClient(stub)

	_, err := c.Do(mustGetReq(t))
	if err == nil {
		t.Fatal("expected an error after exhausting attempts")
	}
	if stub.calls != maxHttpAttempts {
		t.Fatalf("expected %d calls, got %d", maxHttpAttempts, stub.calls)
	}
}
