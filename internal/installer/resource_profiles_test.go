// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	. "github.com/codesphere-cloud/oms/internal/util/testing"
)

var _ = Describe("ApplyResourceProfile", func() {
	Describe("noRequests", func() {
		It("preserves existing config while adding zero requests", func() {
			config := &files.RootConfig{
				Cluster: files.ClusterConfig{
					Monitoring: &files.MonitoringConfig{
						Prometheus: &files.PrometheusConfig{
							RemoteWrite: &files.RemoteWriteConfig{
								Enabled:     true,
								ClusterName: "existing-cluster",
							},
						},
					},
					Gateway: files.GatewayConfig{
						ServiceType: "LoadBalancer",
						Override: map[string]interface{}{
							"ingress-nginx": map[string]interface{}{
								"controller": map[string]interface{}{
									"existing": "value",
								},
							},
						},
					},
				},
				Codesphere: files.CodesphereConfig{
					Override: map[string]interface{}{
						"global": map[string]interface{}{
							"services": map[string]interface{}{
								"auth_service": map[string]interface{}{
									"existing": "value",
								},
							},
						},
					},
				},
			}

			Expect(installer.ApplyResourceProfile(config, installer.ResourceProfileNoRequests)).To(Succeed())

			Expect(config.Cluster.Monitoring).NotTo(BeNil())
			Expect(config.Cluster.Monitoring.Prometheus).NotTo(BeNil())
			Expect(config.Cluster.Monitoring.Prometheus.RemoteWrite).NotTo(BeNil())
			Expect(config.Cluster.Monitoring.Prometheus.RemoteWrite.Enabled).To(BeTrue())
			Expect(config.Cluster.Monitoring.Prometheus.RemoteWrite.ClusterName).To(Equal("existing-cluster"))

			controller := MustMap[any](MustMap[any](config.Cluster.Gateway.Override["ingress-nginx"])["controller"])
			AssertZeroRequests(MustMap[any](controller["resources"])["requests"])

			deployService := MustMap[any](MustMap[any](MustMap[any](config.Codesphere.Override["global"])["services"])["deployment_service"])
			AssertZeroRequests(deployService["requests"])
			Expect(deployService["replicas"]).To(Equal(2))
			authService := MustMap[any](MustMap[any](MustMap[any](config.Codesphere.Override["global"])["services"])["auth_service"])
			Expect(authService["replicas"]).To(Equal(2))
			publicAPIService := MustMap[any](MustMap[any](MustMap[any](config.Codesphere.Override["global"])["services"])["public_api_service"])
			Expect(publicAPIService["replicas"]).To(Equal(2))
			workspaceService := MustMap[any](MustMap[any](MustMap[any](config.Codesphere.Override["global"])["services"])["workspace_service"])
			Expect(workspaceService["replicas"]).To(Equal(2))
			underprovisionFactors := MustMap[string](MustMap[any](config.Codesphere.Override["global"])["underprovisionFactors"])
			Expect(underprovisionFactors["cpu"]).To(Equal("0.01"))
			Expect(underprovisionFactors["memory"]).To(Equal("0.01"))

			Expect(config.Cluster.CertManager).NotTo(BeNil())
			Expect(config.Cluster.TrustManager).NotTo(BeNil())
			Expect(config.Cluster.CertManager.Override).NotTo(BeNil())
			Expect(config.Cluster.TrustManager.Override).NotTo(BeNil())
			Expect(config.Cluster.Monitoring.BlackboxExporter).NotTo(BeNil())
			Expect(config.Cluster.Monitoring.PushGateway).NotTo(BeNil())
			Expect(config.Cluster.PublicGateway.Override).NotTo(BeNil())
			Expect(config.Cluster.PgOperator).NotTo(BeNil())
			Expect(config.Cluster.BarmanCloudPlugin).NotTo(BeNil())
			Expect(config.Cluster.RgwLoadBalancer).NotTo(BeNil())
			Expect(config.Cluster.RgwLoadBalancer.Override).NotTo(BeNil())
			Expect(config.ManagedServiceBackends).NotTo(BeNil())
			Expect(config.ManagedServiceBackends.Postgres).NotTo(BeNil())
			Expect(config.ManagedServiceBackends.S3).NotTo(BeNil())

			trustManager := MustMap[any](config.Cluster.TrustManager.Override["trust-manager"])
			AssertZeroRequests(MustMap[any](trustManager["resources"])["requests"])

			pgOperator := MustMap[any](config.Cluster.PgOperator.Override["cloudnative-pg"])
			AssertZeroRequests(MustMap[any](pgOperator["resources"])["requests"])
			Expect(MustMap[any](pgOperator["config"])["clusterWide"]).To(Equal(false))

			barmanCloud := MustMap[any](config.Cluster.BarmanCloudPlugin.Override["plugin-barman-cloud"])
			AssertZeroRequests(MustMap[any](barmanCloud["resources"])["requests"])

			Expect(config.Cluster.RgwLoadBalancer.Override["replicas"]).To(Equal(1))

			managedPostgres := config.ManagedServiceBackends.Postgres.Override
			Expect(managedPostgres["replicas"]).To(Equal(1))
			AssertZeroRequests(MustMap[any](managedPostgres["resources"])["requests"])

			managedS3 := config.ManagedServiceBackends.S3.Override
			Expect(managedS3["replicas"]).To(Equal(1))
			AssertZeroRequests(MustMap[any](managedS3["resources"])["requests"])
		})
	})

	It("returns an error for an invalid profile", func() {
		config := &files.RootConfig{}
		Expect(installer.ApplyResourceProfile(config, installer.ResourceProfile("invalid"))).To(MatchError(ContainSubstring("unsupported resource profile")))
	})

	It("returns an error for a nil config", func() {
		Expect(installer.ApplyResourceProfile(nil, installer.ResourceProfileNoRequests)).To(MatchError("root config is nil"))
	})
})
