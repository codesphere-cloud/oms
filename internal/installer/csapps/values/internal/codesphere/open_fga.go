// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// openFgaValues keeps the chart's in-cluster default unless the config points this data center at
// another OpenFGA, as every data center that does not deploy one does.
func openFgaValues(openFga *files.OpenFgaConfig) any {
	if openFga == nil || openFga.APIURL == "" {
		return nil
	}

	return map[string]any{"apiUrl": openFga.APIURL}
}
