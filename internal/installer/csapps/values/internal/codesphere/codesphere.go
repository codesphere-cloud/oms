// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package codesphere builds the cs-applications chart's global values from config.codesphere.
package codesphere

import (
	"k8s.io/utils/ptr"

	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/cluster"
	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/images"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// Codesphere are the global values built from config.codesphere, with the old installer's defaults.
// The optional ones are left out when the config has none, so the chart's defaults apply.
type Codesphere struct {
	PublicIP                   string          `yaml:"publicIP"`
	CertIssuer                 CertIssuer      `yaml:"certIssuer"`
	WorkspaceHostingBaseDomain string          `yaml:"workspaceHostingBaseDomain"`
	VPN                        VPN             `yaml:"vpn"`
	DNSServers                 []string        `yaml:"dnsServers"`
	Hosts                      []string        `yaml:"hosts"`
	PrePullImages              bool            `yaml:"prePullImages"`
	GitProviders               GitProviders    `yaml:"gitProviders"`
	Experiments                Flags           `yaml:"experiments"`
	Features                   Flags           `yaml:"features"`
	CustomDomains              CustomDomains   `yaml:"customDomains"`
	OAuth                      OAuth           `yaml:"oauth"`
	SignUp                     SignUp          `yaml:"signUp"`
	DeployConfig               DeployConfig    `yaml:"deployConfig"`
	WorkspaceImages            WorkspaceImages `yaml:"workspaceImages"`
	FrontendGateway            FrontendGateway `yaml:"frontendGateway"`

	IPService             *IPService                     `yaml:"ipService,omitempty"`
	SSHWorkspaceProxy     *files.SSHWorkspaceProxyConfig `yaml:"sshWorkspaceProxy,omitempty"`
	Branding              *files.BrandingConfig          `yaml:"branding,omitempty"`
	TLSCertificate        *TLSCertificate                `yaml:"tlsCertificate,omitempty"`
	UnderprovisionFactors *files.UnderprovisionFactors   `yaml:"underprovisionFactors,omitempty"`
	ExtraWorkspaceFiles   []files.ExtraWorkspaceFile     `yaml:"extraWorkspaceFiles,omitempty"`
	SMTP                  *files.SMTPConfig              `yaml:"smtp,omitempty"`
	ExtraCAPem            string                         `yaml:"extraCaPem,omitempty"`
	ExtraWorkspaceEnvVars map[string]string              `yaml:"extraWorkspaceEnvVars,omitempty"`
}

// VPN is the peer id of the workspace VPN tunnels.
type VPN struct {
	LeftPeerID string `yaml:"leftPeerId,omitempty"`
}

// TLSCertificate is the custom certificate Codesphere serves. Its key is the vault entry KeyName.
type TLSCertificate struct {
	Cert    string `yaml:"cert"`
	KeyName string `yaml:"keyName"`
}

// Inputs are what the codesphere values depend on besides the codesphere config.
type Inputs struct {
	Images   images.Images
	Gateways cluster.Gateways
	MetalLB  *files.MetalLBConfig
	Registry *files.RegistryConfig
}

// Values builds the values of the codesphere key.
func Values(conf files.CodesphereConfig, in Inputs) (Codesphere, error) {
	deployConfig, err := deployConfigValues(conf, in.Images, in.Registry)
	if err != nil {
		return Codesphere{}, err
	}

	workspaceImages, err := workspaceImagesValues(conf.WorkspaceImages, in.Images)
	if err != nil {
		return Codesphere{}, err
	}

	certIssuer, err := certIssuerValues(conf.CertIssuer)
	if err != nil {
		return Codesphere{}, err
	}

	frontendGateway, err := frontendGatewayValues(in.Images, conf.CertIssuer, in.Gateways.Codesphere.Enabled)
	if err != nil {
		return Codesphere{}, err
	}

	ipService, err := ipServiceValues(conf.IPService, in.MetalLB)
	if err != nil {
		return Codesphere{}, err
	}

	smtp, err := smtpValues(conf.Email)
	if err != nil {
		return Codesphere{}, err
	}

	return Codesphere{
		PublicIP:                   conf.PublicIP,
		CertIssuer:                 certIssuer,
		WorkspaceHostingBaseDomain: conf.WorkspaceHostingBaseDomain,
		VPN:                        VPN{LeftPeerID: conf.VPNLeftPeerID},
		DNSServers:                 dnsServers(conf.DNSServers),
		Hosts:                      []string{conf.Domain},
		PrePullImages:              ptr.Deref(conf.PrePullImages, true),
		GitProviders:               gitProvidersValues(conf.GitProviders),
		Experiments:                experimentsValues(conf, in.Gateways.Workspaces.Enabled),
		Features:                   featuresValues(conf.Features),
		CustomDomains:              customDomainsValues(conf.CustomDomains, in.Gateways.Workspaces.Use),
		OAuth:                      oauthValues(conf.OAuth),
		SignUp:                     signUpValues(conf.SignUp),
		DeployConfig:               deployConfig,
		WorkspaceImages:            workspaceImages,
		FrontendGateway:            frontendGateway,
		IPService:                  ipService,
		SSHWorkspaceProxy:          conf.SSHWorkspaceProxy,
		Branding:                   conf.Branding,
		TLSCertificate:             tlsCertificateValues(conf.CustomCertificate),
		UnderprovisionFactors:      conf.UnderprovisionFactors,
		ExtraWorkspaceFiles:        conf.ExtraWorkspaceFiles,
		SMTP:                       smtp,
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

func tlsCertificateValues(certificate *files.CustomCertificate) *TLSCertificate {
	if certificate == nil {
		return nil
	}

	return &TLSCertificate{Cert: certificate.Cert, KeyName: files.SecretCustomCertificateKey}
}
