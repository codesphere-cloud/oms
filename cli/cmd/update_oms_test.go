// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd_test

import (
	"context"

	"github.com/creativeprojects/go-selfupdate"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"

	"github.com/codesphere-cloud/oms/cli/cmd"
	"github.com/codesphere-cloud/oms/internal/version"
)

type mockOMSUpdater struct{ mock.Mock }

func (m *mockOMSUpdater) Update(ctx context.Context, current string, repo selfupdate.Repository) (string, string, error) {
	args := m.Called(ctx, current, repo)
	return args.String(0), args.String(1), args.Error(2)
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
		c = cmd.UpdateOmsCmd{
			Version: mockVersion,
			Updater: mockGit,
		}
	})

	It("Detects when current version is latest version", func() {
		v := "0.0.42"
		mockVersion.EXPECT().Version().Return(v)

		mockGit.On("Update", mock.Anything, v, selfupdate.ParseSlug(cmd.GitHubRepo)).Return(v, "", nil)
		err := c.SelfUpdate()
		Expect(err).NotTo(HaveOccurred())
		mockGit.AssertExpectations(GinkgoT())
	})

	It("Updates when a newer version exists", func() {
		current := "0.0.0"
		latest := "0.0.42"
		mockVersion.EXPECT().Version().Return(current)
		mockGit.On("Update", mock.Anything, current, selfupdate.ParseSlug(cmd.GitHubRepo)).Return(latest, "notes", nil)
		err := c.SelfUpdate()
		Expect(err).NotTo(HaveOccurred())
		mockGit.AssertExpectations(GinkgoT())
	})
})
