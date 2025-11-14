// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer"
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

		Context("with prod profile", func() {
			It("should configure HA multi-node setup", func() {
				err := manager.ApplyProfile(installer.PROFILE_PROD)
				Expect(err).ToNot(HaveOccurred())

				config := manager.GetInstallConfig()
				Expect(config.Datacenter.Name).To(Equal("production"))
				Expect(config.Postgres.Primary.IP).To(Equal("10.50.0.2"))
				Expect(config.Postgres.Primary.Hostname).To(Equal("pg-primary"))
				Expect(config.Postgres.Replica).ToNot(BeNil())
				Expect(config.Postgres.Replica.IP).To(Equal("10.50.0.3"))
			})

			It("should configure multiple Ceph nodes", func() {
				err := manager.ApplyProfile(installer.PROFILE_PROD)
				Expect(err).ToNot(HaveOccurred())

				config := manager.GetInstallConfig()
				Expect(config.Ceph.Hosts).To(HaveLen(3))
				Expect(config.Ceph.Hosts[0].IsMaster).To(BeTrue())
				Expect(config.Ceph.Hosts[1].IsMaster).To(BeFalse())
				Expect(config.Ceph.Hosts[2].IsMaster).To(BeFalse())
			})

			It("should configure multiple Kubernetes nodes", func() {
				err := manager.ApplyProfile(installer.PROFILE_PROD)
				Expect(err).ToNot(HaveOccurred())

				config := manager.GetInstallConfig()
				Expect(config.Kubernetes.ControlPlanes).To(HaveLen(1))
				Expect(config.Kubernetes.Workers).To(HaveLen(3))
			})

			It("should use production domain names", func() {
				err := manager.ApplyProfile(installer.PROFILE_PROD)
				Expect(err).ToNot(HaveOccurred())

				config := manager.GetInstallConfig()
				Expect(config.Codesphere.Domain).To(Equal("codesphere.yourcompany.com"))
				Expect(config.Codesphere.WorkspaceHostingBaseDomain).To(Equal("ws.yourcompany.com"))
			})
		})

		Context("with production profile", func() {
			It("should apply production profile as alias for prod", func() {
				err := manager.ApplyProfile(installer.PROFILE_PRODUCTION)
				Expect(err).ToNot(HaveOccurred())

				config := manager.GetInstallConfig()
				Expect(config.Datacenter.Name).To(Equal("production"))
			})
		})

		Context("with minimal profile", func() {
			It("should configure minimal single-node setup", func() {
				err := manager.ApplyProfile(installer.PROFILE_MINIMAL)
				Expect(err).ToNot(HaveOccurred())

				config := manager.GetInstallConfig()
				Expect(config.Datacenter.Name).To(Equal("minimal"))
				Expect(config.Postgres.Primary.IP).To(Equal("127.0.0.1"))
				Expect(config.Kubernetes.Workers).To(BeEmpty())
			})

			It("should configure minimal workspace plan", func() {
				err := manager.ApplyProfile(installer.PROFILE_MINIMAL)
				Expect(err).ToNot(HaveOccurred())

				config := manager.GetInstallConfig()
				plan := config.Codesphere.Plans.WorkspacePlans[1]
				Expect(plan.MaxReplicas).To(Equal(1))
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
				It("should set common configuration for "+profile, func() {
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

					// Secrets
					Expect(config.Secrets.BaseDir).To(Equal("/root/secrets"))
				})

				It("should configure Ceph OSDs for "+profile, func() {
					err := manager.ApplyProfile(profile)
					Expect(err).ToNot(HaveOccurred())

					config := manager.GetInstallConfig()
					Expect(config.Ceph.OSDs).To(HaveLen(1))
					osd := config.Ceph.OSDs[0]
					Expect(osd.SpecID).To(Equal("default"))
					Expect(osd.Placement.HostPattern).To(Equal("*"))
					Expect(osd.DataDevices.Size).To(Equal("240G:300G"))
					Expect(osd.DataDevices.Limit).To(Equal(1))
					Expect(osd.DBDevices.Size).To(Equal("120G:150G"))
					Expect(osd.DBDevices.Limit).To(Equal(1))
				})

				It("should configure workspace images for "+profile, func() {
					err := manager.ApplyProfile(profile)
					Expect(err).ToNot(HaveOccurred())

					config := manager.GetInstallConfig()
					Expect(config.Codesphere.WorkspaceImages).ToNot(BeNil())
					Expect(config.Codesphere.WorkspaceImages.Agent).ToNot(BeNil())
					Expect(config.Codesphere.WorkspaceImages.Agent.BomRef).To(Equal("workspace-agent-24.04"))
				})

				It("should configure deploy config for "+profile, func() {
					err := manager.ApplyProfile(profile)
					Expect(err).ToNot(HaveOccurred())

					config := manager.GetInstallConfig()
					images := config.Codesphere.DeployConfig.Images
					Expect(images).To(HaveKey("ubuntu-24.04"))
					ubuntu := images["ubuntu-24.04"]
					Expect(ubuntu.Name).To(Equal("Ubuntu 24.04"))
					Expect(ubuntu.SupportedUntil).To(Equal("2028-05-31"))
					Expect(ubuntu.Flavors).To(HaveKey("default"))
				})

				It("should configure hosting plans for "+profile, func() {
					err := manager.ApplyProfile(profile)
					Expect(err).ToNot(HaveOccurred())

					config := manager.GetInstallConfig()
					hostingPlans := config.Codesphere.Plans.HostingPlans
					Expect(hostingPlans).To(HaveKey(1))
					plan := hostingPlans[1]
					Expect(plan.CPUTenth).To(Equal(10))
					Expect(plan.MemoryMb).To(Equal(2048))
					Expect(plan.StorageMb).To(Equal(20480))
					Expect(plan.TempStorageMb).To(Equal(1024))
				})

				It("should configure workspace plans for "+profile, func() {
					err := manager.ApplyProfile(profile)
					Expect(err).ToNot(HaveOccurred())

					config := manager.GetInstallConfig()
					workspacePlans := config.Codesphere.Plans.WorkspacePlans
					Expect(workspacePlans).To(HaveKey(1))
					plan := workspacePlans[1]
					Expect(plan.HostingPlanID).To(Equal(1))
					Expect(plan.OnDemand).To(BeTrue())
				})

				It("should configure managed service backends for "+profile, func() {
					err := manager.ApplyProfile(profile)
					Expect(err).ToNot(HaveOccurred())

					config := manager.GetInstallConfig()
					Expect(config.ManagedServiceBackends).ToNot(BeNil())
					Expect(config.ManagedServiceBackends.Postgres).ToNot(BeNil())
				})
			}
		})

		Context("profile-specific differences", func() {
			It("should have different datacenter names", func() {
				devManager := installer.NewInstallConfigManager()
				prodManager := installer.NewInstallConfigManager()
				minimalManager := installer.NewInstallConfigManager()

				devManager.ApplyProfile(installer.PROFILE_DEV)
				prodManager.ApplyProfile(installer.PROFILE_PROD)
				minimalManager.ApplyProfile(installer.PROFILE_MINIMAL)

				Expect(devManager.GetInstallConfig().Datacenter.Name).To(Equal("dev"))
				Expect(prodManager.GetInstallConfig().Datacenter.Name).To(Equal("production"))
				Expect(minimalManager.GetInstallConfig().Datacenter.Name).To(Equal("minimal"))
			})

			It("should have different PostgreSQL replica configurations", func() {
				devManager := installer.NewInstallConfigManager()
				prodManager := installer.NewInstallConfigManager()

				devManager.ApplyProfile(installer.PROFILE_DEV)
				prodManager.ApplyProfile(installer.PROFILE_PROD)

				Expect(prodManager.GetInstallConfig().Postgres.Replica.IP).To(Equal("10.50.0.3"))
			})

			It("should have different number of Ceph hosts", func() {
				devManager := installer.NewInstallConfigManager()
				prodManager := installer.NewInstallConfigManager()

				devManager.ApplyProfile(installer.PROFILE_DEV)
				prodManager.ApplyProfile(installer.PROFILE_PROD)

				Expect(devManager.GetInstallConfig().Ceph.Hosts).To(HaveLen(1))
				Expect(prodManager.GetInstallConfig().Ceph.Hosts).To(HaveLen(3))
			})

			It("should have different worker node counts", func() {
				devManager := installer.NewInstallConfigManager()
				prodManager := installer.NewInstallConfigManager()
				minimalManager := installer.NewInstallConfigManager()

				devManager.ApplyProfile(installer.PROFILE_DEV)
				prodManager.ApplyProfile(installer.PROFILE_PROD)
				minimalManager.ApplyProfile(installer.PROFILE_MINIMAL)

				Expect(devManager.GetInstallConfig().Kubernetes.Workers).To(HaveLen(1))
				Expect(prodManager.GetInstallConfig().Kubernetes.Workers).To(HaveLen(3))
				Expect(minimalManager.GetInstallConfig().Kubernetes.Workers).To(BeEmpty())
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
