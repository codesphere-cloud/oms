// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// OpenFGA is the OpenFGA of another data center this one uses.
type OpenFGA struct {
	APIURL string `yaml:"apiUrl"`
}

// openFgaValues keeps the chart's in-cluster default unless the config points this data center at
// another OpenFGA, as every data center that does not deploy one does.
func openFgaValues(openFga *files.OpenFgaConfig) *OpenFGA {
	if openFga == nil || openFga.APIURL == "" {
		return nil
	}

	return &OpenFGA{APIURL: openFga.APIURL}
}
