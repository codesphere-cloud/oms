// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codespherevalues

import (
	"fmt"
	"slices"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// The functions here return nil, not an empty map, when the config leaves a section out, so the
// values leave it out too.

func sshBaseDomain(proxy *files.SSHWorkspaceProxyConfig) string {
	if proxy == nil {
		return ""
	}

	return proxy.BaseDomain
}

func brandingValues(branding *files.BrandingConfig) any {
	if branding == nil {
		return nil
	}

	return omitEmpty(map[string]any{
		"desc":           branding.Desc,
		"docsUrl":        branding.DocsURL,
		"faviconHref":    branding.FaviconHref,
		"showOnboarding": boolOr(branding.ShowOnboarding, false),
		"title":          branding.Title,
		"tosUrl":         branding.TosURL,
	})
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
		values[i] = map[string]any{"name": file.Name, "content": file.Content, "encoded": file.Encoded}
	}

	return values
}

func smtpValues(email *files.EmailConfig) any {
	if email == nil {
		return nil
	}

	smtp := map[string]any{"host": email.SMTP.Host, "port": email.SMTP.Port, "secure": email.SMTP.Secure}
	if email.SMTP.Username != "" {
		smtp["username"] = email.SMTP.Username
	}

	return smtp
}

func openBaoValues(openBao *files.OpenBaoConfig) any {
	if openBao == nil {
		return nil
	}

	return map[string]any{"engine": openBao.Engine, "uri": openBao.URI, "user": openBao.User}
}

// openFgaValues keeps the chart's in-cluster default unless the config points this data center at
// another OpenFGA, as every data center that does not deploy one does.
func openFgaValues(openFga *files.OpenFgaConfig) any {
	if openFga == nil || openFga.APIURL == "" {
		return nil
	}

	return map[string]any{"apiUrl": openFga.APIURL}
}

// ipServiceValues returns the configured IP service, or one using all MetalLB pools when MetalLB is
// enabled and nothing is configured.
func ipServiceValues(ipService *files.IPServiceConfig, metalLB *files.MetalLBConfig) (map[string]any, error) {
	metalLBEnabled := metalLB != nil && metalLB.Enabled
	poolNames := []string{}

	if metalLBEnabled {
		for _, pool := range metalLB.Pools {
			poolNames = append(poolNames, pool.Name)
		}
	}

	if ipService == nil {
		if !metalLBEnabled {
			return nil, nil
		}

		return map[string]any{"loadBalancerKind": "metallb", "addressPools": poolNames}, nil
	}

	if metalLBEnabled && ipService.LoadBalancerKind != "metallb" {
		return nil, fmt.Errorf("if metallb is enabled in the cluster config, codesphere.ipService must also use metallb")
	}

	if metalLBEnabled {
		for _, pool := range ipService.AddressPools {
			if !slices.Contains(poolNames, pool) {
				return nil, fmt.Errorf("pool %s is not defined in the metallb config (%v)", pool, poolNames)
			}
		}
	}

	values := map[string]any{"loadBalancerKind": ipService.LoadBalancerKind}
	if ipService.AddressPools != nil {
		values["addressPools"] = ipService.AddressPools
	}

	return values, nil
}
