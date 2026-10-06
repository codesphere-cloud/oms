// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package k0s

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestWaitForK0sReady(t *testing.T) {
	responses := []struct {
		output string
		err    error
	}{
		{output: "k0s is not running", err: errors.New("exit status 1")},
		{output: `{"pid":123,"role":"controller","workerToAPIConnectionStatus":{"success":false}}`},
		{output: `{"pid":123,"role":"controller","workerToAPIConnectionStatus":{"success":true}}`},
	}

	var calls int

	err := waitForK0sReady(context.Background(), time.Millisecond, func(context.Context) ([]byte, error) {
		if calls >= len(responses) {
			t.Fatal("queried k0s status after it reported readiness")
		}

		response := responses[calls]
		calls++

		return []byte(response.output), response.err
	})
	if err != nil {
		t.Fatalf("waitForK0sReady() error = %v", err)
	}

	if calls != len(responses) {
		t.Fatalf("status calls = %d, want %d", calls, len(responses))
	}
}

func TestWaitForK0sReadyTimesOutWithoutAPIReadiness(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := waitForK0sReady(ctx, time.Millisecond, func(context.Context) ([]byte, error) {
		return []byte(`{"pid":123,"role":"controller","workerToAPIConnectionStatus":{"success":false}}`), nil
	})
	if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "last k0s status") {
		t.Fatalf("waitForK0sReady() error = %v, want canceled context with last status", err)
	}
}

func TestWaitForK0sNodeAfterRegistration(t *testing.T) {
	responses := []struct {
		output string
		err    error
	}{
		{output: "The connection to the server was refused", err: errors.New("exit status 1")},
		{output: "No resources found"},
		{output: "node/first-node\n"},
	}
	var calls int

	node, err := waitForK0sNode(context.Background(), time.Millisecond, func(context.Context) ([]byte, error) {
		if calls >= len(responses) {
			t.Fatal("queried nodes after one was registered")
		}
		response := responses[calls]
		calls++
		return []byte(response.output), response.err
	})
	if err != nil || node != "node/first-node" {
		t.Fatalf("waitForK0sNode() = %q, %v; want node/first-node", node, err)
	}
	if calls != len(responses) {
		t.Fatalf("node queries = %d, want %d", calls, len(responses))
	}
}

func TestWaitForK0sNodeTimesOutWithoutRegistration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := waitForK0sNode(ctx, time.Millisecond, func(context.Context) ([]byte, error) {
		return []byte("No resources found"), nil
	})
	if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "No resources found") {
		t.Fatalf("waitForK0sNode() error = %v, want canceled context with last node query", err)
	}
}
