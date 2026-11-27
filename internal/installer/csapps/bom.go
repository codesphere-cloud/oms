// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package csapps

import (
	"fmt"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/bom"
)

// ReadBOM reads the package BOM and checks it has the charts Install deploys.
func ReadBOM(pm installer.PackageManager, force, verbose bool) (*bom.Config, error) {
	if err := pm.ExtractDependency(bom.Filename, force, verbose); err != nil {
		return nil, fmt.Errorf("failed to extract the package BOM: %w", err)
	}

	csBom, err := bom.Parse(pm.GetDependencyPath(bom.Filename))
	if err != nil {
		return nil, fmt.Errorf("failed to read the package BOM: %w", err)
	}

	for _, component := range []string{csAppsComponent, codesphereComponent} {
		if _, ok := csBom.GetChart(component); !ok {
			return nil, fmt.Errorf("%s chart not found in the package BOM", component)
		}
	}

	return csBom, nil
}
