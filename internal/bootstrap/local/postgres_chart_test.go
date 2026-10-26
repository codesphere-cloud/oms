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
	It("uses the default registry and supplied password with the BOM chart name and version", func() {
		helm := installer.NewMockHelmClient(GinkgoT())
		helm.EXPECT().LoginRegistry(mock.Anything, "ghcr.io", "user", "provided-password").Return(nil)
		helm.EXPECT().FindRelease(codesphereNamespace, cnpgReleaseName).Return(nil, nil)
		helm.EXPECT().InstallChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
			return cfg.ChartName == "oci://ghcr.io/codesphere-cloud/charts/postgres-operator" && cfg.Version == "1.2.3"
		}), installer.InstallChartOptions{}).Return(nil)
		b := &LocalBootstrapper{
			ctx: context.Background(), helmClient: helm,
			Env: &CodesphereEnvironment{RegistryUser: "user", RegistryPassword: "provided-password"},
			installerBOM: &bom.Config{Components: map[string]bom.ComponentConfig{
				cnpgBOMComponent: {Files: map[string]bom.FileRef{
					"chart": {OciRef: "oci://bom-registry.example.com/team/charts/postgres-operator:1.2.3"},
				}},
			}},
		}
		Expect(b.InstallCloudNativePGHelmChart()).To(Succeed())
	})

	It("uses the configured registry with the BOM chart name and version", func() {
		helm := installer.NewMockHelmClient(GinkgoT())
		helm.EXPECT().LoginRegistry(mock.Anything, "registry.example.com", "user", "password").Return(nil)
		helm.EXPECT().FindRelease(codesphereNamespace, cnpgReleaseName).Return(nil, nil)
		helm.EXPECT().InstallChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
			return cfg.ChartName == "oci://registry.example.com/team/codesphere-cloud/charts/postgres-operator" &&
				cfg.Version == "1.2.3" && cfg.Namespace == codesphereNamespace
		}), installer.InstallChartOptions{}).Return(nil)
		b := &LocalBootstrapper{
			ctx: context.Background(), helmClient: helm,
			Env: &CodesphereEnvironment{
				RegistryUser: "user", RegistryPassword: "password",
				RegistryURL: "registry.example.com/team/",
			},
			installerBOM: &bom.Config{Components: map[string]bom.ComponentConfig{
				cnpgBOMComponent: {Files: map[string]bom.FileRef{
					"chart": {OciRef: "oci://ghcr.io/codesphere-cloud/charts/postgres-operator:1.2.3"},
				}},
			}},
		}
		Expect(b.InstallCloudNativePGHelmChart()).To(Succeed())
	})

	It("upgrades from a registry URL without the OCI scheme", func() {
		helm := installer.NewMockHelmClient(GinkgoT())
		helm.EXPECT().LoginRegistry(mock.Anything, "registry.example.com", "user", "password").Return(nil)
		helm.EXPECT().FindRelease(codesphereNamespace, cnpgReleaseName).Return(&installer.ReleaseInfo{Name: cnpgReleaseName}, nil)
		helm.EXPECT().UpgradeChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
			return cfg.ChartName == "oci://registry.example.com/custom/codesphere-cloud/charts/postgres-operator" &&
				cfg.Version == "1.2.3" && cfg.Namespace == codesphereNamespace
		}), installer.UpgradeChartOptions{}).Return(nil)
		b := &LocalBootstrapper{
			ctx: context.Background(), helmClient: helm,
			Env: &CodesphereEnvironment{
				RegistryUser: "user", RegistryPassword: "password",
				RegistryURL: "registry.example.com/custom",
			},
			installerBOM: &bom.Config{Components: map[string]bom.ComponentConfig{
				cnpgBOMComponent: {Files: map[string]bom.FileRef{
					"chart": {OciRef: "oci://ghcr.io/codesphere-cloud/charts/postgres-operator:1.2.3"},
				}},
			}},
		}
		Expect(b.InstallCloudNativePGHelmChart()).To(Succeed())
	})
})
