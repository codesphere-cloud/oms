// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package argocd_test

import (
	"errors"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/argocd"
	"github.com/codesphere-cloud/oms/internal/installer/bom"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
)

var _ = Describe("ArgoCD chart from BOM", func() {
	var config *bom.Config

	BeforeEach(func() {
		config = &bom.Config{Components: map[string]bom.ComponentConfig{
			"argocd": {Files: map[string]bom.FileRef{"chart": {OciRef: "oci://registry.example.com/charts/argocd:2.3.4"}}},
		}}
	})

	It("installs the exact chart and version from the BOM with registry credentials", func() {
		cfg, err := argocd.ConfigFromBOM(config, argocd.InstallerConfig{OciPassword: "token"})
		Expect(err).NotTo(HaveOccurred())

		helm := installer.NewMockHelmClient(GinkgoT())
		helm.EXPECT().LoginRegistry(mock.Anything, "registry.example.com", "github", "token").Return(nil)
		helm.EXPECT().FindRelease("argocd", "argocd").Return(nil, nil)
		helm.EXPECT().InstallChart(mock.Anything, mock.MatchedBy(func(chart installer.ChartConfig) bool {
			return chart.ChartName == "oci://registry.example.com/charts/argocd" && chart.Version == "2.3.4" && chart.RepoURL == ""
		}), mock.Anything).Return(nil)
		install := &argocd.Installer{InstallerConfig: cfg, Helm: helm}
		Expect(install.Install()).To(Succeed())
	})

	It("takes the repository and version from the BOM while preserving other settings", func() {
		cfg, err := argocd.ConfigFromBOM(config, argocd.InstallerConfig{
			RepoURL: "https://charts.example.com", Version: "3.0.0", ForceConflicts: true,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.ChartName).To(Equal("argocd"))
		Expect(cfg.RepoURL).To(Equal("oci://registry.example.com/charts"))
		Expect(cfg.Version).To(Equal("2.3.4"))
		Expect(cfg.ForceConflicts).To(BeTrue())
	})

	It("pulls the BOM chart and version from a configured mirror", func() {
		cfg, err := argocd.ConfigFromBOM(config, argocd.InstallerConfig{OciRegistryURL: "mirror.example.com/codesphere-cloud/charts"})
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.ChartName).To(Equal("argocd"))
		Expect(cfg.RepoURL).To(Equal("oci://mirror.example.com/codesphere-cloud/charts"))
		Expect(cfg.Version).To(Equal("2.3.4"))
	})

	It("accepts an OCI reference without the optional scheme", func() {
		config.Components["argocd"] = bom.ComponentConfig{Files: map[string]bom.FileRef{
			"chart": {OciRef: "registry.example.com/custom/argo-cd:4.5.6"},
		}}
		cfg, err := argocd.ConfigFromBOM(config, argocd.InstallerConfig{})
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.ChartName).To(Equal("argo-cd"))
		Expect(cfg.RepoURL).To(Equal("oci://registry.example.com/custom"))
		Expect(cfg.Version).To(Equal("4.5.6"))
	})

	It("requires a BOM before initializing installer clients", func() {
		_, err := argocd.NewInstaller(argocd.InstallerConfig{}, nil)
		Expect(err).To(MatchError("installer BOM is required"))
	})

	It("requires an ArgoCD BOM entry before initializing installer clients", func() {
		_, err := argocd.NewInstaller(argocd.InstallerConfig{}, &bom.Config{})
		Expect(err).To(MatchError(ContainSubstring("argocd chart")))
	})

	It("rejects a missing BOM", func() {
		_, err := argocd.ConfigFromBOM(nil, argocd.InstallerConfig{})
		Expect(err).To(MatchError("installer BOM is required"))
	})

	It("rejects a missing ArgoCD chart instead of using an upstream default", func() {
		_, err := argocd.ConfigFromBOM(&bom.Config{}, argocd.InstallerConfig{})
		Expect(err).To(MatchError(ContainSubstring("argocd chart")))
	})

	It("rejects an untagged ArgoCD chart", func() {
		config.Components["argocd"] = bom.ComponentConfig{Files: map[string]bom.FileRef{
			"chart": {OciRef: "registry.example.com/charts/argocd"},
		}}
		_, err := argocd.ConfigFromBOM(config, argocd.InstallerConfig{})
		Expect(err).To(MatchError(ContainSubstring("valid tagged OCI reference")))
	})

	It("reports registry login failures before installing", func() {
		cfg, err := argocd.ConfigFromBOM(config, argocd.InstallerConfig{OciUsername: "user", OciPassword: "token"})
		Expect(err).NotTo(HaveOccurred())

		helm := installer.NewMockHelmClient(GinkgoT())
		loginErr := errors.New("invalid credentials")
		helm.EXPECT().LoginRegistry(mock.Anything, "registry.example.com", "user", "token").Return(loginErr)
		install := &argocd.Installer{InstallerConfig: cfg, Helm: helm}
		Expect(install.Install()).To(MatchError(ContainSubstring("logging in to ArgoCD chart registry")))
	})
})
