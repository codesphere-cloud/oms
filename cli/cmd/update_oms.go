// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/blang/semver"
	"github.com/inconshreveable/go-update"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/codesphere-cloud/oms/internal/util"
	"github.com/codesphere-cloud/oms/internal/version"
)

type OMSUpdater interface {
	Apply(update io.Reader) error
}

type OMSSelfUpdater struct{}

func (s *OMSSelfUpdater) Apply(r io.Reader) error {
	return update.Apply(r, update.Options{})
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
		Long:  `Updates the OMS CLI to the latest release from OMS Portal.`,
		RunE: func(_ *cobra.Command, args []string) error {
			p := portal.NewPortalClient()
			return cmdState.SelfUpdate(p)
		},
	}
	parentCmd.AddCommand(omsCmd)
}

func (c *UpdateOmsCmd) SelfUpdate(p portal.Portal) error {
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
		err = p.DownloadBuildArtifact(portal.OmsProduct, download, writer, false)
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
