// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd_test

import (
	"github.com/blang/semver"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"

	"github.com/codesphere-cloud/oms/cli/cmd"
	"github.com/codesphere-cloud/oms/internal/version"
)

type mockOMSUpdater struct{ mock.Mock }

func (m *mockOMSUpdater) Update(v semver.Version, repo string) (semver.Version, string, error) {
	args := m.Called(v, repo)
	return args.Get(0).(semver.Version), args.String(1), args.Error(2)
}

var _ = Describe("Update", func() {
	var (
		mockVersion *version.MockVersion
		mockGit     *mockOMSUpdater
		c           cmd.UpdateOmsCmd
	)

	BeforeEach(func() {
		mockVersion = version.NewMockVersion(GinkgoT())
		mockGit = &mockOMSUpdater{}
		// GitUpdate is a function type; forward calls to the testify mock.
		gitFunc := func(v semver.Version, repo string) (semver.Version, string, error) {
			return mockGit.Update(v, repo)
		}
		c = cmd.UpdateOmsCmd{
			Version: mockVersion,
			Updater: gitFunc,
		}
	})

	It("Detects when current version is latest version", func() {
		v := "0.0.42"
		mockVersion.EXPECT().Version().Return(v)

		mockGit.On("Update", semver.MustParse(v), cmd.GitHubRepo).Return(semver.MustParse(v), "", nil)
		err := c.SelfUpdate()
		Expect(err).NotTo(HaveOccurred())
		mockGit.AssertExpectations(GinkgoT())
	})

	It("Updates when a newer version exists", func() {
		current := "0.0.0"
		latest := "0.0.42"
		mockVersion.EXPECT().Version().Return(current)
		mockGit.On("Update", semver.MustParse(current), cmd.GitHubRepo).Return(semver.MustParse(latest), "notes", nil)
		err := c.SelfUpdate()
		Expect(err).NotTo(HaveOccurred())
		mockGit.AssertExpectations(GinkgoT())
	})
})
