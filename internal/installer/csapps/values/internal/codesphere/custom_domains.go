// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

func customDomainsValues(conf files.CustomDomainsConfig, useGateway bool) map[string]any {
	values := map[string]any{
		"cNameBaseDomain": conf.CNameBaseDomain,
		"gatewayApi":      map[string]any{"useGateway": useGateway},
	}
	if conf.InsecureSkipDomainControlVerify != nil {
		values["insecureSkipDomainControlVerify"] = *conf.InsecureSkipDomainControlVerify
	}

	if conf.Cert != nil {
		cert := map[string]any{}
		if conf.Cert.Algorithm != "" {
			cert["algorithm"] = conf.Cert.Algorithm
		}

		if conf.Cert.Size != 0 {
			cert["size"] = conf.Cert.Size
		}

		values["cert"] = cert
	}

	return values
}
