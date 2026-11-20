// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package argocd

import (
	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// GetPcAppsValues exposes getPcAppsValues to the tests.
func (i *AppInstaller) GetPcAppsValues(bomConfig *bom.Config) (files.ChartValues, error) {
	return i.getPcAppsValues(bomConfig)
}
