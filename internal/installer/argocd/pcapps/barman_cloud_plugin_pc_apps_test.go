// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/argocd/pcapps"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("barman-cloud-plugin application", func() {
	withBarman := func(barman *files.BarmanCloudPluginConfig) *files.RootConfig {
		return &files.RootConfig{ManagedServiceBackends: &files.ManagedServiceBackendsConfig{BarmanCloudPlugin: barman}}
	}
	application := func(config *files.RootConfig) files.ChartValues {
		return applicationOf(pcapps.BarmanCloudPluginPcAppsValues(config), pcapps.BarmanCloudPlugin)
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

		It("keeps the plugin on where the operator is installed by something else, whose CRDs exist", func() {
			config := withBarman(&files.BarmanCloudPluginConfig{Enabled: true})
			config.Cluster.PgOperator = &files.PgOperatorConfig{Enabled: false, ManagedExternally: true}

			Expect(application(config)).To(HaveKey("valuesObject"))
		})

		It("turns the application off when no operator is installed, as the plugin has no CRDs to run on", func() {
			config := withBarman(&files.BarmanCloudPluginConfig{Enabled: true})
			config.Cluster.PgOperator = &files.PgOperatorConfig{Enabled: false}

			Expect(application(config)).To(Equal(off))
		})

		It("keeps the plugin on with the operator enabled", func() {
			config := &files.RootConfig{Cluster: files.ClusterConfig{PgOperator: &files.PgOperatorConfig{Enabled: true}}}

			Expect(application(config)).To(HaveKey("valuesObject"))
		})

		It("never writes enabled: true, so a handover stays with the pc-applications default", func() {
			Expect(application(&files.RootConfig{})).ToNot(HaveKey("enabled"))
		})
	})

	Describe("the values", func() {
		It("sets no pull secret and no image without a registry", func() {
			Expect(chartOf(application(&files.RootConfig{}))).To(Equal(files.ChartValues{"imagePullSecrets": []any{}}))
		})

		It("names the registry pull secret when the config has a registry", func() {
			config := &files.RootConfig{Registry: &files.RegistryConfig{Server: "registry.example.com"}}

			Expect(chartOf(application(config))).To(Equal(files.ChartValues{
				"imagePullSecrets": []any{map[string]any{"name": "docker-regcred"}},
			}))
		})

		It("merges managedServiceBackends.barmanCloudPlugin.override over the derived values", func() {
			config := withBarman(&files.BarmanCloudPluginConfig{Enabled: true, Override: files.ChartOverride{
				"plugin-barman-cloud": map[string]any{"replicaCount": 1},
			}})
			chart := chartOf(application(config))

			Expect(chart).To(HaveKeyWithValue("replicaCount", 1))
			Expect(chart).To(HaveKey("imagePullSecrets"))
		})

		It("lets the override win over a derived value", func() {
			config := withBarman(&files.BarmanCloudPluginConfig{Enabled: true, Override: files.ChartOverride{
				"plugin-barman-cloud": map[string]any{"imagePullSecrets": []any{map[string]any{"name": "custom"}}},
			}})

			Expect(chartOf(application(config))).To(HaveKeyWithValue("imagePullSecrets", []any{map[string]any{"name": "custom"}}))
		})
	})
})
