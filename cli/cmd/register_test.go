// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"

	"github.com/codesphere-cloud/oms/cli/cmd"
	"github.com/codesphere-cloud/oms/internal/portal"
)

var _ = Describe("RegisterCmd", func() {
	var (
		mockPortal   *portal.MockPortal
		c            cmd.RegisterCmd
		expiresAt    string
		owner        string
		organization string
		role         string
	)

	BeforeEach(func() {
		mockPortal = portal.NewMockPortal(GinkgoT())
		expiresAt = "2025-05-01T15:04:05Z"
		owner = "test-owner"
		organization = "test-org"
		role = "Admin"
		c = cmd.RegisterCmd{
			Opts: cmd.RegisterOpts{
				Owner:        owner,
				Organization: organization,
				Role:         role,
				ExpiresAt:    expiresAt,
			},
		}
	})

	Context("when expiration date is valid", func() {
		It("registers the API key successfully", func() {
			parsedTime, _ := time.Parse(time.RFC3339, expiresAt)
			mockPortal.EXPECT().RegisterAPIKey(owner, organization, role, parsedTime).Return(&portal.ApiKey{}, nil)
			ak, err := c.Register(mockPortal)
			Expect(err).To(BeNil())
			Expect(ak).NotTo(BeNil())
		})

		It("returns error if Register fails", func() {
			parsedTime, _ := time.Parse(time.RFC3339, expiresAt)
			mockPortal.EXPECT().RegisterAPIKey(owner, organization, role, parsedTime).Return((*portal.ApiKey)(nil), fmt.Errorf("some error"))
			ak, err := c.Register(mockPortal)
			Expect(ak).To(BeNil())
			Expect(err).To(MatchError(ContainSubstring("failed to register API key")))
		})
	})

	Context("when expiration date is invalid", func() {
		It("returns error for invalid expiration date", func() {
			c.Opts.ExpiresAt = "invalid-date"
			ak, err := c.Register(mockPortal)
			Expect(ak).To(BeNil())
			Expect(err).To(MatchError(ContainSubstring("failed to parse expiration date")))
		})
	})

	Context("when role is valid", func() {
		It("accepts Admin role", func() {
			c.Opts.Role = "Admin"
			parsedTime, _ := time.Parse(time.RFC3339, expiresAt)

			mockPortal.EXPECT().RegisterAPIKey(owner, organization, "Admin", parsedTime).Return(&portal.ApiKey{}, nil)

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
