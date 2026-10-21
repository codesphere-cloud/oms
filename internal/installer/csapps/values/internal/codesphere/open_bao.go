// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

func openBaoValues(openBao *files.OpenBaoConfig) any {
	if openBao == nil {
		return nil
	}

	return map[string]any{"engine": openBao.Engine, "uri": openBao.URI, "user": openBao.User}
}
