// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package codesphere builds the values of the codesphere key.
package codesphere

import (
	"maps"

	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/cluster"
	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/images"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// Values builds the values of the codesphere key. Some of them also depend on the cluster gateways,
// MetalLB and the registry.
func Values(
	conf files.CodesphereConfig,
	imgs images.Images,
	gateways cluster.Gateways,
	metalLB *files.MetalLBConfig,
	registry *files.RegistryConfig,
) (map[string]any, error) {
	if err := validateDeployConfig(conf); err != nil {
		return nil, err
	}

	if err := validateTelemetryExport(conf.TelemetryExport); err != nil {
		return nil, err
	}

	resolved, err := resolvedValues(conf, imgs, gateways, metalLB, registry)
	if err != nil {
		return nil, err
	}

	values := configuredValues(conf, gateways)
	maps.Copy(values, optionalValues(conf))
	maps.Copy(values, resolved)

	return values, nil
}

// configuredValues are the values taken from the config with the installer's defaults.
func configuredValues(conf files.CodesphereConfig, gateways cluster.Gateways) map[string]any {
	sshWorkspaceProxy := omitEmpty(map[string]any{
		"baseDomain": sshBaseDomain(conf.SSHWorkspaceProxy),
	})
	vpn := omitEmpty(map[string]any{"leftPeerId": conf.VPNLeftPeerID})
	customDomains := customDomainsValues(conf.CustomDomains, gateways.Workspaces.Use)
	planID := intOr(conf.ManagedServiceWorkspacePlanID, defaultManagedServicePlanID)

	return map[string]any{
		"publicIP":                      conf.PublicIP,
		"certIssuer":                    certIssuerValues(conf.CertIssuer),
		"workspaceHostingBaseDomain":    conf.WorkspaceHostingBaseDomain,
		"sshWorkspaceProxy":             sshWorkspaceProxy,
		"vpn":                           vpn,
		"dnsServers":                    dnsServers(conf.DNSServers),
		"hosts":                         []string{conf.Domain},
		"prePullImages":                 boolOr(conf.PrePullImages, true),
		"gitProviders":                  gitProvidersValues(conf.GitProviders),
		"experiments":                   experimentsValues(conf, gateways.Workspaces.Enabled),
		"features":                      featuresValues(conf.Features),
		"customDomains":                 customDomains,
		"oauth":                         oauthValues(conf.OAuth),
		"signUp":                        signUpValues(conf.SignUp),
		"managedServiceWorkspacePlanId": planID,
		"otel":                          otelValues(conf.TelemetryExport),
	}
}

// resolvedValues are the values that resolve images or check the config against other keys,
// and so can fail.
func resolvedValues(
	conf files.CodesphereConfig,
	imgs images.Images,
	gateways cluster.Gateways,
	metalLB *files.MetalLBConfig,
	registry *files.RegistryConfig,
) (map[string]any, error) {
	deployConfig, err := deployConfigValues(conf, registry, imgs)
	if err != nil {
		return nil, err
	}

	workspaceImages, err := workspaceImagesValues(conf.WorkspaceImages, imgs)
	if err != nil {
		return nil, err
	}

	frontendGateway, err := frontendGatewayValues(
		imgs,
		conf.CertIssuer,
		gateways.Codesphere.Enabled,
	)
	if err != nil {
		return nil, err
	}

	ipService, err := ipServiceValues(conf.IPService, metalLB)
	if err != nil {
		return nil, err
	}

	managedServices, err := managedServicesValues(conf.ManagedServices)
	if err != nil {
		return nil, err
	}

	values := map[string]any{
		"deployConfig":    deployConfig,
		"workspaceImages": workspaceImages,
		"frontendGateway": frontendGateway,
	}
	if ipService != nil {
		values["ipService"] = ipService
	}

	if managedServices != nil {
		values["managedServices"] = managedServices
	}

	return values, nil
}

// optionalValues are the values the installer passes through only when the config has them.
func optionalValues(conf files.CodesphereConfig) map[string]any {
	optional := map[string]any{
		"branding":              brandingValues(conf.Branding),
		"tlsCertificate":        customCertificateValues(conf.CustomCertificate),
		"underprovisionFactors": underprovisionFactorsValues(conf.UnderprovisionFactors),
		"extraWorkspaceFiles":   extraWorkspaceFilesValues(conf.ExtraWorkspaceFiles),
		"smtp":                  smtpValues(conf.Email),
		"openBao":               openBaoValues(conf.OpenBao),
		"openFga":               openFgaValues(conf.OpenFga),
	}
	if conf.ExtraCAPem != "" {
		optional["extraCaPem"] = conf.ExtraCAPem
	}

	if conf.ExtraWorkspaceEnvVars != nil {
		// As a plain map, so the override merges into it key by key.
		envVars := make(map[string]any, len(conf.ExtraWorkspaceEnvVars))
		for name, value := range conf.ExtraWorkspaceEnvVars {
			envVars[name] = value
		}

		optional["extraWorkspaceEnvVars"] = envVars
	}

	for key, value := range optional {
		if value == nil {
			delete(optional, key)
		}
	}

	return optional
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

func customCertificateValues(certificate *files.CustomCertificate) any {
	if certificate == nil {
		return nil
	}

	return map[string]any{"cert": certificate.Cert, "key": certificate.Key}
}

func underprovisionFactorsValues(factors *files.UnderprovisionFactors) any {
	if factors == nil {
		return nil
	}

	return map[string]any{"cpu": factors.CPU, "memory": factors.Memory}
}

func extraWorkspaceFilesValues(workspaceFiles []files.ExtraWorkspaceFile) any {
	if workspaceFiles == nil {
		return nil
	}

	values := make([]any, len(workspaceFiles))
	for i, file := range workspaceFiles {
		values[i] = map[string]any{
			"name":    file.Name,
			"content": file.Content,
			"encoded": file.Encoded,
		}
	}

	return values
}

func boolOr(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}

	return *value
}

func intOr(value *int, fallback int) int {
	if value == nil {
		return fallback
	}

	return *value
}

// omitEmpty drops the empty strings from values, like JSON drops the installer's undefined fields.
func omitEmpty(values map[string]any) map[string]any {
	for key, value := range values {
		if value == "" {
			delete(values, key)
		}
	}

	return values
}
