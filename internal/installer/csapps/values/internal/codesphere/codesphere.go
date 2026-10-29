// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package codesphere builds the values of the codesphere key.
package codesphere

import (
	"k8s.io/utils/ptr"

	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/cluster"
	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/images"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// Codesphere are the values of the codesphere key, from the config with the installer's defaults.
// The optional ones are left out when the config has none, so the chart's defaults apply.
type Codesphere struct {
	PublicIP                   string            `yaml:"publicIP"`
	CertIssuer                 CertIssuer        `yaml:"certIssuer"`
	WorkspaceHostingBaseDomain string            `yaml:"workspaceHostingBaseDomain"`
	SSHWorkspaceProxy          SSHWorkspaceProxy `yaml:"sshWorkspaceProxy"`
	VPN                        VPN               `yaml:"vpn"`
	DNSServers                 []string          `yaml:"dnsServers"`
	Hosts                      []string          `yaml:"hosts"`
	PrePullImages              bool              `yaml:"prePullImages"`
	CustomDomains              CustomDomains     `yaml:"customDomains"`
	DeployConfig               DeployConfig      `yaml:"deployConfig"`
	WorkspaceImages            WorkspaceImages   `yaml:"workspaceImages"`
	FrontendGateway            FrontendGateway   `yaml:"frontendGateway"`

	IPService             *IPService             `yaml:"ipService,omitempty"`
	TLSCertificate        *TLSCertificate        `yaml:"tlsCertificate,omitempty"`
	UnderprovisionFactors *UnderprovisionFactors `yaml:"underprovisionFactors,omitempty"`
	ExtraWorkspaceFiles   []ExtraWorkspaceFile   `yaml:"extraWorkspaceFiles,omitempty"`
	ExtraCAPem            string                 `yaml:"extraCaPem,omitempty"`
	ExtraWorkspaceEnvVars map[string]string      `yaml:"extraWorkspaceEnvVars,omitempty"`
}

// SSHWorkspaceProxy is the ssh proxy's jump host domain.
type SSHWorkspaceProxy struct {
	BaseDomain string `yaml:"baseDomain,omitempty"`
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

// UnderprovisionFactors scale the resources workspaces request.
type UnderprovisionFactors struct {
	CPU    float64 `yaml:"cpu"`
	Memory float64 `yaml:"memory"`
}

// ExtraWorkspaceFile is a file every workspace gets.
type ExtraWorkspaceFile struct {
	Name    string `yaml:"name"`
	Content string `yaml:"content"`
	Encoded bool   `yaml:"encoded"`
}

// Values builds the values of the codesphere key. Some of them also depend on the cluster gateways,
// MetalLB and the registry.
func Values(
	conf files.CodesphereConfig,
	imgs images.Images,
	gateways cluster.Gateways,
	metalLB *files.MetalLBConfig,
	registry *files.RegistryConfig,
) (Codesphere, error) {
	deployConfig, err := deployConfigValues(conf, registry, imgs)
	if err != nil {
		return Codesphere{}, err
	}

	workspaceImages, err := workspaceImagesValues(conf.WorkspaceImages, imgs)
	if err != nil {
		return Codesphere{}, err
	}

	certIssuer, err := certIssuerValues(conf.CertIssuer)
	if err != nil {
		return Codesphere{}, err
	}

	frontendGateway, err := frontendGatewayValues(imgs, conf.CertIssuer, gateways.Codesphere.Enabled)
	if err != nil {
		return Codesphere{}, err
	}

	ipService, err := ipServiceValues(conf.IPService, metalLB)
	if err != nil {
		return Codesphere{}, err
	}

	return Codesphere{
		PublicIP:                   conf.PublicIP,
		CertIssuer:                 certIssuer,
		WorkspaceHostingBaseDomain: conf.WorkspaceHostingBaseDomain,
		SSHWorkspaceProxy:          SSHWorkspaceProxy{BaseDomain: sshBaseDomain(conf.SSHWorkspaceProxy)},
		VPN:                        VPN{LeftPeerID: conf.VPNLeftPeerID},
		DNSServers:                 dnsServers(conf.DNSServers),
		Hosts:                      []string{conf.Domain},
		PrePullImages:              ptr.Deref(conf.PrePullImages, true),
		CustomDomains:              customDomainsValues(conf.CustomDomains, gateways.Workspaces.Use),
		DeployConfig:               deployConfig,
		WorkspaceImages:            workspaceImages,
		FrontendGateway:            frontendGateway,
		IPService:                  ipService,
		TLSCertificate:             tlsCertificateValues(conf.CustomCertificate),
		UnderprovisionFactors:      underprovisionFactorsValues(conf.UnderprovisionFactors),
		ExtraWorkspaceFiles:        extraWorkspaceFilesValues(conf.ExtraWorkspaceFiles),
		ExtraCAPem:                 conf.ExtraCAPem,
		ExtraWorkspaceEnvVars:      conf.ExtraWorkspaceEnvVars,
	}, nil
}

func dnsServers(configured []string) []string {
	if configured == nil {
		return []string{"8.8.8.8"}
	}

	return configured
}

func sshBaseDomain(proxy *files.SSHWorkspaceProxyConfig) string {
	if proxy == nil {
		return ""
	}

	return proxy.BaseDomain
}

func tlsCertificateValues(certificate *files.CustomCertificate) *TLSCertificate {
	if certificate == nil {
		return nil
	}

	return &TLSCertificate{Cert: certificate.Cert, Key: certificate.Key}
}

func underprovisionFactorsValues(factors *files.UnderprovisionFactors) *UnderprovisionFactors {
	if factors == nil {
		return nil
	}

	return &UnderprovisionFactors{CPU: factors.CPU, Memory: factors.Memory}
}

func extraWorkspaceFilesValues(workspaceFiles []files.ExtraWorkspaceFile) []ExtraWorkspaceFile {
	if workspaceFiles == nil {
		return nil
	}

	values := make([]ExtraWorkspaceFile, len(workspaceFiles))
	for i, file := range workspaceFiles {
		values[i] = ExtraWorkspaceFile{Name: file.Name, Content: file.Content, Encoded: file.Encoded}
	}

	return values
}
