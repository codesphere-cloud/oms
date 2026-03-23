// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer"
)

var _ = Describe("ConfigGeneratorCollector", func() {
	var (
		manager installer.InstallConfigManager
	)

	BeforeEach(func() {
		manager = installer.NewInstallConfigManager()
	})

	Describe("CollectInteractively", func() {
		It("should collect configuration after applying profile", func() {
			err := manager.ApplyProfile("dev")
			Expect(err).ToNot(HaveOccurred())

			err = manager.CollectInteractively()
			Expect(err).ToNot(HaveOccurred())

			config := manager.GetInstallConfig()
			Expect(config).ToNot(BeNil())
			Expect(config.Datacenter.Name).ToNot(BeEmpty())
		})
	})

	Describe("Prompter", func() {
		var prompter *installer.Prompter

		Context("Non-interactive mode", func() {
			BeforeEach(func() {
				prompter = installer.NewPrompter(false)
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
