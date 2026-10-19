// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/bom"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
)

var _ = Describe("CloudNativePG chart", func() {
	It("uses the BOM repository", func() {
		helm := installer.NewMockHelmClient(GinkgoT())
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
		Expect(b.InstallCloudNativePGHelmChart()).To(Succeed())
	})
})
