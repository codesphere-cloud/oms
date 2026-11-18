// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package files

import (
	"fmt"
	"slices"
)

const (
	// DefaultSSHWorkspaceProxyServiceType is the service type of the SSH workspace proxy when none is configured.
	DefaultSSHWorkspaceProxyServiceType = "LoadBalancer"
	// DefaultSSHWorkspaceProxyNodePort is the node port of the SSH workspace proxy when none is configured.
	DefaultSSHWorkspaceProxyNodePort = 30022

	maxPort = 65535
)

// SSHWorkspaceProxyServiceTypes are the Kubernetes service types the SSH workspace proxy can be exposed with.
var SSHWorkspaceProxyServiceTypes = []string{"ClusterIP", "NodePort", "LoadBalancer"}

// ServiceType returns the configured service type, or the default when unset.
func (s *SSHWorkspaceProxyServiceConfig) ServiceType() string {
	if s == nil || s.Type == "" {
		return DefaultSSHWorkspaceProxyServiceType
	}

	return s.Type
}

// ServiceNodePort returns the configured node port, or the default when unset.
func (s *SSHWorkspaceProxyServiceConfig) ServiceNodePort() int {
	if s == nil || s.NodePort == 0 {
		return DefaultSSHWorkspaceProxyNodePort
	}

	return s.NodePort
}

// UsesNodePort reports whether the service type allocates a node port. Kubernetes rejects a
// node port on a ClusterIP service.
func (s *SSHWorkspaceProxyServiceConfig) UsesNodePort() bool {
	return s.ServiceType() != "ClusterIP"
}

// Validate checks the service type and node port.
func (s *SSHWorkspaceProxyServiceConfig) Validate() error {
	if s == nil {
		return nil
	}

	if !slices.Contains(SSHWorkspaceProxyServiceTypes, s.ServiceType()) {
		return fmt.Errorf("invalid codesphere.sshWorkspaceProxy.service.type %q (must be one of %v)", s.Type, SSHWorkspaceProxyServiceTypes)
	}

	if s.NodePort < 0 || s.NodePort > maxPort {
		return fmt.Errorf("invalid codesphere.sshWorkspaceProxy.service.nodePort %d (must be between 1 and %d)", s.NodePort, maxPort)
	}

	return nil
}

// ServiceConfig returns the configured service, or nil when the config or its service block is unset.
func (c *SSHWorkspaceProxyConfig) ServiceConfig() *SSHWorkspaceProxyServiceConfig {
	if c == nil {
		return nil
	}

	return c.Service
}

// EnsureSSHWorkspaceProxyService returns the SSH workspace proxy service config, creating it
// and its parent block when unset.
func (c *CodesphereConfig) EnsureSSHWorkspaceProxyService() *SSHWorkspaceProxyServiceConfig {
	if c.SSHWorkspaceProxy == nil {
		c.SSHWorkspaceProxy = &SSHWorkspaceProxyConfig{}
	}

	if c.SSHWorkspaceProxy.Service == nil {
		c.SSHWorkspaceProxy.Service = &SSHWorkspaceProxyServiceConfig{}
	}

	return c.SSHWorkspaceProxy.Service
}
