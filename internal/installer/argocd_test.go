// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	"errors"

	"github.com/codesphere-cloud/oms/internal/installer"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
)

var _ = Describe("ArgoCD.Install", func() {

	var (
		helmMock            *installer.MockHelmClient
		argoCDResourcesMock *installer.MockArgoCDResources
		a                   *installer.ArgoCD
	)

	BeforeEach(func() {
		helmMock = installer.NewMockHelmClient(GinkgoT())
		argoCDResourcesMock = installer.NewMockArgoCDResources(GinkgoT())
		a = &installer.ArgoCD{Version: "7.0.0", Helm: helmMock, Resources: argoCDResourcesMock}
	})

	Context("when no existing release is found", func() {
		BeforeEach(func() {
			helmMock.EXPECT().FindRelease("argocd", "argocd").Return(nil, nil)
		})

		It("performs a fresh install with a specific version", func() {
			helmMock.EXPECT().InstallChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
				return cfg.Version == "7.0.0" &&
					cfg.ReleaseName == "argocd" &&
					cfg.Namespace == "argocd" &&
					cfg.CreateNamespace == true
			})).Return(nil)

			err := a.Install()
			Expect(err).ToNot(HaveOccurred())
		})

		It("performs a fresh install with latest version when Version is empty", func() {
			helmMock.EXPECT().FindRelease("argocd", "argocd").Return(nil, nil)
			helmMock.EXPECT().InstallChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
				return cfg.Version == ""
			})).Return(nil)

			a = &installer.ArgoCD{Version: "", Helm: helmMock}

			err := a.Install()
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error when InstallChart fails", func() {
			helmMock.EXPECT().InstallChart(mock.Anything, mock.Anything).
				Return(errors.New("chart not found"))

			err := a.Install()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("chart not found"))
		})
	})

	Context("when an existing release is found", func() {
		BeforeEach(func() {
			helmMock.EXPECT().FindRelease("argocd", "argocd").Return(&installer.ReleaseInfo{
				Name:             "argocd",
				InstalledVersion: "6.0.0",
			}, nil)
		})

		It("upgrades to a newer version", func() {

			helmMock.EXPECT().UpgradeChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
				return cfg.Version == "7.0.0"
			}), installer.UpgradeChartOptions{InstallIfNotExist: false}).Return(nil)

			err := a.Install()
			Expect(err).ToNot(HaveOccurred())
		})

		It("upgrades to the same version (no-op upgrade)", func() {
			helmMock.EXPECT().UpgradeChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
				return cfg.Version == "6.0.0"
			}), installer.UpgradeChartOptions{InstallIfNotExist: false}).Return(nil)

			a = &installer.ArgoCD{Version: "6.0.0", Helm: helmMock}

			err := a.Install()
			Expect(err).ToNot(HaveOccurred())
		})

		It("upgrades to latest when Version is empty", func() {
			helmMock.EXPECT().UpgradeChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
				return cfg.Version == ""
			}), installer.UpgradeChartOptions{InstallIfNotExist: false}).Return(nil)

			a = &installer.ArgoCD{Version: "", Helm: helmMock}

			err := a.Install()
			Expect(err).ToNot(HaveOccurred())
		})

		It("rejects a downgrade", func() {
			a = &installer.ArgoCD{Version: "5.0.0", Helm: helmMock}

			err := a.Install()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("downgrade is not allowed"))
		})

		It("returns an error when UpgradeChart fails", func() {
			helmMock.EXPECT().UpgradeChart(mock.Anything, mock.Anything, mock.Anything).
				Return(errors.New("timeout waiting for condition"))

			a = &installer.ArgoCD{Version: "7.0.0", Helm: helmMock}

			err := a.Install()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("timeout"))
		})
	})

	Context("when FindRelease returns an error", func() {
		It("propagates the error without installing or upgrading", func() {
			helmMock.EXPECT().FindRelease("argocd", "argocd").
				Return(nil, errors.New("cluster unreachable"))

			a = &installer.ArgoCD{Version: "7.0.0", Helm: helmMock}

			err := a.Install()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cluster unreachable"))
		})
	})

	Context("chart configuration", func() {
		It("always uses the correct chart name and repo URL", func() {
			helmMock.EXPECT().FindRelease("argocd", "argocd").Return(nil, nil)
			helmMock.EXPECT().InstallChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
				return cfg.ChartName == "argo-cd" &&
					cfg.RepoURL == "https://argoproj.github.io/argo-helm"
			})).Return(nil)

			a = &installer.ArgoCD{Version: "7.0.0", Helm: helmMock}

			err := a.Install()
			Expect(err).ToNot(HaveOccurred())
		})

		It("disables dex in the values", func() {
			helmMock.EXPECT().FindRelease("argocd", "argocd").Return(nil, nil)
			helmMock.EXPECT().InstallChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
				dex, ok := cfg.Values["dex"].(map[string]interface{})
				return ok && dex["enabled"] == false
			})).Return(nil)

			a = &installer.ArgoCD{Version: "7.0.0", Helm: helmMock}

			err := a.Install()
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("full installation", func() {
		BeforeEach(func() {
			helmMock.EXPECT().FindRelease("argocd", "argocd").Return(nil, nil)
			helmMock.EXPECT().InstallChart(mock.Anything, mock.Anything).Return(nil)
		})
		It("installs extra ArgoCD resources when FullInstall option in true", func() {
			argoCDResourcesMock.EXPECT().ApplyAll(mock.Anything).Return(nil)
			a.FullInstall = true

			err := a.Install()
			Expect(err).ToNot(HaveOccurred())
		})
		It("does not install extra ArgoCD resources when FullInstall option in false", func() {
			a.FullInstall = false

			err := a.Install()
			Expect(err).ToNot(HaveOccurred())
		})
	})

})
