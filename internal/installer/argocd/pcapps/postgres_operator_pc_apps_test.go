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

var _ = Describe("postgres-operator application", func() {
	withOperator := func(operator *files.PgOperatorConfig) *files.RootConfig {
		return &files.RootConfig{Cluster: files.ClusterConfig{PgOperator: operator}}
	}
	application := func(config *files.RootConfig, skipSteps ...string) files.ChartValues {
		values, err := pcapps.InstallerChartsPcAppsValues(config, skipSteps, bomWith("postgres-operator", map[string]string{
			"cloudnativePg": "ghcr.io/codesphere-cloud/docker/ghcr.io/cloudnative-pg/cloudnative-pg:1.30.0",
		}))
		Expect(err).NotTo(HaveOccurred())

		applications, _ := values["applications"].(files.ChartValues)
		app, _ := applications[pcapps.PostgresOperator].(files.ChartValues)

		return app
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

		It("turns the application off when the run skips set-up-cluster", func() {
			Expect(application(&files.RootConfig{}, installer.SetUpClusterStep)).To(Equal(files.ChartValues{"enabled": false}))
		})

		It("keeps the application on when only ms-backends is skipped", func() {
			Expect(application(&files.RootConfig{}, installer.MsBackendsStep)).To(HaveKey("valuesObject"))
		})

		It("never writes enabled: true, so a handover stays with the pc-applications default", func() {
			Expect(application(&files.RootConfig{})).ToNot(HaveKey("enabled"))
		})
	})

	Describe("the values", func() {
		It("sets the image of the BOM and no pull secret without a registry", func() {
			Expect(application(&files.RootConfig{})["valuesObject"]).To(Equal(files.ChartValues{
				"cloudnative-pg": files.ChartValues{
					"image": files.ChartValues{
						"repository": "ghcr.io/codesphere-cloud/docker/ghcr.io/cloudnative-pg/cloudnative-pg",
						"tag":        "1.30.0",
					},
					"imagePullSecrets": []any{},
				},
			}))
		})

		It("names the registry pull secret when the config has a registry", func() {
			config := &files.RootConfig{Registry: &files.RegistryConfig{Server: "registry.example.com"}}

			Expect(application(config)["valuesObject"].(files.ChartValues)["cloudnative-pg"]).To(HaveKeyWithValue(
				"imagePullSecrets", []any{map[string]any{"name": "docker-regcred"}},
			))
		})

		It("merges cluster.pgOperator.override over the derived values", func() {
			config := withOperator(&files.PgOperatorConfig{Enabled: true, Override: files.ChartOverride{
				"cloudnative-pg": map[string]any{"resources": map[string]any{"requests": map[string]any{"cpu": "0"}}},
			}})

			chart := application(config)["valuesObject"].(files.ChartValues)["cloudnative-pg"].(map[string]any)

			Expect(chart).To(HaveKey("resources"))
			Expect(chart).To(HaveKey("image"))
		})

		It("lets the override win over a derived value", func() {
			config := withOperator(&files.PgOperatorConfig{Enabled: true, Override: files.ChartOverride{
				"cloudnative-pg": map[string]any{"image": map[string]any{"tag": "custom"}},
			}})

			chart := application(config)["valuesObject"].(files.ChartValues)["cloudnative-pg"].(map[string]any)

			Expect(chart["image"]).To(HaveKeyWithValue("tag", "custom"))
			Expect(chart["image"]).To(HaveKeyWithValue("repository", "ghcr.io/codesphere-cloud/docker/ghcr.io/cloudnative-pg/cloudnative-pg"))
		})
	})
})
