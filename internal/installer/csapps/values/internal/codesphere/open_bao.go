// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// OpenBao is the OpenBao Codesphere reads secrets from.
type OpenBao struct {
	Engine string `yaml:"engine"`
	URI    string `yaml:"uri"`
	User   string `yaml:"user"`
}

func openBaoValues(openBao *files.OpenBaoConfig) *OpenBao {
	if openBao == nil {
		return nil
	}

	return &OpenBao{Engine: openBao.Engine, URI: openBao.URI, User: openBao.User}
}
