// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"

	"github.com/codesphere-cloud/oms/cli/cmd"
	"github.com/codesphere-cloud/oms/internal/portal"
)

var _ = Describe("RevokeCmd", func() {
	var (
		mockPortal *portal.MockPortal
		c          cmd.RevokeAPIKeyCmd
		key        string
	)

	BeforeEach(func() {
		mockPortal = portal.NewMockPortal(GinkgoT())
		key = "test-key"
		c = cmd.RevokeAPIKeyCmd{
			Opts: cmd.RevokeAPIKeyOpts{
				ID: key,
			},
		}
	})

	Context("when revoking API key succeeds", func() {
		It("returns nil error", func() {
			mockPortal.EXPECT().RevokeAPIKey(key).Return(nil)
			err := c.Revoke(mockPortal)
			Expect(err).To(BeNil())
		})
	})

	Context("when revoking API key fails", func() {
		It("returns error", func() {
			mockPortal.EXPECT().RevokeAPIKey(key).Return(fmt.Errorf("some error"))
			err := c.Revoke(mockPortal)
			Expect(err).To(MatchError(ContainSubstring("failed to revoke API key")))
		})
	})
})

var _ = Describe("AddRevokeAPIKeyCmd", func() {
	It("adds the api-key command to the parent", func() {
		parent := &cobra.Command{}
		opts := cmd.GlobalOptions{}
		cmd.AddRevokeAPIKeyCmd(parent, opts)
		found := false
		for _, c := range parent.Commands() {
			if c.Use == "api-key" {
				found = true
				break
			}
		}
		Expect(found).To(BeTrue())
	})
})
