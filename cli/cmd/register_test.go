// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/mock"

	"github.com/codesphere-cloud/oms/cli/cmd"
	"github.com/codesphere-cloud/oms/internal/portal"
)

var _ = Describe("RegisterCmd", func() {
	var (
		mockPortal   *portal.MockPortal
		c            cmd.RegisterCmd
		validFor     string
		owner        string
		organization string
		role         string
	)

	BeforeEach(func() {
		mockPortal = portal.NewMockPortal(GinkgoT())
		validFor = "10d"
		owner = "test-owner"
		organization = "test-org"
		role = cmd.API_KEY_ROLE_ADMIN
		c = cmd.RegisterCmd{
			Opts: cmd.RegisterOpts{
				Owner:        owner,
				Organization: organization,
				Role:         role,
				ValidFor:     validFor,
			},
		}
	})

	Context("when valid-for duration is valid", func() {
		It("registers the API key successfully", func() {
			start := time.Now()
			mockPortal.EXPECT().RegisterAPIKey(
				owner,
				organization,
				role,
				mock.MatchedBy(func(expiresAt time.Time) bool {
					expected := start.AddDate(0, 0, 10)
					// Allow a small delta to avoid flakiness from runtime overhead.
					return expiresAt.After(expected.Add(-2*time.Second)) && expiresAt.Before(expected.Add(2*time.Second))
				}),
			).Return(&portal.ApiKey{}, nil)
			ak, err := c.Register(mockPortal)
			Expect(err).To(BeNil())
			Expect(ak).NotTo(BeNil())
		})

		It("returns error if Register fails", func() {
			mockPortal.EXPECT().RegisterAPIKey(owner, organization, role, mock.Anything).Return((*portal.ApiKey)(nil), fmt.Errorf("some error"))
			ak, err := c.Register(mockPortal)
			Expect(ak).To(BeNil())
			Expect(err).To(MatchError(ContainSubstring("failed to register API key")))
		})
	})

	Context("when valid-for duration is invalid", func() {
		It("returns error for invalid valid-for duration", func() {
			c.Opts.ValidFor = "invalid-date"
			ak, err := c.Register(mockPortal)
			Expect(ak).To(BeNil())
			Expect(err).To(MatchError(ContainSubstring("failed to parse valid-for duration")))
		})
	})

	Context("when role is valid", func() {
		It("accepts Admin role", func() {
			c.Opts.Role = cmd.API_KEY_ROLE_ADMIN
			mockPortal.EXPECT().RegisterAPIKey(owner, organization, cmd.API_KEY_ROLE_ADMIN, mock.Anything).Return(&portal.ApiKey{}, nil)

			ak, err := c.Register(mockPortal)
			Expect(err).To(BeNil())
			Expect(ak).NotTo(BeNil())
		})

		It("accepts Dev role", func() {
			c.Opts.Role = cmd.API_KEY_ROLE_DEV
			mockPortal.EXPECT().RegisterAPIKey(owner, organization, cmd.API_KEY_ROLE_DEV, mock.Anything).Return(&portal.ApiKey{}, nil)

			ak, err := c.Register(mockPortal)
			Expect(err).To(BeNil())
			Expect(ak).NotTo(BeNil())
		})

		It("accepts Ext role", func() {
			c.Opts.Role = cmd.API_KEY_ROLE_EXT
			mockPortal.EXPECT().RegisterAPIKey(owner, organization, cmd.API_KEY_ROLE_EXT, mock.Anything).Return(&portal.ApiKey{}, nil)

			ak, err := c.Register(mockPortal)
			Expect(err).To(BeNil())
			Expect(ak).NotTo(BeNil())
		})
	})

	Context("when valid-for duration is not provided", func() {
		It("passes zero expiration time to portal client", func() {
			c.Opts.ValidFor = ""
			mockPortal.EXPECT().RegisterAPIKey(owner, organization, role, time.Time{}).Return(&portal.ApiKey{}, nil)

			ak, err := c.Register(mockPortal)
			Expect(err).To(BeNil())
			Expect(ak).NotTo(BeNil())
		})
	})

	Context("when role is invalid", func() {
		It("returns error for invalid role", func() {
			c.Opts.Role = "InvalidRole"
			ak, err := c.Register(mockPortal)
			Expect(ak).To(BeNil())
			Expect(err).To(MatchError(ContainSubstring("invalid role: InvalidRole")))
		})
	})
})

var _ = Describe("AddRegisterCmd", func() {
	It("adds the register command to the parent", func() {
		parent := &cobra.Command{}
		opts := &cmd.GlobalOptions{}
		cmd.AddRegisterCmd(parent, opts)
		found := false
		for _, c := range parent.Commands() {
			if c.Use == "register" {
				found = true
				break
			}
		}
		Expect(found).To(BeTrue())
	})
})
