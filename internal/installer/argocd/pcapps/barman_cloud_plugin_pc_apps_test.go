// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/argocd/pcapps"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("barman-cloud-plugin application", func() {
	pluginImages := map[string]string{
		"barmanCloudPlugin":        "ghcr.io/codesphere-cloud/docker/ghcr.io/cloudnative-pg/plugin-barman-cloud:v0.16.0",
		"barmanCloudPluginSidecar": "ghcr.io/codesphere-cloud/docker/ghcr.io/cloudnative-pg/plugin-barman-cloud-sidecar:v0.16.0",
	}
	withBarman := func(barman *files.BarmanCloudPluginConfig) *files.RootConfig {
		return &files.RootConfig{ManagedServiceBackends: &files.ManagedServiceBackendsConfig{BarmanCloudPlugin: barman}}
	}
	applicationFor := func(config *files.RootConfig, images map[string]string, skipSteps ...string) files.ChartValues {
		values, err := pcapps.InstallerChartsPcAppsValues(config, skipSteps, bomWith("barman-cloud-plugin", images))
		Expect(err).NotTo(HaveOccurred())

		applications, _ := values["applications"].(files.ChartValues)
		app, _ := applications[pcapps.BarmanCloudPlugin].(files.ChartValues)

		return app
	}
	application := func(config *files.RootConfig, skipSteps ...string) files.ChartValues {
		return applicationFor(config, pluginImages, skipSteps...)
	}
	chartOf := func(app files.ChartValues) files.ChartValues {
		return app["valuesObject"].(files.ChartValues)["plugin-barman-cloud"].(files.ChartValues)
	}
	off := files.ChartValues{"enabled": false}

	Describe("whether the config asks for it", func() {
		It("asks for the plugin by default", func() {
			Expect(application(&files.RootConfig{})).To(HaveKey("valuesObject"))
		})

		It("turns the application off when the plugin is disabled", func() {
			Expect(application(withBarman(&files.BarmanCloudPluginConfig{Enabled: false}))).To(Equal(off))
		})

		It("keeps the plugin on where the operator is not installed by the run, as the installer reads no operator key there", func() {
			config := withBarman(&files.BarmanCloudPluginConfig{Enabled: true})
			config.Cluster.PgOperator = &files.PgOperatorConfig{Enabled: false, ManagedExternally: true}

			Expect(application(config)).To(HaveKey("valuesObject"))
		})

		It("turns the application off when the run skips ms-backends", func() {
			Expect(application(&files.RootConfig{}, installer.MsBackendsStep)).To(Equal(off))
		})

		It("keeps the application on when only set-up-cluster is skipped", func() {
			Expect(application(&files.RootConfig{}, installer.SetUpClusterStep)).To(HaveKey("valuesObject"))
		})

		It("never writes enabled: true, so a handover stays with the pc-applications default", func() {
			Expect(application(&files.RootConfig{})).ToNot(HaveKey("enabled"))
		})
	})

	Describe("the values", func() {
		It("splits the images of the BOM into the registry, repository and tag the chart takes", func() {
			Expect(chartOf(application(&files.RootConfig{}))).To(Equal(files.ChartValues{
				"image": files.ChartValues{
					"registry":   "ghcr.io",
					"repository": "codesphere-cloud/docker/ghcr.io/cloudnative-pg/plugin-barman-cloud",
					"tag":        "v0.16.0",
				},
				"sidecarImage": files.ChartValues{
					"registry":   "ghcr.io",
					"repository": "codesphere-cloud/docker/ghcr.io/cloudnative-pg/plugin-barman-cloud-sidecar",
					"tag":        "v0.16.0",
				},
				"imagePullSecrets": []any{},
			}))
		})

		It("moves the plugin and its sidecar onto the registry that replaces the images", func() {
			config := &files.RootConfig{Registry: &files.RegistryConfig{Server: "registry.example.com/mirror", ReplaceImagesInBom: true}}
			chart := chartOf(application(config))

			Expect(chart["image"]).To(HaveKeyWithValue("registry", "registry.example.com"))
			Expect(chart["image"]).To(HaveKeyWithValue("repository", "mirror/codesphere-cloud/docker/ghcr.io/cloudnative-pg/plugin-barman-cloud"))
			Expect(chart["sidecarImage"]).To(HaveKeyWithValue("repository", "mirror/codesphere-cloud/docker/ghcr.io/cloudnative-pg/plugin-barman-cloud-sidecar"))
			Expect(chart["imagePullSecrets"]).To(Equal([]any{map[string]any{"name": "docker-regcred"}}))
		})

		It("leaves the sidecar to the chart for a BOM without a sidecar image", func() {
			chart := chartOf(applicationFor(&files.RootConfig{}, map[string]string{"barmanCloudPlugin": pluginImages["barmanCloudPlugin"]}))

			Expect(chart).To(HaveKey("image"))
			Expect(chart).ToNot(HaveKey("sidecarImage"))
		})

		It("merges managedServiceBackends.barmanCloudPlugin.override over the derived values", func() {
			config := withBarman(&files.BarmanCloudPluginConfig{Enabled: true, Override: files.ChartOverride{
				"plugin-barman-cloud": map[string]any{"replicaCount": 1},
			}})
			chart := chartOf(application(config))

			Expect(chart).To(HaveKeyWithValue("replicaCount", 1))
			Expect(chart).To(HaveKey("image"))
		})

		It("lets the override win over a derived value", func() {
			config := withBarman(&files.BarmanCloudPluginConfig{Enabled: true, Override: files.ChartOverride{
				"plugin-barman-cloud": map[string]any{"image": map[string]any{"tag": "custom"}},
			}})
			chart := chartOf(application(config))

			Expect(chart["image"]).To(HaveKeyWithValue("tag", "custom"))
			Expect(chart["image"]).To(HaveKeyWithValue("registry", "ghcr.io"))
		})
	})
})
