// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/codesphere-cloud/oms/internal/installer"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
)

// writeValuesFile writes content to a temp YAML file and returns its path.
func writeValuesFile(content string) string {
	path := filepath.Join(GinkgoT().TempDir(), "values.yaml")
	Expect(os.WriteFile(path, []byte(content), 0o600)).To(Succeed())
	return path
}

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
			}), mock.Anything).Return(nil)

			err := a.Install()
			Expect(err).ToNot(HaveOccurred())
		})

		It("performs a fresh install with latest version when Version is empty", func() {
			helmMock.EXPECT().FindRelease("argocd", "argocd").Return(nil, nil)
			helmMock.EXPECT().InstallChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
				return cfg.Version == ""
			}), mock.Anything).Return(nil)

			a = &installer.ArgoCD{Version: "", Helm: helmMock}

			err := a.Install()
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error when InstallChart fails", func() {
			helmMock.EXPECT().InstallChart(mock.Anything, mock.Anything, mock.Anything).
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
			}), mock.Anything).Return(nil)

			a = &installer.ArgoCD{Version: "7.0.0", Helm: helmMock}

			err := a.Install()
			Expect(err).ToNot(HaveOccurred())
		})

		It("disables dex in the values", func() {
			helmMock.EXPECT().FindRelease("argocd", "argocd").Return(nil, nil)
			helmMock.EXPECT().InstallChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
				dex, ok := cfg.Values["dex"].(map[string]interface{})
				return ok && dex["enabled"] == false
			}), mock.Anything).Return(nil)

			a = &installer.ArgoCD{Version: "7.0.0", Helm: helmMock}

			err := a.Install()
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("values overrides", func() {
		BeforeEach(func() {
			helmMock.EXPECT().FindRelease("argocd", "argocd").Return(nil, nil)
		})

		It("lets a value file override the dex.enabled default", func() {
			valuesFile := writeValuesFile("dex:\n  enabled: true\n")

			helmMock.EXPECT().InstallChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
				dex, ok := cfg.Values["dex"].(map[string]interface{})
				return ok && dex["enabled"] == true
			}), mock.Anything).Return(nil)

			a = &installer.ArgoCD{Version: "7.0.0", Helm: helmMock, ValueFiles: []string{valuesFile}}

			err := a.Install()
			Expect(err).ToNot(HaveOccurred())
		})

		It("keeps the dex.enabled default when the value file does not set it", func() {
			valuesFile := writeValuesFile("server:\n  replicas: 2\n")

			helmMock.EXPECT().InstallChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
				dex, ok := cfg.Values["dex"].(map[string]interface{})
				if !ok || dex["enabled"] != false {
					return false
				}
				server, ok := cfg.Values["server"].(map[string]interface{})
				return ok && server["replicas"] == float64(2)
			}), mock.Anything).Return(nil)

			a = &installer.ArgoCD{Version: "7.0.0", Helm: helmMock, ValueFiles: []string{valuesFile}}

			err := a.Install()
			Expect(err).ToNot(HaveOccurred())
		})

		It("merges values from multiple files with later files taking precedence", func() {
			first := writeValuesFile("dex:\n  enabled: true\nserver:\n  replicas: 1\n")
			second := writeValuesFile("server:\n  replicas: 3\n")

			helmMock.EXPECT().InstallChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
				dex, ok := cfg.Values["dex"].(map[string]interface{})
				if !ok || dex["enabled"] != true {
					return false
				}
				server, ok := cfg.Values["server"].(map[string]interface{})
				return ok && server["replicas"] == float64(3)
			}), mock.Anything).Return(nil)

			a = &installer.ArgoCD{Version: "7.0.0", Helm: helmMock, ValueFiles: []string{first, second}}

			err := a.Install()
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("RepoURL validation", func() {
		DescribeTable("accepts supported schemes",
			func(repoURL string) {
				helmMock.EXPECT().FindRelease("argocd", "argocd").Return(nil, nil)
				helmMock.EXPECT().InstallChart(mock.Anything, mock.Anything, mock.Anything).Return(nil)

				a = &installer.ArgoCD{Version: "7.0.0", Helm: helmMock, RepoURL: repoURL}

				err := a.Install()
				Expect(err).ToNot(HaveOccurred())
			},
			Entry("empty (uses default)", ""),
			Entry("http", "http://my.repo/helm"),
			Entry("https", "https://my.repo/helm"),
			Entry("oci", "oci://ghcr.io/argoproj/argo-helm"),
		)

		DescribeTable("rejects unsupported schemes without touching helm",
			func(repoURL string) {
				a = &installer.ArgoCD{Version: "7.0.0", Helm: helmMock, RepoURL: repoURL}

				err := a.Install()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("must start with http://, https://, or oci://"))
			},
			Entry("ftp", "ftp://my.repo/helm"),
			Entry("no scheme", "my.repo/helm"),
			Entry("git ssh", "git@github.com:argoproj/argo-helm.git"),
		)
	})

	Context("full installation", func() {
		BeforeEach(func() {
			helmMock.EXPECT().FindRelease("argocd", "argocd").Return(nil, nil)
			helmMock.EXPECT().InstallChart(mock.Anything, mock.Anything, mock.Anything).Return(nil)
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

	Context("ForceConflicts", func() {
		It("passes ForceConflicts=true to InstallChart on a fresh install", func() {
			helmMock.EXPECT().FindRelease("argocd", "argocd").Return(nil, nil)
			helmMock.EXPECT().InstallChart(mock.Anything, mock.Anything, mock.MatchedBy(func(opts installer.InstallChartOptions) bool {
				return opts.ForceConflicts == true
			})).Return(nil)

			a.ForceConflicts = true
			err := a.Install()
			Expect(err).ToNot(HaveOccurred())
		})

		It("passes ForceConflicts=true to UpgradeChart on an existing release", func() {
			helmMock.EXPECT().FindRelease("argocd", "argocd").Return(&installer.ReleaseInfo{
				Name: "argocd", InstalledVersion: "6.0.0",
			}, nil)
			helmMock.EXPECT().UpgradeChart(mock.Anything, mock.Anything, mock.MatchedBy(func(opts installer.UpgradeChartOptions) bool {
				return opts.ForceConflicts == true
			})).Return(nil)

			a.ForceConflicts = true
			err := a.Install()
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("RepoURL", func() {
		BeforeEach(func() {
			helmMock.EXPECT().FindRelease("argocd", "argocd").Return(nil, nil)
		})

		It("uses a custom HTTP repo URL", func() {
			helmMock.EXPECT().InstallChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
				return cfg.RepoURL == "https://my.repo/helm" &&
					cfg.ChartName == "argo-cd"
			}), mock.Anything).Return(nil)

			a.RepoURL = "https://my.repo/helm"
			err := a.Install()
			Expect(err).ToNot(HaveOccurred())
		})

		It("builds the full OCI chart reference and clears RepoURL", func() {
			helmMock.EXPECT().InstallChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
				return cfg.ChartName == "oci://ghcr.io/argoproj/argo-helm/argo-cd" &&
					cfg.RepoURL == ""
			}), mock.Anything).Return(nil)

			a.RepoURL = "oci://ghcr.io/argoproj/argo-helm"
			err := a.Install()
			Expect(err).ToNot(HaveOccurred())
		})
	})

})
