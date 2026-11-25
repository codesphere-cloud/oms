// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/argocd/pcapps"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("postgres-operator application", func() {
	withOperator := func(operator *files.PgOperatorConfig) *files.RootConfig {
		return &files.RootConfig{Cluster: files.ClusterConfig{PgOperator: operator}}
	}
	application := func(config *files.RootConfig) files.ChartValues {
		return applicationOf(pcapps.PostgresOperatorPcAppsValues(config), pcapps.PostgresOperator)
	}
	chartOf := func(app files.ChartValues) files.ChartValues {
		return app["valuesObject"].(files.ChartValues)["cloudnative-pg"].(files.ChartValues)
	}

	Describe("whether the config asks for it", func() {
		It("asks for the operator without a cluster.pgOperator block", func() {
			Expect(application(&files.RootConfig{})).To(HaveKey("valuesObject"))
		})

		It("asks for the operator when it is enabled", func() {
			Expect(application(withOperator(&files.PgOperatorConfig{Enabled: true}))).To(HaveKey("valuesObject"))
		})

		It("turns the application off when the operator is disabled", func() {
			Expect(application(withOperator(&files.PgOperatorConfig{Enabled: false}))).To(Equal(files.ChartValues{"enabled": false}))
		})

		It("turns the application off for an operator something else installs", func() {
			Expect(application(withOperator(&files.PgOperatorConfig{ManagedExternally: true}))).To(Equal(files.ChartValues{"enabled": false}))
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

		It("merges cluster.pgOperator.override over the derived values", func() {
			config := withOperator(&files.PgOperatorConfig{Enabled: true, Override: files.ChartOverride{
				"cloudnative-pg": map[string]any{"resources": map[string]any{"requests": map[string]any{"cpu": "0"}}},
			}})

			chart := chartOf(application(config))

			Expect(chart).To(HaveKey("resources"))
			Expect(chart).To(HaveKey("imagePullSecrets"))
		})

		It("lets the override win over a derived value", func() {
			config := withOperator(&files.PgOperatorConfig{Enabled: true, Override: files.ChartOverride{
				"cloudnative-pg": map[string]any{"imagePullSecrets": []any{map[string]any{"name": "custom"}}},
			}})

			Expect(chartOf(application(config))).To(HaveKeyWithValue("imagePullSecrets", []any{map[string]any{"name": "custom"}}))
		})
	})
})
