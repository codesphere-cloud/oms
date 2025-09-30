// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"io"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/blang/semver"
	"github.com/inconshreveable/go-update"
	"github.com/spf13/cobra"

	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/codesphere-cloud/oms/internal/util"
	"github.com/codesphere-cloud/oms/internal/version"
)

type UpdateCmd struct {
	cmd     *cobra.Command
	Version version.Version
	Updater Updater
}

func (c *UpdateCmd) RunE(_ *cobra.Command, args []string) error {

	p := portal.NewPortalClient()

	return c.SelfUpdate(p)
}

func AddUpdateCmd(rootCmd *cobra.Command) {
	update := UpdateCmd{
		cmd: &cobra.Command{
			Use:   "update",
			Short: "Update Codesphere OMS",
			Long:  `Updates the OMS to the latest release from OMS Portal.`,
		},
		Version: &version.Build{},
		Updater: &SelfUpdater{},
	}
	rootCmd.AddCommand(update.cmd)
	update.cmd.RunE = update.RunE
}

func (c *UpdateCmd) SelfUpdate(p portal.Portal) error {
	currentVersion := semver.MustParse(c.Version.Version())

	latest, err := p.GetBuild(portal.OmsProduct, "", "")
	if err != nil {
		return fmt.Errorf("failed to query OMS Portal for latest version: %w", err)
	}
	latestVersion := semver.MustParse(strings.TrimPrefix(latest.Version, "oms-v"))

	fmt.Printf("current version: %v\n", currentVersion)
	fmt.Printf("latest version: %v\n", latestVersion)
	if latestVersion.Equals(currentVersion) {
		fmt.Println("Current OMS CLI is already the latest version", c.Version.Version())
		return nil
	}

	// Need a build with a single artifact to download it
	download, err := latest.GetBuildForDownload(fmt.Sprintf("%s_%s.tar.gz", c.Version.Os(), c.Version.Arch()))
	if err != nil {
		return fmt.Errorf("failed to find OMS CLI in package: %w", err)
	}

	// Use a pipe to unzip the file while downloading without storing on the filesystem
	reader, writer := io.Pipe()
	defer func() { _ = reader.Close() }()

	eg := errgroup.Group{}
	eg.Go(func() error {
		defer func() { _ = writer.Close() }()
		err = p.DownloadBuildArtifact(portal.OmsProduct, download, writer)
		if err != nil {
			return fmt.Errorf("failed to download latest OMS package: %w", err)
		}
		return nil
	})

	cliReader, err := util.StreamFileFromGzip(reader, "oms-cli")
	if err != nil {
		return fmt.Errorf("failed to extract binary from archive: %w", err)
	}

	err = c.Updater.Apply(cliReader)
	if err != nil {
		return fmt.Errorf("failed to apply update: %w", err)
	}

	_, _ = io.Copy(io.Discard, reader)

	// Wait for download to finish and handle any error from the go routine
	err = eg.Wait()
	if err != nil {
		return err
	}

	fmt.Println("Update finished successfully.")
	return nil
}

type Updater interface {
	Apply(update io.Reader) error
}

type SelfUpdater struct{}

func (s *SelfUpdater) Apply(r io.Reader) error {
	return update.Apply(r, update.Options{})
}
