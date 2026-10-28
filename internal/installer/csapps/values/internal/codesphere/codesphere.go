// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package codesphere builds the cs-applications chart's global values from config.codesphere.
package codesphere

import (
	"k8s.io/utils/ptr"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// Codesphere are the global values built from config.codesphere, with the old installer's defaults.
// The optional ones are left out when the config has none, so the chart's defaults apply.
type Codesphere struct {
	PublicIP                   string   `yaml:"publicIP"`
	WorkspaceHostingBaseDomain string   `yaml:"workspaceHostingBaseDomain"`
	VPN                        VPN      `yaml:"vpn"`
	DNSServers                 []string `yaml:"dnsServers"`
	Hosts                      []string `yaml:"hosts"`
	PrePullImages              bool     `yaml:"prePullImages"`

	SSHWorkspaceProxy     *files.SSHWorkspaceProxyConfig `yaml:"sshWorkspaceProxy,omitempty"`
	TLSCertificate        *TLSCertificate                `yaml:"tlsCertificate,omitempty"`
	UnderprovisionFactors *files.UnderprovisionFactors   `yaml:"underprovisionFactors,omitempty"`
	ExtraWorkspaceFiles   []files.ExtraWorkspaceFile     `yaml:"extraWorkspaceFiles,omitempty"`
	ExtraCAPem            string                         `yaml:"extraCaPem,omitempty"`
	ExtraWorkspaceEnvVars map[string]string              `yaml:"extraWorkspaceEnvVars,omitempty"`
}

// VPN is the peer id of the workspace VPN tunnels.
type VPN struct {
	LeftPeerID string `yaml:"leftPeerId,omitempty"`
}

// TLSCertificate is the custom certificate Codesphere serves.
type TLSCertificate struct {
	Cert string `yaml:"cert"`
	Key  string `yaml:"key"`
}

// Values builds the global values from config.codesphere.
func Values(conf files.CodesphereConfig) Codesphere {
	return Codesphere{
		PublicIP:                   conf.PublicIP,
		WorkspaceHostingBaseDomain: conf.WorkspaceHostingBaseDomain,
		VPN:                        VPN{LeftPeerID: conf.VPNLeftPeerID},
		DNSServers:                 dnsServers(conf.DNSServers),
		Hosts:                      []string{conf.Domain},
		PrePullImages:              ptr.Deref(conf.PrePullImages, true),
		SSHWorkspaceProxy:          conf.SSHWorkspaceProxy,
		TLSCertificate:             tlsCertificateValues(conf.CustomCertificate),
		UnderprovisionFactors:      conf.UnderprovisionFactors,
		ExtraWorkspaceFiles:        conf.ExtraWorkspaceFiles,
		ExtraCAPem:                 conf.ExtraCAPem,
		ExtraWorkspaceEnvVars:      conf.ExtraWorkspaceEnvVars,
	}
}

func dnsServers(configured []string) []string {
	if configured == nil {
		return []string{"8.8.8.8"}
	}

	return configured
}

func tlsCertificateValues(certificate *files.CustomCertificate) *TLSCertificate {
	if certificate == nil {
		return nil
	}

	return &TLSCertificate{Cert: certificate.Cert, Key: certificate.Key}
}
