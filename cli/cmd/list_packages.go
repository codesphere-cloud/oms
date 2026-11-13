// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"

	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/codesphere-cloud/oms/internal/portal"
	intutil "github.com/codesphere-cloud/oms/internal/util"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"
)

// ListBuildsCmd implements the list packages command.
type ListBuildsCmd struct {
	cmd         *cobra.Command
	Opts        ListBuildsOpts
	TableWriter intutil.TableWriter
}

// ListBuildsOpts holds the flags of the list packages command.
type ListBuildsOpts struct {
	*util.GlobalOptions
	Internal bool
	Sort     string
}

// RunE lists the available packages from the portal.
func (c *ListBuildsCmd) RunE(_ *cobra.Command, _ []string) error {
	if c.Opts.Sort != portal.SortSemver && c.Opts.Sort != portal.SortDate {
		return fmt.Errorf("invalid sort parameter: %s (must be '%s' or '%s')", c.Opts.Sort, portal.SortSemver, portal.SortDate)
	}

	p := portal.NewPortalClient(c.cmd.Context())

	packages, err := p.ListBuilds(portal.CodesphereProduct, c.Opts.Sort)
	if err != nil {
		return fmt.Errorf("failed to list codesphere packages: %w", err)
	}

	c.PrintPackagesTable(packages)

	return nil
}

// AddListPackagesCmd registers the packages command on the list command.
func AddListPackagesCmd(list *cobra.Command, opts *util.GlobalOptions) {
	builds := ListBuildsCmd{
		cmd: &cobra.Command{
			Use:   "packages",
			Short: "List available packages",
			Long:  io.Long(`List packages available for download via the OMS portal.`),
		},
		Opts:        ListBuildsOpts{GlobalOptions: opts},
		TableWriter: intutil.GetTableWriter(),
	}

	builds.cmd.RunE = builds.RunE
	builds.cmd.Flags().BoolVarP(&builds.Opts.Internal, "list-internal", "i", false, "List internal packages")
	_ = builds.cmd.Flags().MarkHidden("list-internal")
	builds.cmd.Flags().StringVarP(&builds.Opts.Sort, "sort", "s", portal.SortSemver, "Sort order: 'semver' (by semantic version) or 'date' (by build date)")

	util.AddCmd(list, builds.cmd)
}

// PrintPackagesTable renders the given packages as a table, hiding internal builds unless requested.
func (c *ListBuildsCmd) PrintPackagesTable(packages portal.Builds) {
	c.TableWriter.AppendHeader(table.Row{"Int", "Version", "Build Date", "Hash", "Artifacts"})

	for _, build := range packages.Builds {
		if !c.Opts.Internal && build.Internal {
			continue
		}

		internal := ""
		if build.Internal {
			internal = "*"
		}

		artifacts := ""

		for i, art := range build.Artifacts {
			if i > 0 {
				artifacts += ", "
			}

			artifacts = artifacts + art.Filename
		}

		c.TableWriter.AppendRow(table.Row{internal, build.Version, build.Date, build.Hash, artifacts})
	}

	c.TableWriter.Render()
}
