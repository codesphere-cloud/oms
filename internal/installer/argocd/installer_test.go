// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package argocd_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/argocd"
	"github.com/codesphere-cloud/oms/internal/installer/bom"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
)

func writeValuesFile(content string) string {
	path := filepath.Join(GinkgoT().TempDir(), "values.yaml")
	Expect(os.WriteFile(path, []byte(content), 0o600)).To(Succeed())
	return path
}

var _ = Describe("Installer.Install", func() {
	var (
		helmMock            *installer.MockHelmClient
		argoCDResourcesMock *argocd.MockArgoCDResources
		a                   *argocd.Installer
	)

	BeforeEach(func() {
		helmMock = installer.NewMockHelmClient(GinkgoT())
		argoCDResourcesMock = argocd.NewMockArgoCDResources(GinkgoT())
		a = &argocd.Installer{InstallerConfig: argocd.InstallerConfig{Version: "7.0.0"}, Helm: helmMock, Resources: argoCDResourcesMock}
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
					cfg.CreateNamespace
			}), mock.Anything).Return(nil)

			err := a.Install()
			Expect(err).ToNot(HaveOccurred())
		})

		It("performs a fresh install with latest version when Version is empty", func() {
			helmMock.EXPECT().FindRelease("argocd", "argocd").Return(nil, nil)
			helmMock.EXPECT().InstallChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
				return cfg.Version == ""
			}), mock.Anything).Return(nil)

			a = &argocd.Installer{InstallerConfig: argocd.InstallerConfig{Version: ""}, Helm: helmMock}

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

			a = &argocd.Installer{InstallerConfig: argocd.InstallerConfig{Version: "6.0.0"}, Helm: helmMock}

			err := a.Install()
			Expect(err).ToNot(HaveOccurred())
		})

		It("upgrades to latest when Version is empty", func() {
			helmMock.EXPECT().UpgradeChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
				return cfg.Version == ""
			}), installer.UpgradeChartOptions{InstallIfNotExist: false}).Return(nil)

			a = &argocd.Installer{InstallerConfig: argocd.InstallerConfig{Version: ""}, Helm: helmMock}

			err := a.Install()
			Expect(err).ToNot(HaveOccurred())
		})

		It("rejects a downgrade", func() {
			a = &argocd.Installer{InstallerConfig: argocd.InstallerConfig{Version: "5.0.0"}, Helm: helmMock}

			err := a.Install()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("downgrade is not allowed"))
		})

		It("returns an error when UpgradeChart fails", func() {
			helmMock.EXPECT().UpgradeChart(mock.Anything, mock.Anything, mock.Anything).
				Return(errors.New("timeout waiting for condition"))

			a = &argocd.Installer{InstallerConfig: argocd.InstallerConfig{Version: "7.0.0"}, Helm: helmMock}

			err := a.Install()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("timeout"))
		})
	})

	Context("when FindRelease returns an error", func() {
		It("propagates the error without installing or upgrading", func() {
			helmMock.EXPECT().FindRelease("argocd", "argocd").
				Return(nil, errors.New("cluster unreachable"))

			a = &argocd.Installer{InstallerConfig: argocd.InstallerConfig{Version: "7.0.0"}, Helm: helmMock}

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

			a = &argocd.Installer{InstallerConfig: argocd.InstallerConfig{Version: "7.0.0"}, Helm: helmMock}

			err := a.Install()
			Expect(err).ToNot(HaveOccurred())
		})

		It("disables dex in the values", func() {
			helmMock.EXPECT().FindRelease("argocd", "argocd").Return(nil, nil)
			helmMock.EXPECT().InstallChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
				dex, ok := cfg.Values["dex"].(map[string]interface{})
				return ok && dex["enabled"] == false
			}), mock.Anything).Return(nil)

			a = &argocd.Installer{InstallerConfig: argocd.InstallerConfig{Version: "7.0.0"}, Helm: helmMock}

			err := a.Install()
			Expect(err).ToNot(HaveOccurred())
		})

		It("trusts the registry certificate authority under the host without its port", func() {
			authority := filepath.Join(GinkgoT().TempDir(), "registry.crt")
			Expect(os.WriteFile(authority, []byte("-----BEGIN CERTIFICATE-----\n"), 0o600)).To(Succeed())

			helmMock.EXPECT().FindRelease("argocd", "argocd").Return(nil, nil)
			helmMock.EXPECT().InstallChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
				configs, ok := cfg.Values["configs"].(map[string]interface{})
				if !ok {
					return false
				}

				tls, ok := configs["tls"].(map[string]interface{})
				if !ok {
					return false
				}

				certificates, ok := tls["certificates"].(map[string]interface{})

				return ok && certificates["10.10.0.7"] == "-----BEGIN CERTIFICATE-----\n"
			}), mock.Anything).Return(nil)

			a = &argocd.Installer{InstallerConfig: argocd.InstallerConfig{
				Version:           "7.0.0",
				OciRegistryURL:    "10.10.0.7:5000/codesphere-cloud/charts",
				OciRegistryCAFile: authority,
			}, Helm: helmMock}

			Expect(a.Install()).To(Succeed())
		})

		It("trusts the certificate authority under the host of a registry URL naming its scheme", func() {
			authority := filepath.Join(GinkgoT().TempDir(), "registry.crt")
			Expect(os.WriteFile(authority, []byte("-----BEGIN CERTIFICATE-----\n"), 0o600)).To(Succeed())

			helmMock.EXPECT().FindRelease("argocd", "argocd").Return(nil, nil)
			helmMock.EXPECT().InstallChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
				configs, ok := cfg.Values["configs"].(map[string]interface{})
				if !ok {
					return false
				}

				tls, ok := configs["tls"].(map[string]interface{})
				if !ok {
					return false
				}

				certificates, ok := tls["certificates"].(map[string]interface{})

				return ok && certificates["10.10.0.7"] != nil
			}), mock.Anything).Return(nil)

			a = &argocd.Installer{InstallerConfig: argocd.InstallerConfig{
				Version:           "7.0.0",
				OciRegistryURL:    "oci://10.10.0.7/codesphere-cloud/charts",
				OciRegistryCAFile: authority,
			}, Helm: helmMock}

			Expect(a.Install()).To(Succeed())
		})

		It("configures no certificate authority when none is given", func() {
			helmMock.EXPECT().FindRelease("argocd", "argocd").Return(nil, nil)
			helmMock.EXPECT().InstallChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
				_, configured := cfg.Values["configs"]
				return !configured
			}), mock.Anything).Return(nil)

			a = &argocd.Installer{InstallerConfig: argocd.InstallerConfig{
				Version:        "7.0.0",
				OciRegistryURL: "ghcr.io/codesphere-cloud/charts",
			}, Helm: helmMock}

			Expect(a.Install()).To(Succeed())
		})

		It("fails when the certificate authority cannot be read", func() {
			a = &argocd.Installer{InstallerConfig: argocd.InstallerConfig{
				Version:           "7.0.0",
				OciRegistryURL:    "10.10.0.7:5000/codesphere-cloud/charts",
				OciRegistryCAFile: "/does/not/exist.crt",
			}, Helm: helmMock}

			Expect(a.Install()).To(MatchError(ContainSubstring("reading registry certificate authority")))
		})
	})

	Context("BOM chart", func() {
		var bomConfig *bom.Config

		BeforeEach(func() {
			bomPath := filepath.Join(GinkgoT().TempDir(), "bom.json")
			Expect(os.WriteFile(bomPath, []byte(`{"components":{"argocd":{
				"files":{"chart":{"ociRef":"ghcr.io/codesphere-cloud/charts/argocd:1.2.3"}},
				"containerImages":{"argocd":"quay.io/argoproj/argocd:v3.1.0","redis":"public.ecr.aws/docker/library/redis:7.2"}
			}}}`), 0o600)).To(Succeed())

			var err error

			bomConfig, err = bom.Parse(bomPath)
			Expect(err).NotTo(HaveOccurred())
		})

		It("uses the argocd OCI chart and version from the BOM with an OCI mirror", func() {
			helmMock.EXPECT().FindRelease("argocd", "argocd").Return(nil, nil)
			helmMock.EXPECT().InstallChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
				argoValues, ok := cfg.Values["argo-cd"].(map[string]interface{})
				if !ok {
					return false
				}

				dex, ok := argoValues["dex"].(map[string]interface{})
				if !ok {
					return false
				}

				global, ok := argoValues["global"].(map[string]interface{})
				if !ok {
					return false
				}

				image, ok := global["image"].(map[string]interface{})
				if !ok {
					return false
				}

				repository, ok := image["repository"].(string)

				return cfg.ChartName == "oci://10.10.0.6:5000/codesphere-cloud/charts/argocd" &&
					cfg.RepoURL == "" && cfg.Version == "1.2.3" &&
					dex["enabled"] == false && cfg.Values["dex"] == nil &&
					ok && strings.HasPrefix(repository, "10.10.0.6:5000/")
			}), mock.Anything).Return(nil)

			a = &argocd.Installer{InstallerConfig: argocd.InstallerConfig{
				BOM:            bomConfig,
				RepoURL:        "oci://10.10.0.6:5000/codesphere-cloud/charts",
				OciRegistryURL: "10.10.0.6:5000/codesphere-cloud/charts",
			}, Helm: helmMock}
			Expect(a.Install()).To(Succeed())
		})

		It("uses the upstream chart when the BOM comes without an OCI repository", func() {
			helmMock.EXPECT().FindRelease("argocd", "argocd").Return(nil, nil)
			helmMock.EXPECT().InstallChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
				return cfg.ChartName == "argo-cd" && cfg.RepoURL == argocd.DefaultRepoURL
			}), mock.Anything).Return(nil)

			a = &argocd.Installer{InstallerConfig: argocd.InstallerConfig{BOM: bomConfig}, Helm: helmMock}
			Expect(a.Install()).To(Succeed())
		})

		It("uses the upstream chart from an OCI repository other than the mirror", func() {
			helmMock.EXPECT().FindRelease("argocd", "argocd").Return(nil, nil)
			helmMock.EXPECT().InstallChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
				return cfg.ChartName == "oci://ghcr.io/argoproj/argo-helm/argo-cd" && cfg.Version == ""
			}), mock.Anything).Return(nil)

			a = &argocd.Installer{InstallerConfig: argocd.InstallerConfig{
				BOM:            bomConfig,
				RepoURL:        "oci://ghcr.io/argoproj/argo-helm",
				OciRegistryURL: "10.10.0.6:5000/codesphere-cloud/charts",
			}, Helm: helmMock}
			Expect(a.Install()).To(Succeed())
		})

		DescribeTable("compares the OCI repository with the registry URL",
			func(repoURL, registryURL, chartName string) {
				helmMock.EXPECT().FindRelease("argocd", "argocd").Return(nil, nil)
				helmMock.EXPECT().InstallChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
					return cfg.ChartName == chartName
				}), mock.Anything).Return(nil)

				a = &argocd.Installer{InstallerConfig: argocd.InstallerConfig{
					BOM:            bomConfig,
					RepoURL:        repoURL,
					OciRegistryURL: registryURL,
				}, Helm: helmMock}
				Expect(a.Install()).To(Succeed())
			},
			Entry("naming its scheme", "oci://10.10.0.6:5000/codesphere-cloud/charts", "oci://10.10.0.6:5000/codesphere-cloud/charts", "oci://10.10.0.6:5000/codesphere-cloud/charts/argocd"),
			Entry("ending the repository in a slash", "oci://10.10.0.6:5000/codesphere-cloud/charts/", "10.10.0.6:5000/codesphere-cloud/charts", "oci://10.10.0.6:5000/codesphere-cloud/charts/argocd"),
			Entry("ending the registry URL in a slash", "oci://10.10.0.6:5000/codesphere-cloud/charts", "10.10.0.6:5000/codesphere-cloud/charts/", "oci://10.10.0.6:5000/codesphere-cloud/charts/argocd"),
			Entry("left empty", "oci://10.10.0.6:5000/codesphere-cloud/charts", "", "oci://10.10.0.6:5000/codesphere-cloud/charts/argo-cd"),
		)

		It("falls back to the upstream chart when no BOM is provided", func() {
			helmMock.EXPECT().FindRelease("argocd", "argocd").Return(nil, nil)
			helmMock.EXPECT().InstallChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
				return cfg.ChartName == "argo-cd" && cfg.RepoURL == argocd.DefaultRepoURL
			}), mock.Anything).Return(nil)

			a = &argocd.Installer{Helm: helmMock}
			Expect(a.Install()).To(Succeed())
		})

		Context("installing from a mirror", func() {
			const (
				mirrorRegistry = "10.10.0.6:5000/codesphere-cloud/charts"
				mirrorURL      = "oci://" + mirrorRegistry
			)

			var installed installer.ChartConfig

			captureInstall := func() {
				helmMock.EXPECT().FindRelease("argocd", "argocd").Return(nil, nil)
				helmMock.EXPECT().InstallChart(mock.Anything, mock.Anything, mock.Anything).
					RunAndReturn(func(_ context.Context, cfg installer.ChartConfig, _ installer.InstallChartOptions) error {
						installed = cfg
						return nil
					})
			}

			// dockerConfigAuth decodes the login a dockerconfigjson secret holds for host.
			dockerConfigAuth := func(host string) func(string) string {
				return func(encoded string) string {
					raw, err := base64.StdEncoding.DecodeString(encoded)
					Expect(err).NotTo(HaveOccurred())

					var config struct {
						Auths map[string]struct {
							Auth string `json:"auth"`
						} `json:"auths"`
					}
					Expect(json.Unmarshal(raw, &config)).To(Succeed())

					auth, err := base64.StdEncoding.DecodeString(config.Auths[host].Auth)
					Expect(err).NotTo(HaveOccurred())

					return string(auth)
				}
			}

			It("points the images at the mirror and creates their pull secret ahead of the chart's hooks", func() {
				captureInstall()

				a = &argocd.Installer{InstallerConfig: argocd.InstallerConfig{
					BOM:            bomConfig,
					RepoURL:        mirrorURL,
					OciRegistryURL: mirrorRegistry,
					OciUsername:    "oms",
					OciPassword:    "secret",
				}, Helm: helmMock}
				Expect(a.Install()).To(Succeed())

				Expect(installed.Values).To(HaveKeyWithValue("extraSecrets", BeEmpty()))
				Expect(installed.Values).To(HaveKeyWithValue("argo-cd", And(
					HaveKeyWithValue("global", And(
						HaveKeyWithValue("image", HaveKeyWithValue("repository", "10.10.0.6:5000/quay_io/argoproj/argocd")),
						HaveKeyWithValue("imagePullSecrets", ConsistOf(HaveKeyWithValue("name", "argocd-registry-pull"))),
					)),
					HaveKeyWithValue("redis", HaveKeyWithValue("image", HaveKeyWithValue("repository", "10.10.0.6:5000/public_ecr_aws/docker/library/redis"))),
					HaveKeyWithValue("extraObjects", ConsistOf(And(
						HaveKeyWithValue("type", "kubernetes.io/dockerconfigjson"),
						HaveKeyWithValue("metadata", And(
							HaveKeyWithValue("name", "argocd-registry-pull"),
							HaveKeyWithValue("annotations", And(
								HaveKeyWithValue("helm.sh/hook", "pre-install,pre-upgrade"),
								HaveKeyWithValue("helm.sh/hook-weight", "-10"),
							)),
						)),
						HaveKeyWithValue("data", HaveKeyWithValue(".dockerconfigjson",
							WithTransform(dockerConfigAuth("10.10.0.6:5000"), Equal("oms:secret")))),
					))),
				)))
			})

			It("installs the version given over the one the BOM pins", func() {
				captureInstall()

				a = &argocd.Installer{InstallerConfig: argocd.InstallerConfig{
					Version:        "2.0.0",
					BOM:            bomConfig,
					RepoURL:        mirrorURL,
					OciRegistryURL: mirrorRegistry,
				}, Helm: helmMock}
				Expect(a.Install()).To(Succeed())

				Expect(installed.ChartName).To(Equal("oci://10.10.0.6:5000/codesphere-cloud/charts/argocd"))
				Expect(installed.Version).To(Equal("2.0.0"))
			})

			It("lets a value file override the mirror defaults", func() {
				captureInstall()

				a = &argocd.Installer{InstallerConfig: argocd.InstallerConfig{
					BOM:            bomConfig,
					RepoURL:        mirrorURL,
					OciRegistryURL: mirrorRegistry,
					ValueFiles:     []string{writeValuesFile("argo-cd:\n  redis:\n    image:\n      repository: registry.example.com/redis\n")},
				}, Helm: helmMock}
				Expect(a.Install()).To(Succeed())

				Expect(installed.Values).To(HaveKeyWithValue("argo-cd", And(
					HaveKeyWithValue("redis", HaveKeyWithValue("image", HaveKeyWithValue("repository", "registry.example.com/redis"))),
					HaveKeyWithValue("global", HaveKeyWithValue("image", HaveKeyWithValue("repository", "10.10.0.6:5000/quay_io/argoproj/argocd"))),
				)))
			})

			chart := map[string]bom.FileRef{"chart": {OciRef: "ghcr.io/codesphere-cloud/charts/argocd:1.2.3"}}

			DescribeTable("fails without touching helm when the BOM cannot serve the mirror",
				func(component bom.ComponentConfig, message string) {
					a = &argocd.Installer{InstallerConfig: argocd.InstallerConfig{
						BOM:            &bom.Config{Components: map[string]bom.ComponentConfig{"argocd": component}},
						RepoURL:        mirrorURL,
						OciRegistryURL: mirrorRegistry,
					}, Helm: helmMock}

					Expect(a.Install()).To(MatchError(ContainSubstring(message)))
				},
				Entry("without a tagged argocd chart", bom.ComponentConfig{}, "BOM has no tagged argocd chart"),
				Entry("without the argocd image", bom.ComponentConfig{
					Files:           chart,
					ContainerImages: map[string]string{"redis": "redis:7.2"},
				}, "BOM lists no argoproj/argocd image"),
				Entry("without the redis image", bom.ComponentConfig{
					Files:           chart,
					ContainerImages: map[string]string{"argocd": "quay.io/argoproj/argocd:v3.1.0"},
				}, "BOM lists no redis image"),
			)
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

			a = &argocd.Installer{InstallerConfig: argocd.InstallerConfig{Version: "7.0.0", ValueFiles: []string{valuesFile}}, Helm: helmMock}

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

			a = &argocd.Installer{InstallerConfig: argocd.InstallerConfig{Version: "7.0.0", ValueFiles: []string{valuesFile}}, Helm: helmMock}

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

			a = &argocd.Installer{InstallerConfig: argocd.InstallerConfig{Version: "7.0.0", ValueFiles: []string{first, second}}, Helm: helmMock}

			err := a.Install()
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("RepoURL validation", func() {
		DescribeTable("accepts supported schemes",
			func(repoURL string) {
				helmMock.EXPECT().FindRelease("argocd", "argocd").Return(nil, nil)
				helmMock.EXPECT().InstallChart(mock.Anything, mock.Anything, mock.Anything).Return(nil)

				a = &argocd.Installer{InstallerConfig: argocd.InstallerConfig{Version: "7.0.0", RepoURL: repoURL}, Helm: helmMock}

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
				a = &argocd.Installer{InstallerConfig: argocd.InstallerConfig{Version: "7.0.0", RepoURL: repoURL}, Helm: helmMock}

				err := a.Install()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("must start with http://, https://, or oci://"))
			},
			Entry("ftp", "ftp://my.repo/helm"),
			Entry("file", "file:///charts"),
			Entry("bare host", "my.repo/helm"),
		)
	})

	Context("FullInstall behavior", func() {
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

	Context("ForceConflicts behavior", func() {
		It("passes ForceConflicts=true to InstallChart on a fresh install", func() {
			helmMock.EXPECT().FindRelease("argocd", "argocd").Return(nil, nil)
			helmMock.EXPECT().InstallChart(mock.Anything, mock.Anything, mock.MatchedBy(func(opts installer.InstallChartOptions) bool {
				return opts.ForceConflicts
			})).Return(nil)

			a.ForceConflicts = true
			err := a.Install()
			Expect(err).ToNot(HaveOccurred())
		})

		It("passes ForceConflicts=true to UpgradeChart on upgrade", func() {
			helmMock.EXPECT().FindRelease("argocd", "argocd").Return(&installer.ReleaseInfo{
				Name: "argocd", InstalledVersion: "6.0.0",
			}, nil)
			helmMock.EXPECT().UpgradeChart(mock.Anything, mock.Anything, mock.MatchedBy(func(opts installer.UpgradeChartOptions) bool {
				return opts.ForceConflicts
			})).Return(nil)

			a.ForceConflicts = true
			err := a.Install()
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("OCI chart reference resolution", func() {
		It("uses full OCI chart reference and empty RepoURL", func() {
			helmMock.EXPECT().FindRelease("argocd", "argocd").Return(nil, nil)
			helmMock.EXPECT().InstallChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
				return cfg.ChartName == "oci://ghcr.io/argoproj/argo-helm/argo-cd" &&
					cfg.RepoURL == ""
			}), mock.Anything).Return(nil)

			a.RepoURL = "oci://ghcr.io/argoproj/argo-helm"
			err := a.Install()
			Expect(err).ToNot(HaveOccurred())
		})

		It("trims a trailing slash in OCI repo URL", func() {
			helmMock.EXPECT().FindRelease("argocd", "argocd").Return(nil, nil)
			helmMock.EXPECT().InstallChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
				return cfg.ChartName == "oci://ghcr.io/argoproj/argo-helm/argo-cd" &&
					cfg.RepoURL == ""
			}), mock.Anything).Return(nil)

			a.RepoURL = "oci://ghcr.io/argoproj/argo-helm/"
			err := a.Install()
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
