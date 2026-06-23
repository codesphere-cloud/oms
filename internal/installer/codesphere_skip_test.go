// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Codesphere skip steps", func() {
	It("detects persisted and CLI skip steps", func() {
		config := files.RootConfig{
			Operations: &files.OperationsConfig{
				Skip: []string{installer.ArgoCDStep},
			},
		}

		Expect(installer.IsStepSkipped(config, nil, installer.ArgoCDStep)).To(BeTrue())
		Expect(installer.IsStepSkipped(files.RootConfig{}, []string{"ms-backends"}, "ms-backends")).To(BeTrue())
		Expect(installer.IsStepSkipped(config, nil, "set-up-cluster")).To(BeFalse())
	})

	It("only applies skips to executable private-cloud installer steps", func() {
		executableSteps := map[string]bool{
			"set-up-cluster": true,
			"ms-backends":    true,
		}
		config := files.RootConfig{
			Operations: &files.OperationsConfig{
				Skip: []string{installer.ArgoCDStep, "set-up-cluster"},
			},
		}

		installer.ApplySkippedSteps(executableSteps, config, []string{"unknown-step", "ms-backends"})

		Expect(executableSteps).To(Equal(map[string]bool{
			"set-up-cluster": false,
			"ms-backends":    false,
		}))
		Expect(executableSteps).NotTo(HaveKey(installer.ArgoCDStep))
		Expect(executableSteps).NotTo(HaveKey("unknown-step"))
	})

	It("detects when an allowed step set has no executable steps left", func() {
		config := files.RootConfig{
			Operations: &files.OperationsConfig{
				Skip: []string{"copy-dependencies", "set-up-cluster"},
			},
		}

		ci := &installer.CodesphereInstaller{
			AllowedSteps: installer.DependenciesSteps,
			SkipSteps:    []string{"extract-dependencies", "ms-backends"},
		}

		Expect(ci.HasExecutableSteps(config)).To(BeFalse())
	})

	It("returns executable steps from known steps filtered by allowed and skipped steps", func() {
		config := files.RootConfig{
			Operations: &files.OperationsConfig{
				Skip: []string{"extract-dependencies", "unknown-step"},
			},
		}

		ci := &installer.CodesphereInstaller{
			AllowedSteps: installer.InfraSteps,
			SkipSteps:    []string{"docker", installer.ArgoCDStep},
		}

		Expect(ci.ExecutableSteps(config)).To(Equal([]string{
			"copy-dependencies",
			"load-container-images",
			"sops",
			"postgres",
			"ceph",
			"kubernetes",
		}))
	})

	It("returns no executable steps when all known allowed steps are skipped", func() {
		config := files.RootConfig{
			Operations: &files.OperationsConfig{
				Skip: []string{"copy-dependencies", "set-up-cluster"},
			},
		}

		ci := &installer.CodesphereInstaller{
			AllowedSteps: installer.DependenciesSteps,
			SkipSteps:    []string{"extract-dependencies", "ms-backends", installer.ArgoCDStep},
		}

		Expect(ci.ExecutableSteps(config)).To(BeEmpty())
	})
})
