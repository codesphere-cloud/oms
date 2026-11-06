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

const cnpgOperatorImage = "ghcr.io/cloudnative-pg/cloudnative-pg:1.27.1"

// cnpgBOM returns an installer BOM with the CloudNativePG chart and, unless empty, its operator image.
func cnpgBOM(chartRef, operatorImage string) *bom.Config {
	component := bom.ComponentConfig{Files: map[string]bom.FileRef{"chart": {OciRef: chartRef}}}
	if operatorImage != "" {
		component.ContainerImages = map[string]string{cnpgOperatorImageKey: operatorImage}
	}

	return &bom.Config{Components: map[string]bom.ComponentConfig{cnpgBOMComponent: component}}
}

// upstreamValues returns the values the chart passes to its cloudnative-pg dependency.
func upstreamValues(cfg installer.ChartConfig) map[string]interface{} {
	values, _ := cfg.Values["cloudnative-pg"].(map[string]interface{})

	return values
}

var _ = Describe("CloudNativePG chart", func() {
	It("uses the default registry and supplied password with the BOM chart name and version", func() {
		var installed installer.ChartConfig
		helm := installer.NewMockHelmClient(GinkgoT())
		helm.EXPECT().LoginRegistry(mock.Anything, "ghcr.io", "user", "provided-password").Return(nil)
		helm.EXPECT().FindRelease(codesphereNamespace, cnpgReleaseName).Return(nil, nil)
		helm.EXPECT().InstallChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
			installed = cfg
			return cfg.ChartName == "oci://ghcr.io/codesphere-cloud/charts/postgres-operator" && cfg.Version == "1.2.3"
		}), installer.InstallChartOptions{}).Return(nil)
		b := &LocalBootstrapper{
			ctx: context.Background(), helmClient: helm,
			Env:          &CodesphereEnvironment{RegistryUser: "user", RegistryPassword: "provided-password"},
			installerBOM: cnpgBOM("oci://bom-registry.example.com/team/charts/postgres-operator:1.2.3", ""),
		}
		Expect(b.InstallCloudNativePGHelmChart()).To(Succeed())
		Expect(upstreamValues(installed)).NotTo(HaveKey("image"))
		Expect(upstreamValues(installed)).NotTo(HaveKey("imagePullSecrets"))
	})

	It("pins the BOM operator image from ghcr.io without a pull secret", func() {
		var installed installer.ChartConfig

		helm := installer.NewMockHelmClient(GinkgoT())
		helm.EXPECT().LoginRegistry(mock.Anything, "ghcr.io", "user", "password").Return(nil)
		helm.EXPECT().FindRelease(codesphereNamespace, cnpgReleaseName).Return(nil, nil)
		helm.EXPECT().InstallChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
			installed = cfg
			return true
		}), installer.InstallChartOptions{}).Return(nil)
		b := &LocalBootstrapper{
			ctx: context.Background(), helmClient: helm,
			Env:          &CodesphereEnvironment{RegistryUser: "user", RegistryPassword: "password"},
			installerBOM: cnpgBOM("oci://ghcr.io/codesphere-cloud/charts/postgres-operator:1.2.3", cnpgOperatorImage),
		}
		Expect(b.InstallCloudNativePGHelmChart()).To(Succeed())
		Expect(upstreamValues(installed)).To(HaveKeyWithValue("image", map[string]interface{}{
			"repository": "ghcr.io/cloudnative-pg/cloudnative-pg", "tag": "1.27.1",
		}))
		Expect(upstreamValues(installed)).NotTo(HaveKey("imagePullSecrets"))
		Expect(upstreamValues(installed)).To(HaveKeyWithValue("priorityClassName", ""))
	})

	It("pulls the chart and the operator image from a configured mirror with its pull secret", func() {
		var installed installer.ChartConfig
		helm := installer.NewMockHelmClient(GinkgoT())
		helm.EXPECT().LoginRegistry(mock.Anything, "registry.example.com", "user", "password").Return(nil)
		helm.EXPECT().FindRelease(codesphereNamespace, cnpgReleaseName).Return(nil, nil)
		helm.EXPECT().InstallChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
			installed = cfg
			return cfg.ChartName == "oci://registry.example.com/team/codesphere-cloud/charts/postgres-operator" &&
				cfg.Version == "1.2.3" && cfg.Namespace == codesphereNamespace
		}), installer.InstallChartOptions{}).Return(nil)
		b := &LocalBootstrapper{
			ctx: context.Background(), helmClient: helm,
			Env: &CodesphereEnvironment{
				RegistryUser: "user", RegistryPassword: "password",
				RegistryURL: "registry.example.com/team/",
			},
			installerBOM: cnpgBOM("oci://ghcr.io/codesphere-cloud/charts/postgres-operator:1.2.3", cnpgOperatorImage),
		}
		Expect(b.InstallCloudNativePGHelmChart()).To(Succeed())
		Expect(upstreamValues(installed)).To(HaveKeyWithValue("image", map[string]interface{}{
			"repository": "registry.example.com/team/ghcr_io/cloudnative-pg/cloudnative-pg", "tag": "1.27.1",
		}))
		Expect(upstreamValues(installed)).To(HaveKeyWithValue("imagePullSecrets", []interface{}{
			map[string]interface{}{"name": cnpgPullSecretName},
		}))
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
			installerBOM: cnpgBOM("oci://ghcr.io/codesphere-cloud/charts/postgres-operator:1.2.3", cnpgOperatorImage),
		}
		Expect(b.InstallCloudNativePGHelmChart()).To(Succeed())
	})

	It("refuses a mirror when the BOM does not name the operator image", func() {
		helm := installer.NewMockHelmClient(GinkgoT())
		b := &LocalBootstrapper{
			ctx: context.Background(), helmClient: helm,
			Env: &CodesphereEnvironment{
				RegistryUser: "user", RegistryPassword: "password",
				RegistryURL: "registry.example.com",
			},
			installerBOM: cnpgBOM("oci://ghcr.io/codesphere-cloud/charts/postgres-operator:1.2.3", ""),
		}
		Expect(b.InstallCloudNativePGHelmChart()).To(MatchError(ContainSubstring("operator image \"cloudnativePg\" is missing")))
	})

	It("keeps the digest of the operator image on its tag", func() {
		b := &LocalBootstrapper{
			Env: &CodesphereEnvironment{},
			installerBOM: cnpgBOM("oci://ghcr.io/codesphere-cloud/charts/postgres-operator:1.2.3",
				cnpgOperatorImage+"@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"),
		}
		values, err := b.cnpgOperatorImageValues()
		Expect(err).NotTo(HaveOccurred())
		Expect(values).To(HaveKeyWithValue("image", map[string]interface{}{
			"repository": "ghcr.io/cloudnative-pg/cloudnative-pg",
			"tag":        "1.27.1@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		}))
	})
})
