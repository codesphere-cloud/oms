// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"fmt"
	"log"

	"github.com/creativeprojects/go-selfupdate"
	"github.com/spf13/cobra"

	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/codesphere-cloud/oms/internal/version"
)

// GitHubRepo is the GitHub repository that OMS CLI releases are fetched from.
const GitHubRepo = "codesphere-cloud/oms"

// OMSUpdater updates the OMS CLI binary to the latest release.
type OMSUpdater interface {
	Update(ctx context.Context, current string, repo selfupdate.Repository) (string, string, error)
}

// OMSSelfUpdater is the production OMSUpdater that replaces the running binary with the latest GitHub release.
type OMSSelfUpdater struct{}

// Update installs the latest release if it is newer than current and returns the resulting version and its release notes.
func (s *OMSSelfUpdater) Update(ctx context.Context, current string, repo selfupdate.Repository) (string, string, error) {
	latest, found, err := selfupdate.DetectLatest(ctx, repo)
	if err != nil {
		return current, "", fmt.Errorf("failed to detect latest release: %w", err)
	}

	if !found {
		return current, "", fmt.Errorf("latest version could not be found from GitHub repository")
	}

	if latest.LessOrEqual(current) {
		return current, "", nil
	}

	exe, err := selfupdate.ExecutablePath()
	if err != nil {
		return current, "", fmt.Errorf("could not locate executable path: %w", err)
	}

	if err := selfupdate.UpdateTo(ctx, latest.AssetURL, latest.AssetName, exe); err != nil {
		return current, "", fmt.Errorf("error occurred while updating binary: %w", err)
	}

	return latest.Version(), latest.ReleaseNotes, nil
}

// UpdateOmsCmd implements the update oms command.
type UpdateOmsCmd struct {
	Version version.Version
	Updater OMSUpdater
}

// AddOmsUpdateCmd registers the oms command on the update command.
func AddOmsUpdateCmd(parentCmd *cobra.Command) {
	cmdState := &UpdateOmsCmd{
		Version: &version.Build{},
		Updater: &OMSSelfUpdater{},
	}

	omsCmd := &cobra.Command{
		Use:   "oms",
		Short: "Update the OMS CLI",
		Long:  `Updates the OMS CLI to the latest release from GitHub.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return cmdState.SelfUpdate()
		},
	}
	util.AddCmd(parentCmd, omsCmd)
}

// SelfUpdate updates the OMS CLI to the latest release and prints the release notes.
func (c *UpdateOmsCmd) SelfUpdate() error {
	ctx := context.Background()
	currentVersion := c.Version.Version()
	repo := selfupdate.ParseSlug(GitHubRepo)

	latestVersion, releaseNotes, err := c.Updater.Update(ctx, currentVersion, repo)
	if err != nil {
		return fmt.Errorf("update failed: %w", err)
	}

	if latestVersion == currentVersion {
		log.Println("Current OMS CLI is the latest version", currentVersion)
		return nil
	}

	log.Printf("Successfully updated from %s to %s\n", currentVersion, latestVersion)
	log.Println("Release notes:\n", releaseNotes)

	return nil
}
