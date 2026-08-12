// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere_test

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/codesphere-cloud/cs-go/api"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/cli/cmd/codesphere"
	intcs "github.com/codesphere-cloud/oms/internal/codesphere"
	"github.com/codesphere-cloud/oms/internal/codesphere/testplan"
)

var _ = Describe("TestCodesphereCmd", func() {
	var (
		mockClient *intcs.MockClient
		opts       *codesphere.TestCodesphereOpts
		out        *bytes.Buffer
		runner     *testplan.Runner
	)

	BeforeEach(func() {
		mockClient = intcs.NewMockClient(GinkgoT())
		out = &bytes.Buffer{}
		opts = &codesphere.TestCodesphereOpts{
			BaseURL:     "https://test.codesphere.com/api",
			Token:       "test-token",
			TeamID:      "123",
			PlanID:      "456",
			Profile:     "ci.yml",
			Quiet:       true, // Suppress log output in tests
			Timeout:     time.Minute,
			WaitTimeout: time.Minute,
			Client:      mockClient,
		}
		runner = &testplan.Runner{Out: out, Quiet: true}
	})

	AfterEach(func() {
		mockClient.AssertExpectations(GinkgoT())
	})

	expectHealthyStatus := func() {
		mockClient.EXPECT().ListWorkspacePlans().Return([]api.WorkspacePlan{{Id: 456, Title: "small"}}, nil).Once()
		mockClient.EXPECT().ListTeams("").Return([]api.Team{{Id: 123, Name: "team"}}, nil).Once()
	}

	Describe("Registry", func() {
		It("offers the status and smoketest tests", func() {
			registry := codesphere.Registry(opts)

			Expect(registry.TestNames()).To(ContainElements(
				codesphere.StatusTestName,
				codesphere.SmoketestTestName,
			))
		})

		It("runs the status test before the smoketest in the default playlist", func() {
			tests, err := codesphere.Registry(opts).SelectPlaylist(codesphere.DefaultPlaylist)

			Expect(err).NotTo(HaveOccurred())
			Expect(tests).To(HaveLen(2))
			Expect(tests[0].Name()).To(Equal(codesphere.StatusTestName))
			Expect(tests[1].Name()).To(Equal(codesphere.SmoketestTestName))
		})

		It("offers a readiness playlist that only checks the status", func() {
			tests, err := codesphere.Registry(opts).SelectPlaylist("readiness")

			Expect(err).NotTo(HaveOccurred())
			Expect(tests).To(HaveLen(1))
			Expect(tests[0].Name()).To(Equal(codesphere.StatusTestName))
		})
	})

	Describe("status test", func() {
		var tests []testplan.Test

		JustBeforeEach(func() {
			var err error

			tests, err = codesphere.Registry(opts).Select([]string{codesphere.StatusTestName})
			Expect(err).NotTo(HaveOccurred())
		})

		It("passes and reports the installation state if the API answers", func() {
			expectHealthyStatus()

			results := runner.Run(context.Background(), tests)

			Expect(testplan.Err(results)).To(BeNil())
			Expect(out.String()).To(ContainSubstring("test.codesphere.com"))
			Expect(out.String()).To(ContainSubstring("Ready"))
		})

		It("fails if the installation is not reachable", func() {
			mockClient.EXPECT().ListWorkspacePlans().Return(nil, fmt.Errorf("connection refused")).Once()

			results := runner.Run(context.Background(), tests)

			Expect(results[0].Status).To(Equal(testplan.StatusFailed))
			Expect(results[0].Err).To(MatchError(ContainSubstring("not ready")))
			Expect(results[0].Err).To(MatchError(ContainSubstring("connection refused")))
		})
	})

	Describe("default playlist", func() {
		It("passes if the installation is ready and the smoketest succeeds", func() {
			expectHealthyStatus()
			mockFullTestRun(mockClient, 123, 456, 789)

			tests, err := codesphere.Registry(opts).SelectPlaylist(codesphere.DefaultPlaylist)
			Expect(err).NotTo(HaveOccurred())

			results := runner.Run(context.Background(), tests)

			Expect(testplan.Err(results)).To(BeNil())
			Expect(results).To(HaveLen(2))
		})

		It("skips the smoketest if the status test fails with fail-fast", func() {
			mockClient.EXPECT().ListWorkspacePlans().Return(nil, fmt.Errorf("connection refused")).Once()

			runner.FailFast = true

			tests, err := codesphere.Registry(opts).SelectPlaylist(codesphere.DefaultPlaylist)
			Expect(err).NotTo(HaveOccurred())

			results := runner.Run(context.Background(), tests)

			Expect(results[0].Status).To(Equal(testplan.StatusFailed))
			Expect(results[1].Name).To(Equal(codesphere.SmoketestTestName))
			Expect(results[1].Status).To(Equal(testplan.StatusSkipped))
			Expect(testplan.Err(results)).To(MatchError(ContainSubstring("status")))
		})
	})
})
