// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"log"

	"github.com/blang/semver"
	"github.com/rhysd/go-github-selfupdate/selfupdate"
	"github.com/spf13/cobra"

	"github.com/codesphere-cloud/oms/internal/version"
)

const GitHubRepo = "codesphere-cloud/oms"

type OMSUpdater interface {
	Update(v semver.Version, repo string) (semver.Version, string, error)
}

type OMSSelfUpdater struct{}

func (s *OMSSelfUpdater) Update(v semver.Version, repo string) (semver.Version, string, error) {
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
		Updater: &OMSSelfUpdater{},
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
	latestVersion, releaseNotes, err := c.Updater.Update(v, GitHubRepo)
	if err != nil {
		return fmt.Errorf("update failed: %w", err)
	}

	if latestVersion.Equals(v) {
		log.Println("Current OMS CLI is the latest version", c.Version.Version())
		return nil
	}

	log.Printf("Successfully updated from %s to %s\n", v.String(), latestVersion.String())
	log.Println("Release notes:\n", releaseNotes)

	return nil
}
