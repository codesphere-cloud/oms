// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("laterPhaseOpts", func() {
	executableSteps := func(opts *InstallCodesphereOpts, allowedSteps []string) []string {
		ci := &installer.CodesphereInstaller{SkipSteps: opts.SkipSteps, AllowedSteps: allowedSteps}
		return ci.ExecutableSteps(files.RootConfig{})
	}

	It("given a full install, when the platform phase runs, then it only installs Codesphere", func() {
		opts := &InstallCodesphereOpts{}

		Expect(executableSteps(laterPhaseOpts(opts), installer.PlatformSteps)).To(Equal([]string{"codesphere"}))
	})

	It("given user skip steps, when the dependencies phase runs, then they are still skipped", func() {
		opts := &InstallCodesphereOpts{SkipSteps: []string{"ms-backends"}}

		Expect(executableSteps(laterPhaseOpts(opts), installer.DependenciesSteps)).To(Equal([]string{"set-up-cluster"}))
	})

	It("given options for the infrastructure phase, when the later phase options are derived, then the original skip steps are unchanged", func() {
		opts := &InstallCodesphereOpts{SkipSteps: []string{"ceph"}}

		laterPhaseOpts(opts)

		Expect(opts.SkipSteps).To(Equal([]string{"ceph"}))
	})
})
