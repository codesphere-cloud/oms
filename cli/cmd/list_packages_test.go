// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd_test

import (
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
	. "github.com/onsi/ginkgo/v2"
	//. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"

	"github.com/codesphere-cloud/oms/cli/cmd"
	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/codesphere-cloud/oms/internal/util"
)

var _ = Describe("ListPackages", func() {

	var (
		mockTableWriter *util.MockTableWriter
		c               cmd.ListBuildsCmd
		internal        bool
		buildDate       time.Time
	)
	JustBeforeEach(func() {
		mockTableWriter = util.NewMockTableWriter(GinkgoT())
		c = cmd.ListBuildsCmd{
			Opts: cmd.ListBuildsOpts{
				Internal: internal,
			},
			TableWriter: mockTableWriter,
		}

		buildDate, _ = time.Parse("2006-01-02", "2025-05-01")

		// header is always appended
		mockTableWriter.EXPECT().AppendHeader(mock.Anything)
		mockTableWriter.EXPECT().Render().Return("")
	})

	Context("Internal packages are excluded", func() {
		It("doesn't list internal packages, artifacts are separated by ,", func() {
			mockTableWriter.EXPECT().AppendRow(
				table.Row{"", "1.42", buildDate, "externalBuild", "installer.tar, installer2.tar"},
			)
			c.PrintPackagesTable(
				portal.Builds{
					Builds: []portal.Build{
						{
							Hash:     "externalBuild",
							Version:  "1.42",
							Date:     buildDate,
							Internal: false,
							Artifacts: []portal.Artifact{
								{Filename: "installer.tar"},
								{Filename: "installer2.tar"},
							},
						},
						{
							Hash:     "internalBuild",
							Internal: true,
						},
					},
				})
		})

	})
	Context("Internal packages are included", func() {
		BeforeEach(func() {
			internal = true
		})
		It("marks internal packages with a *", func() {
			mockTableWriter.EXPECT().AppendRow(
				table.Row{"", "1.42", buildDate, "externalBuild", "installer.tar, installer2.tar"},
			)
			mockTableWriter.EXPECT().AppendRow(
				table.Row{"*", "master", buildDate, "internalBuild", "installer.tar, installer2.tar"},
			)
			c.PrintPackagesTable(
				portal.Builds{
					Builds: []portal.Build{
						{
							Hash:     "externalBuild",
							Version:  "1.42",
							Date:     buildDate,
							Internal: false,
							Artifacts: []portal.Artifact{
								{Filename: "installer.tar"},
								{Filename: "installer2.tar"},
							},
						},
						{
							Hash:     "internalBuild",
							Version:  "master",
							Date:     buildDate,
							Internal: true,
							Artifacts: []portal.Artifact{
								{Filename: "installer.tar"},
								{Filename: "installer2.tar"},
							},
						},
					},
				})
		})
	})
})
