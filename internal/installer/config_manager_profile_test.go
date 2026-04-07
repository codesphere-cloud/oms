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

var _ = Describe("ConfigManagerProfile", func() {
	var (
		manager installer.InstallConfigManager
	)

	BeforeEach(func() {
		manager = installer.NewInstallConfigManager()
	})

	Describe("ApplyProfile", func() {
		Context("with dev profile", func() {
			It("should configure single-node development setup", func() {
				err := manager.ApplyProfile(installer.PROFILE_DEV)
				Expect(err).ToNot(HaveOccurred())

				config := manager.GetInstallConfig()
				Expect(config.Datacenter.Name).To(Equal("dev"))
				Expect(config.Postgres.Primary.IP).To(Equal("127.0.0.1"))
				Expect(config.Postgres.Primary.Hostname).To(Equal("localhost"))
				Expect(config.Ceph.NodesSubnet).To(Equal("127.0.0.1/32"))
				Expect(config.Ceph.Hosts).To(HaveLen(1))
				Expect(config.Ceph.Hosts[0].IPAddress).To(Equal("127.0.0.1"))
				Expect(config.Ceph.Hosts[0].IsMaster).To(BeTrue())
				Expect(config.Kubernetes.APIServerHost).To(Equal("127.0.0.1"))
				Expect(config.Kubernetes.ControlPlanes).To(HaveLen(1))
				Expect(config.Kubernetes.Workers).To(HaveLen(1))
				Expect(config.Codesphere.Domain).To(Equal("codesphere.local"))
			})

			It("should set managed Kubernetes configuration", func() {
				err := manager.ApplyProfile(installer.PROFILE_DEV)
				Expect(err).ToNot(HaveOccurred())

				config := manager.GetInstallConfig()
				Expect(config.Kubernetes.ManagedByCodesphere).To(BeTrue())
				Expect(config.Kubernetes.NeedsKubeConfig).To(BeFalse())
			})
		})

		Context("with development profile", func() {
			It("should apply development profile as alias for dev", func() {
				err := manager.ApplyProfile(installer.PROFILE_DEVELOPMENT)
				Expect(err).ToNot(HaveOccurred())

				config := manager.GetInstallConfig()
				Expect(config.Datacenter.Name).To(Equal("dev"))
			})
		})

		Context("with unknown profile", func() {
			It("should return error for unknown profile", func() {
				err := manager.ApplyProfile("unknown")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unknown profile"))
			})
		})

		Context("common configuration across all profiles", func() {
			profiles := []string{
				installer.PROFILE_DEV,
				installer.PROFILE_PROD,
				installer.PROFILE_MINIMAL,
			}

			for _, profile := range profiles {
				It("should set all common properties for "+profile, func() {
					err := manager.ApplyProfile(profile)
					Expect(err).ToNot(HaveOccurred())

					config := manager.GetInstallConfig()

					// Datacenter config
					Expect(config.Datacenter.ID).To(Equal(1))
					Expect(config.Datacenter.City).To(Equal("Karlsruhe"))
					Expect(config.Datacenter.CountryCode).To(Equal("DE"))

					// PostgreSQL mode
					Expect(config.Postgres.Mode).To(Equal("install"))
					Expect(config.Postgres.Primary).ToNot(BeNil())

					// Kubernetes
					Expect(config.Kubernetes.ManagedByCodesphere).To(BeTrue())
					Expect(config.Kubernetes.NeedsKubeConfig).To(BeFalse())

					// Cluster certificates
					Expect(config.Cluster.Certificates.CA.Algorithm).To(Equal("RSA"))
					Expect(config.Cluster.Certificates.CA.KeySizeBits).To(Equal(2048))

					// Gateway
					Expect(config.Cluster.Gateway.ServiceType).To(Equal("LoadBalancer"))
					Expect(config.Cluster.PublicGateway.ServiceType).To(Equal("LoadBalancer"))

					// MetalLB
					Expect(config.MetalLB).ToNot(BeNil())
					Expect(config.MetalLB.Enabled).To(BeFalse())

					// Ceph OSDs
					Expect(config.Ceph.OSDs).To(HaveLen(1))
					osd := config.Ceph.OSDs[0]
					Expect(osd.SpecID).To(Equal("default"))
					Expect(osd.Placement.HostPattern).To(Equal("*"))
					Expect(osd.DataDevices.Size).To(Equal("240G:300G"))
					Expect(osd.DataDevices.Limit).To(Equal(1))
					Expect(osd.DBDevices.Size).To(Equal("120G:150G"))
					Expect(osd.DBDevices.Limit).To(Equal(1))

					// Workspace images
					Expect(config.Codesphere.WorkspaceImages).ToNot(BeNil())
					Expect(config.Codesphere.WorkspaceImages.Agent).ToNot(BeNil())
					Expect(config.Codesphere.WorkspaceImages.Agent.BomRef).To(Equal("workspace-agent-24.04"))

					// Deploy config
					images := config.Codesphere.DeployConfig.Images
					Expect(images).To(HaveKey("ubuntu-24.04"))
					ubuntu := images["ubuntu-24.04"]
					Expect(ubuntu.Name).To(Equal("Ubuntu 24.04"))
					Expect(ubuntu.SupportedUntil).To(Equal("2028-05-31"))
					Expect(ubuntu.Flavors).To(HaveKey("default"))

					// Hosting plans
					hostingPlans := config.Codesphere.Plans.HostingPlans
					Expect(hostingPlans).To(HaveKey(1))
					hostingPlan := hostingPlans[1]
					Expect(hostingPlan.CPUTenth).To(Equal(10))
					Expect(hostingPlan.MemoryMb).To(Equal(2048))
					Expect(hostingPlan.StorageMb).To(Equal(20480))
					Expect(hostingPlan.TempStorageMb).To(Equal(1024))

					// Workspace plans
					workspacePlans := config.Codesphere.Plans.WorkspacePlans
					Expect(workspacePlans).To(HaveKey(1))
					workspacePlan := workspacePlans[1]
					Expect(workspacePlan.HostingPlanID).To(Equal(1))
					Expect(workspacePlan.OnDemand).To(BeTrue())

					// Managed service backends
					Expect(config.ManagedServiceBackends).ToNot(BeNil())
					Expect(config.ManagedServiceBackends.Postgres).ToNot(BeNil())

					// Managed service config
					Expect(config.Codesphere.ManagedServices).ToNot(BeNil())
					Expect(len(config.Codesphere.ManagedServices)).To(Equal(4))

					// Secrets
					Expect(config.Secrets.BaseDir).To(Equal("/root/secrets"))
				})
			}
		})

		Context("profile-specific differences", func() {
			It("should have the expected datacenter names", func() {
				devManager := installer.NewInstallConfigManager()
				prodManager := installer.NewInstallConfigManager()
				minimalManager := installer.NewInstallConfigManager()

				err := devManager.ApplyProfile(installer.PROFILE_DEV)
				Expect(err).ToNot(HaveOccurred())
				err = prodManager.ApplyProfile(installer.PROFILE_PROD)
				Expect(err).ToNot(HaveOccurred())
				err = minimalManager.ApplyProfile(installer.PROFILE_MINIMAL)
				Expect(err).ToNot(HaveOccurred())

				Expect(devManager.GetInstallConfig().Datacenter.Name).To(Equal("dev"))
				Expect(minimalManager.GetInstallConfig().Datacenter.Name).To(Equal("dev"))
				Expect(prodManager.GetInstallConfig().Datacenter.Name).To(Equal("production"))
			})

			It("should have different resource profiles", func() {
				devManager := installer.NewInstallConfigManager()
				prodManager := installer.NewInstallConfigManager()
				minimalManager := installer.NewInstallConfigManager()

				err := devManager.ApplyProfile(installer.PROFILE_DEV)
				Expect(err).ToNot(HaveOccurred())
				err = prodManager.ApplyProfile(installer.PROFILE_PROD)
				Expect(err).ToNot(HaveOccurred())
				err = minimalManager.ApplyProfile(installer.PROFILE_MINIMAL)
				Expect(err).ToNot(HaveOccurred())

				Expect(devManager.GetInstallConfig().Datacenter.Name).To(Equal("dev"))
				Expect(minimalManager.GetInstallConfig().Datacenter.Name).To(Equal("dev"))
				Expect(prodManager.GetInstallConfig().Datacenter.Name).To(Equal("production"))

				// DEV
				AssertZeroRequests(getAuthServiceRequests(devManager.GetInstallConfig()))
				Expect(devManager.GetInstallConfig().Cluster.Monitoring.Grafana.Enabled).To(BeFalse())
				// Minimal
				AssertZeroRequests(getAuthServiceRequests(minimalManager.GetInstallConfig()))
				Expect(minimalManager.GetInstallConfig().Cluster.Monitoring.Loki.Enabled).To(BeTrue())
				Expect(minimalManager.GetInstallConfig().Cluster.Monitoring.Grafana.Enabled).To(BeTrue())
				Expect(minimalManager.GetInstallConfig().Cluster.Monitoring.GrafanaAlloy.Enabled).To(BeTrue())
				// Prod
				Expect(prodManager.GetInstallConfig().Cluster.Monitoring.Loki.Enabled).To(BeTrue())
				Expect(prodManager.GetInstallConfig().Cluster.Monitoring.Grafana.Enabled).To(BeTrue())
				Expect(prodManager.GetInstallConfig().Cluster.Monitoring.GrafanaAlloy.Enabled).To(BeTrue())
				Expect(prodManager.GetInstallConfig().Codesphere.Override).To(BeNil())

			})
		})
	})

	Describe("Profile Constants", func() {
		It("should have correct profile constant values", func() {
			Expect(installer.PROFILE_DEV).To(Equal("dev"))
			Expect(installer.PROFILE_DEVELOPMENT).To(Equal("development"))
			Expect(installer.PROFILE_PROD).To(Equal("prod"))
			Expect(installer.PROFILE_PRODUCTION).To(Equal("production"))
			Expect(installer.PROFILE_MINIMAL).To(Equal("minimal"))
		})
	})
})

func getAuthServiceRequests(config *files.RootConfig) map[string]int {
	authService := MustMap[any](MustMap[any](MustMap[any](config.Codesphere.Override["global"])["services"])["auth_service"])
	return MustMap[int](authService["requests"])
}
