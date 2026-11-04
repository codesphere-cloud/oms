// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/prompt"
)

var _ = Describe("ConfigGeneratorCollector", func() {
	var (
		manager installer.InstallConfigManager
	)

	BeforeEach(func() {
		manager = newPlainInstallConfigManager()
	})

	Describe("CollectInteractively", func() {
		DescribeTable("asks for the Codesphere version and optional hash", func(initial files.CodespherePackageConfig, input string, expected files.CodespherePackageConfig) {
			Expect(manager.ApplyProfile("dev")).To(Succeed())
			cfg := manager.GetInstallConfig()
			cfg.Operations = &files.OperationsConfig{Codesphere: initial, Skip: []string{"ceph"}}
			inputPath := filepath.Join(GinkgoT().TempDir(), "stdin")
			Expect(os.WriteFile(inputPath, []byte(input), 0600)).To(Succeed())
			stdin, err := os.Open(inputPath)
			Expect(err).NotTo(HaveOccurred())

			oldStdin := os.Stdin
			os.Stdin = stdin

			DeferCleanup(func() { os.Stdin = oldStdin; Expect(stdin.Close()).To(Succeed()) })
			Expect(manager.CollectInteractively()).To(Succeed())
			Expect(cfg.Operations.Codesphere).To(Equal(expected))
			Expect(cfg.Operations.Skip).To(Equal([]string{"ceph"}))
		},
			Entry("version with no hash", files.CodespherePackageConfig{}, "1.111.0\n\n", files.CodespherePackageConfig{Version: "1.111.0"}),
			Entry("branch with hash", files.CodespherePackageConfig{}, "master\nabc123\n", files.CodespherePackageConfig{Version: "master", Hash: "abc123"}),
			Entry("accept supplied defaults", files.CodespherePackageConfig{Version: "1.111.0", Hash: "abc123"}, "\n\n", files.CodespherePackageConfig{Version: "1.111.0", Hash: "abc123"}),
			Entry("new version clears previous hash default", files.CodespherePackageConfig{Version: "1.111.0", Hash: "abc123"}, "1.112.0\n\n", files.CodespherePackageConfig{Version: "1.112.0"}),
			Entry("leave optional selection empty", files.CodespherePackageConfig{}, "\n", files.CodespherePackageConfig{}),
		)
		It("should collect configuration after applying profile", func() {
			err := manager.ApplyProfile("dev")
			Expect(err).ToNot(HaveOccurred())

			err = manager.CollectInteractively()
			Expect(err).ToNot(HaveOccurred())

			config := manager.GetInstallConfig()
			Expect(config).ToNot(BeNil())
			Expect(config.Datacenter.Name).ToNot(BeEmpty())
		})

		It("should preserve existing MetalLB pools as prompt defaults", func() {
			err := manager.ApplyProfile(installer.PROFILE_DEV)
			Expect(err).ToNot(HaveOccurred())

			config := manager.GetInstallConfig()
			config.Cluster.MetalLB = &files.MetalLBConfig{
				Enabled: true,
				Pools: []files.MetalLBPoolDef{
					{Name: "existing-pool", IPAddresses: []string{"10.0.0.1-10.0.0.5"}},
				},
			}

			// CollectInteractively uses an interactive prompter; point stdin at an
			// empty file so every prompt resolves to its default value.
			stdin, err := os.CreateTemp("", "stdin-*")
			Expect(err).NotTo(HaveOccurred())

			oldStdin := os.Stdin
			os.Stdin = stdin

			DeferCleanup(func() { os.Stdin = oldStdin; _ = os.Remove(stdin.Name()) })

			err = manager.CollectInteractively()
			Expect(err).ToNot(HaveOccurred())

			config = manager.GetInstallConfig()
			Expect(config.Cluster.MetalLB).ToNot(BeNil())
			Expect(config.Cluster.MetalLB.Enabled).To(BeTrue())
			Expect(config.Cluster.MetalLB.Pools).To(HaveLen(1))
			Expect(config.Cluster.MetalLB.Pools[0].Name).To(Equal("existing-pool"))
			Expect(config.Cluster.MetalLB.Pools[0].IPAddresses).To(Equal([]string{"10.0.0.1-10.0.0.5"}))
		})
	})

	Describe("Prompter", func() {
		var prompter prompt.Prompter

		Context("Non-interactive mode", func() {
			BeforeEach(func() {
				prompter = prompt.NewPrompter(false)
			})

			It("should return default string value", func() {
				result := prompter.String("Test", "default")
				Expect(result).To(Equal("default"))
			})

			It("should return default int value", func() {
				result := prompter.Int("Test", 42)
				Expect(result).To(Equal(42))
			})

			It("should return default bool value", func() {
				result := prompter.Bool("Test", true)
				Expect(result).To(BeTrue())

				result = prompter.Bool("Test", false)
				Expect(result).To(BeFalse())
			})

			It("should return default string slice value", func() {
				defaultVal := []string{"a", "b", "c"}
				result := prompter.StringSlice("Test", defaultVal)
				Expect(result).To(Equal(defaultVal))
			})

			It("should return default choice value", func() {
				result := prompter.Choice("Test", []string{"opt1", "opt2", "opt3"}, "opt2")
				Expect(result).To(Equal("opt2"))
			})

			It("should handle empty default values", func() {
				result := prompter.String("Test", "")
				Expect(result).To(Equal(""))
			})

			It("should handle zero default values", func() {
				result := prompter.Int("Test", 0)
				Expect(result).To(Equal(0))
			})

			It("should handle empty slice defaults", func() {
				result := prompter.StringSlice("Test", []string{})
				Expect(result).To(Equal([]string{}))
			})
		})
	})

	Describe("Configuration Fields After Collection", func() {
		It("should have common configuration properties", func() {
			err := manager.ApplyProfile("prod")
			Expect(err).ToNot(HaveOccurred())
			err = manager.CollectInteractively()
			Expect(err).ToNot(HaveOccurred())

			config := manager.GetInstallConfig()

			// Datacenter
			Expect(config.Datacenter.ID).To(Equal(1))
			Expect(config.Datacenter.City).To(Equal("Karlsruhe"))
			Expect(config.Datacenter.CountryCode).To(Equal("DE"))

			// PostgreSQL
			Expect(config.Postgres.Mode).To(Equal("install"))
			Expect(config.Postgres.Primary).ToNot(BeNil())

			// Kubernetes
			Expect(config.Kubernetes.ManagedByCodesphere).To(BeTrue())
			Expect(config.Kubernetes.NeedsKubeConfig).To(BeFalse())

			// Ceph
			Expect(config.Ceph.Hosts[0].IsMaster).To(BeTrue())

			// Codesphere
			Expect(config.Codesphere.Plans.HostingPlans).To(HaveLen(1))
			Expect(config.Codesphere.Plans.WorkspacePlans).To(HaveLen(1))
		})
	})
})
