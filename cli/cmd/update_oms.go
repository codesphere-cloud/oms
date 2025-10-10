// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"

	"github.com/blang/semver"
	"github.com/rhysd/go-github-selfupdate/selfupdate"
	"github.com/spf13/cobra"

	"github.com/codesphere-cloud/oms/internal/version"
)

const GitHubRepo = "codesphere-cloud/oms"

type OMSUpdater func(v semver.Version, repo string) (semver.Version, string, error)

var OMSSelfUpdater OMSUpdater = func(v semver.Version, repo string) (semver.Version, string, error) {
	latest, err := selfupdate.UpdateSelf(v, repo)
	if err != nil {
		return v, "", err
	}

	return latest.Version, latest.ReleaseNotes, nil
}

type UpdateOmsCmd struct {
	Version version.Version
	Updater OMSUpdater
}

func AddOmsUpdateCmd(parentCmd *cobra.Command) {
	cmdState := &UpdateOmsCmd{
		Version: &version.Build{},
		Updater: OMSSelfUpdater,
	}

	omsCmd := &cobra.Command{
		Use:   "oms",
		Short: "Update the OMS CLI",
		Long:  `Updates the OMS CLI to the latest release from GitHub.`,
		RunE: func(_ *cobra.Command, args []string) error {
			return cmdState.SelfUpdate()
		},
	}
	parentCmd.AddCommand(omsCmd)
}
func (c *UpdateOmsCmd) SelfUpdate() error {
	v := semver.MustParse(c.Version.Version())
	latestVersion, releaseNotes, err := c.Updater(v, GitHubRepo)
	if err != nil {
		return fmt.Errorf("update failed: %w", err)
	}

	if latestVersion.Equals(v) {
		fmt.Println("Current OMS CLI is the latest version", c.Version.Version())
		return nil
	}

	fmt.Printf("Successfully updated from %s to %s\n", v.String(), latestVersion.String())
	fmt.Println("Release notes:\n", releaseNotes)

	return nil
}
