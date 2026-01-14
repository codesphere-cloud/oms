// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"fmt"
	"log"

	"github.com/creativeprojects/go-selfupdate"
	"github.com/spf13/cobra"

	"github.com/codesphere-cloud/oms/internal/version"
)

const GitHubRepo = "codesphere-cloud/oms"

type OMSUpdater interface {
	Update(ctx context.Context, current string, repo selfupdate.Repository) (string, string, error)
}

type OMSSelfUpdater struct{}

func (s *OMSSelfUpdater) Update(ctx context.Context, current string, repo selfupdate.Repository) (string, string, error) {
	latest, found, err := selfupdate.DetectLatest(ctx, repo)
	if err != nil {
		return current, "", err
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
