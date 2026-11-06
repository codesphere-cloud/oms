// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps_test

import (
	"github.com/codesphere-cloud/oms/internal/installer/argocd/pcapps"
	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ClusterConfigPcAppsValues", func() {
	var (
		config     files.RootConfig
		vault      *files.InstallVault
		packageBOM *bom.Config
	)

	BeforeEach(func() {
		config = files.RootConfig{Registry: &files.RegistryConfig{Server: "mirror.example"}}
		vault = &files.InstallVault{Secrets: []files.SecretEntry{
			{Name: files.SecretRegistryUsername, Fields: &files.SecretFields{Password: "registry-user"}},
			{Name: files.SecretRegistryPassword, Fields: &files.SecretFields{Password: "registry-password"}},
		}}
		packageBOM = &bom.Config{Components: map[string]bom.ComponentConfig{"pc-applications": {Files: map[string]bom.FileRef{"chart": {OciRef: "ghcr.io/codesphere-cloud/charts/pc-applications:0.374.0"}}}}}
	})
	It("keeps the TS installer in charge for older packages", func() {
		packageBOM.Components["cluster-config"] = bom.ComponentConfig{Files: map[string]bom.FileRef{"chart": {OciRef: "ghcr.io/codesphere-cloud/charts/cluster-config:1.9.0"}}}
		values, err := pcapps.ClusterConfigPcAppsValues(&config, nil, packageBOM)
		Expect(err).NotTo(HaveOccurred())
		Expect(values).To(BeNil())
	})
	It("enables Argo when the packaged BOM retains only cluster-config images", func() {
		packageBOM.Components["cluster-config"] = bom.ComponentConfig{ContainerImages: map[string]string{"reflector": "ghcr.io/codesphere-cloud/docker/emberstack/kubernetes-reflector:10.0.60"}}
		values, err := pcapps.ClusterConfigPcAppsValues(&config, vault, packageBOM)
		Expect(err).NotTo(HaveOccurred())
		Expect(values).To(Equal(files.ChartValues{"applications": files.ChartValues{"cluster-config": files.ChartValues{
			"enabled": true, "valuesObject": files.ChartValues{"registryCredentials": files.ChartValues{"registry": "mirror.example", "username": "registry-user", "password": "registry-password"}},
		}}}))
	})
	It("supplies mirror credentials when dependency expansion retains the Argo-managed chart", func() {
		packageBOM.Components["pc-applications"] = bom.ComponentConfig{Files: map[string]bom.FileRef{"chart": {OciRef: "ghcr.io/codesphere-cloud/charts/pc-applications:0.385.0"}}}
		packageBOM.Components["cluster-config"] = bom.ComponentConfig{Files: map[string]bom.FileRef{"chart": {OciRef: "ghcr.io/codesphere-cloud/charts/cluster-config:1.16.7"}}}
		values, err := pcapps.ClusterConfigPcAppsValues(&config, vault, packageBOM)
		Expect(err).NotTo(HaveOccurred())

		app := values["applications"].(files.ChartValues)["cluster-config"].(files.ChartValues)
		Expect(app["enabled"]).To(BeTrue())
		credentials := app["valuesObject"].(files.ChartValues)["registryCredentials"]
		Expect(credentials).To(Equal(files.ChartValues{"registry": "mirror.example", "username": "registry-user", "password": "registry-password"}))
	})

	It("preserves component overrides and lets explicit pcApps values win", func() {
		config.Cluster.ClusterConfig = &files.ChartOverrideConfig{Override: files.ChartOverride{"registryCredentials": map[string]any{"username": "legacy-override"}, "workspacesNamespace": "custom-workspaces"}}
		values, err := pcapps.ClusterConfigPcAppsValues(&config, vault, packageBOM)
		Expect(err).NotTo(HaveOccurred())

		app := values["applications"].(files.ChartValues)["cluster-config"].(files.ChartValues)
		chart := app["valuesObject"].(files.ChartValues)
		Expect(chart["workspacesNamespace"]).To(Equal("custom-workspaces"))
		Expect(chart["registryCredentials"].(files.ChartValues)["username"]).To(Equal("legacy-override"))

		explicit := files.ChartValues{"applications": files.ChartValues{"cluster-config": files.ChartValues{"enabled": false}}}
		merged := util.DeepMergeMaps(values, explicit)
		Expect(merged["applications"].(files.ChartValues)["cluster-config"].(files.ChartValues)["enabled"]).To(BeFalse())
	})
	It("rejects a missing registry password without exposing the registry username", func() {
		vault.GetSecret(files.SecretRegistryPassword).Fields.Password = ""
		_, err := pcapps.ClusterConfigPcAppsValues(&config, vault, packageBOM)
		Expect(err).To(MatchError(ContainSubstring("registry password is required")))
		Expect(err.Error()).NotTo(ContainSubstring("registry-user"))
	})
})
