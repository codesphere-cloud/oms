// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// SSHWorkspaceProxy is the SSH proxy into workspaces. Its service is configured through the
// pc-applications values instead.
type SSHWorkspaceProxy struct {
	BaseDomain string `yaml:"baseDomain"`
}

// sshWorkspaceProxyValues keeps the chart's default base domain (the Codesphere domain) unless the
// config sets one. The service settings only go to the pc-applications values.
func sshWorkspaceProxyValues(proxy *files.SSHWorkspaceProxyConfig) *SSHWorkspaceProxy {
	if proxy == nil || proxy.BaseDomain == "" {
		return nil
	}

	return &SSHWorkspaceProxy{BaseDomain: proxy.BaseDomain}
}
