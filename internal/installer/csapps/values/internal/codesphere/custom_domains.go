// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// CustomDomains are the settings of the workspaces' custom domains.
type CustomDomains struct {
	CNameBaseDomain                 string                         `yaml:"cNameBaseDomain"`
	GatewayAPI                      CustomDomainsGatewayAPI        `yaml:"gatewayApi"`
	InsecureSkipDomainControlVerify *bool                          `yaml:"insecureSkipDomainControlVerify,omitempty"`
	Cert                            *files.CustomDomainsCertConfig `yaml:"cert,omitempty"`
}

// CustomDomainsGatewayAPI says whether custom domains are served through the Gateway API.
type CustomDomainsGatewayAPI struct {
	UseGateway bool `yaml:"useGateway"`
}

func customDomainsValues(conf files.CustomDomainsConfig, useGateway bool) CustomDomains {
	return CustomDomains{
		CNameBaseDomain:                 conf.CNameBaseDomain,
		GatewayAPI:                      CustomDomainsGatewayAPI{UseGateway: useGateway},
		InsecureSkipDomainControlVerify: conf.InsecureSkipDomainControlVerify,
		Cert:                            conf.Cert,
	}
}
