// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"context"
	"fmt"
	"os/exec"
)

// K0sReadinessClient queries the local k0s service and registered nodes.
//
//mockery:generate: true
type K0sReadinessClient interface {
	Status(context.Context) ([]byte, error)
	Nodes(context.Context) ([]byte, error)
}

type k0sReadinessClient struct {
	binary string
}

// NewK0sReadinessClient queries k0s using the installed binary.
func NewK0sReadinessClient(binary string) K0sReadinessClient {
	return &k0sReadinessClient{binary: binary}
}

func (c *k0sReadinessClient) Status(ctx context.Context) ([]byte, error) {
	out, err := exec.CommandContext(ctx, c.binary, "status", "-o", "json").CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("query k0s status: %w", err)
	}

	return out, nil
}

func (c *k0sReadinessClient) Nodes(ctx context.Context) ([]byte, error) {
	out, err := exec.CommandContext(ctx, c.binary, "kubectl", "get", "nodes", "-o", "name").CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("list k0s nodes: %w", err)
	}

	return out, nil
}
