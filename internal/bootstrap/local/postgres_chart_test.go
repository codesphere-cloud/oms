// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"testing"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/stretchr/testify/mock"
)

func TestCloudNativePGChartUsesBOMRepository(t *testing.T) {
	helm := installer.NewMockHelmClient(t)
	helm.EXPECT().LoginRegistry(mock.Anything, "registry.example.com", "user", "password").Return(nil)
	helm.EXPECT().FindRelease(codesphereNamespace, cnpgReleaseName).Return(nil, nil)
	helm.EXPECT().InstallChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
		return cfg.ChartName == "oci://registry.example.com/team/charts/cloudnative-pg" &&
			cfg.Version == "1.2.3" && cfg.Namespace == codesphereNamespace
	}), installer.InstallChartOptions{}).Return(nil)
	b := &LocalBootstrapper{
		ctx: context.Background(), helmClient: helm,
		Env: &CodesphereEnvironment{RegistryUser: "user", RegistryPassword: "password"},
		installerBOM: &bom.Config{Components: map[string]bom.ComponentConfig{
			cnpgBOMComponent: {Files: map[string]bom.FileRef{
				"chart": {OciRef: "oci://registry.example.com/team/charts/cloudnative-pg:1.2.3"},
			}},
		}},
	}
	if err := b.InstallCloudNativePGHelmChart(); err != nil {
		t.Fatal(err)
	}
}
