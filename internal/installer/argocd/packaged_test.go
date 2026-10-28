// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package argocd_test

import (
	"context"
	"errors"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/argocd"
	"github.com/codesphere-cloud/oms/internal/installer/bom"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
)

var _ = Describe("Packaged ArgoCD installation", func() {
	var (
		helm      *installer.MockHelmClient
		resources *argocd.MockResources
		loader    *argocd.MockPackagedChartLoader
		a         *argocd.Installer
	)

	BeforeEach(func() {
		helm = installer.NewMockHelmClient(GinkgoT())
		resources = argocd.NewMockResources(GinkgoT())
		loader = argocd.NewMockPackagedChartLoader(GinkgoT())
		a = &argocd.Installer{
			InstallerConfig: argocd.InstallerConfig{PackageChart: "/package/argocd/files/chart", Version: "2.3.0", FullInstall: true},
			Helm:            helm, Resources: resources, PackageLoader: loader,
		}
	})

	expectExisting := func(chart, version string) {
		helm.EXPECT().FindRelease("argocd", "argocd").Return(&installer.ReleaseInfo{Name: "argocd", ChartName: chart, InstalledVersion: version}, nil)
	}
	expectChartValues := func(overrides map[string]any) {
		loader.EXPECT().LoadChart(a.PackageChart, a.Version).Return(map[string]any{
			"argo-cd": map[string]any{
				"global": map[string]any{"image": map[string]any{"repository": "ghcr.io/codesphere-cloud/docker/quay.io/argoproj/argocd"}},
				"dex":    map[string]any{"image": map[string]any{"repository": "ghcr.io/codesphere-cloud/docker/ghcr.io/dexidp/dex"}},
				"redis":  map[string]any{"image": map[string]any{"repository": "ghcr.io/codesphere-cloud/docker/redis"}},
			},
		}, nil)
		loader.EXPECT().LoadValues(a.ValueFiles).Return(overrides, nil)
	}
	expectResources := func() { resources.EXPECT().ApplyAll(mock.Anything).Return(nil) }
	expectFresh := func() {
		helm.EXPECT().FindRelease("argocd", "argocd").Return(nil, nil)
	}

	It("upgrades an older Codesphere chart using the local package", func() {
		expectExisting("argocd", "2.2.0")
		expectChartValues(nil)
		helm.EXPECT().UpgradeChart(mock.Anything, mock.MatchedBy(func(c installer.ChartConfig) bool {
			return c.ChartName == a.PackageChart && c.Version == "2.3.0" && c.RepoURL == ""
		}), mock.Anything).Return(nil)
		expectResources()
		Expect(a.Install()).To(Succeed())
	})

	It("retains the same Codesphere chart version and still configures credentials", func() {
		expectExisting("argocd", "2.3.0")
		expectChartValues(nil)
		expectResources()
		Expect(a.Install()).To(Succeed())
	})

	It("retains a newer Codesphere chart without downgrading", func() {
		expectExisting("argocd", "2.4.0")
		expectChartValues(nil)
		expectResources()
		Expect(a.Install()).To(Succeed())
	})

	It("preserves an upstream chart without comparing unrelated chart versions", func() {
		a.OciPassword = "registry-token"

		expectExisting("argo-cd", "10.3.2")
		expectChartValues(nil)
		expectResources()
		Expect(a.Install()).To(Succeed())
	})

	It("preserves a release whose chart identity is unknown", func() {
		expectExisting("", "unparseable")
		expectChartValues(nil)
		expectResources()
		Expect(a.Install()).To(Succeed())
	})

	It("installs the local chart with nested bootstrap settings and mirrored images", func() {
		expectFresh()

		a.ImageRegistry = "mirror.example.com"

		expectChartValues(nil)
		helm.EXPECT().InstallChart(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, c installer.ChartConfig, _ installer.InstallChartOptions) error {
			Expect(c.ChartName).To(Equal(a.PackageChart))
			Expect(c.RepoURL).To(BeEmpty())
			Expect(c.Values["extraSecrets"]).To(BeEmpty())
			upstream := c.Values["argo-cd"].(map[string]any)
			Expect(upstream["dex"].(map[string]any)["enabled"]).To(BeFalse())
			image := upstream["global"].(map[string]any)["image"].(map[string]any)
			Expect(image["repository"]).To(Equal("mirror.example.com/ghcr_io/codesphere-cloud/docker/quay.io/argoproj/argocd"))

			return nil
		})
		expectResources()
		Expect(a.Install()).To(Succeed())
	})

	It("propagates chart loading or validation failures without installing", func() {
		loader.EXPECT().LoadValues(a.ValueFiles).Return(nil, nil)
		loader.EXPECT().LoadChart(a.PackageChart, a.Version).Return(nil, errors.New("chart does not match BOM"))
		Expect(a.Install()).To(MatchError(ContainSubstring("does not match BOM")))
	})

	It("lets explicit wrapper values override bootstrap defaults", func() {
		expectFresh()

		a.ImageRegistry = "mirror.example.com"
		a.ValueFiles = []string{"custom-values.yaml"}

		expectChartValues(map[string]any{"argo-cd": map[string]any{
			"server": map[string]any{"replicas": 2},
			"global": map[string]any{"image": map[string]any{"repository": "custom.example.com/argocd"}},
		}})
		helm.EXPECT().InstallChart(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, c installer.ChartConfig, _ installer.InstallChartOptions) error {
			upstream := c.Values["argo-cd"].(map[string]any)
			Expect(upstream["server"].(map[string]any)["replicas"]).To(Equal(2))
			Expect(upstream["global"].(map[string]any)["image"].(map[string]any)["repository"]).To(Equal("custom.example.com/argocd"))

			return nil
		})
		expectResources()
		Expect(a.Install()).To(Succeed())
	})
	It("uses root values for an explicit upstream installation", func() {
		a.PackageChart = ""
		a.UseUpstream = true
		a.RepoURL = argocd.DefaultRepoURL

		expectFresh()
		loader.EXPECT().LoadValues(a.ValueFiles).Return(map[string]any{"server": map[string]any{"replicas": 2}}, nil)
		helm.EXPECT().InstallChart(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, c installer.ChartConfig, _ installer.InstallChartOptions) error {
			Expect(c.ChartName).To(Equal("argo-cd"))
			Expect(c.RepoURL).To(Equal(argocd.DefaultRepoURL))
			Expect(c.Values).NotTo(HaveKey("argo-cd"))
			Expect(c.Values["server"]).To(Equal(map[string]any{"replicas": 2}))

			return nil
		})
		expectResources()
		Expect(a.Install()).To(Succeed())
	})
})

var _ = Describe("Codesphere ArgoCD chart selection", func() {
	var config *bom.Config

	BeforeEach(func() {
		config = &bom.Config{Components: map[string]bom.ComponentConfig{
			"argocd": {Files: map[string]bom.FileRef{"chart": {OciRef: "ghcr.io/codesphere-cloud/charts/argocd:2.3.0"}}},
		}}
	})

	It("uses the BOM chart when no version override is given", func() {
		cfg, err := argocd.CodesphereChartConfig(config, "/package/chart", "", argocd.DefaultRepoURL)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.PackageChart).To(Equal("/package/chart"))
		Expect(cfg.Version).To(Equal("2.3.0"))
	})

	It("selects upstream for an explicit version even if it matches the wrapper version", func() {
		cfg, err := argocd.CodesphereChartConfig(config, "/package/chart", "2.3.0", argocd.DefaultRepoURL)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.PackageChart).To(BeEmpty())
		Expect(cfg.Version).To(Equal("2.3.0"))
		Expect(cfg.RepoURL).To(Equal(argocd.DefaultRepoURL))
	})

	It("accepts a custom repository with an explicit upstream version without an ArgoCD BOM entry", func() {
		cfg, err := argocd.CodesphereChartConfig(&bom.Config{}, "/package/chart", "10.3.2", "oci://registry.example.com/helm")
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.PackageChart).To(BeEmpty())
		Expect(cfg.Version).To(Equal("10.3.2"))
		Expect(cfg.RepoURL).To(Equal("oci://registry.example.com/helm"))
	})

	It("uses the official repository when an upstream version has no repository override", func() {
		cfg, err := argocd.CodesphereChartConfig(&bom.Config{}, "/package/chart", "10.3.2", "")
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.UseUpstream).To(BeTrue())
		Expect(cfg.RepoURL).To(Equal(argocd.DefaultRepoURL))
	})

	It("rejects a repository override without an upstream version", func() {
		_, err := argocd.CodesphereChartConfig(config, "/package/chart", "", "https://custom.example.com")
		Expect(err).To(MatchError(ContainSubstring("requires --argo-version")))
	})

	It("requires the packaged chart reference when no upstream version is given", func() {
		_, err := argocd.CodesphereChartConfig(&bom.Config{}, "/package/chart", "", "")
		Expect(err).To(MatchError(ContainSubstring("no tagged argocd chart")))
	})
})
