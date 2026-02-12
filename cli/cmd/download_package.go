// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/spf13/cobra"

	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/codesphere-cloud/oms/internal/util"
)

// DownloadPackageCmd represents the package command
type DownloadPackageCmd struct {
	cmd        *cobra.Command
	Opts       DownloadPackageOpts
	FileWriter util.FileIO
}

type DownloadPackageOpts struct {
	*GlobalOptions
	Version    string
	Hash       string
	Filename   string
	Quiet      bool
	MaxRetries int
}

func (c *DownloadPackageCmd) RunE(_ *cobra.Command, args []string) error {
	if c.Opts.Version == "" && len(args) == 1 {
		c.Opts.Version = args[0]
	}

	if c.Opts.Hash != "" {
		log.Printf("Downloading package '%s' with hash '%s'\n", c.Opts.Version, c.Opts.Hash)
	} else {
		log.Printf("Downloading package '%s'\n", c.Opts.Version)
	}

	p := portal.NewPortalClient()
	build, err := p.GetBuild(portal.CodesphereProduct, c.Opts.Version, c.Opts.Hash)
	if err != nil {
		return fmt.Errorf("failed to get codesphere package: %w", err)
	}

	err = c.DownloadBuild(p, build, c.Opts.Filename)
	if err != nil {
		return fmt.Errorf("failed to download codesphere package: %w", err)
	}

	return nil
}

func AddDownloadPackageCmd(download *cobra.Command, opts *GlobalOptions) {
	pkg := DownloadPackageCmd{
		cmd: &cobra.Command{
			Use:   "package [VERSION]",
			Short: "Download a codesphere package",
			Long: io.Long(`Download a specific version of a Codesphere package
				To list available packages, run oms list packages.`),
			Example: formatExamplesWithBinary("download package", []io.Example{
				{Cmd: "codesphere-v1.55.0", Desc: "Download Codesphere version 1.55.0"},
				{Cmd: "--version codesphere-v1.55.0", Desc: "Download Codesphere version 1.55.0"},
				{Cmd: "--version codesphere-v1.55.0 --file installer-lite.tar.gz", Desc: "Download lite package of Codesphere version 1.55.0"},
			}, "oms-cli"),
			PreRunE: func(cmd *cobra.Command, args []string) error {
				// if version flag is not set, expect version as argument
				cmd.Args = cobra.NoArgs
				if !cmd.Flags().Changed("version") { // also covers the shorthand "-V"
					cmd.Args = cobra.ExactArgs(1)
				}

				err := cmd.Args(cmd, args)
				if err != nil {
					return err
				}

				return nil
			},
		},
		FileWriter: util.NewFilesystemWriter(),
	}

	pkg.cmd.Flags().StringVarP(&pkg.Opts.Version, "version", "V", "", "Codesphere version to download")
	pkg.cmd.Flags().StringVarP(&pkg.Opts.Hash, "hash", "H", "", "Hash of the version to download if multiple builds exist for the same version")
	pkg.cmd.Flags().StringVarP(&pkg.Opts.Filename, "file", "f", "installer.tar.gz", "Specify artifact to download")
	pkg.cmd.Flags().BoolVarP(&pkg.Opts.Quiet, "quiet", "q", false, "Suppress progress output during download")
	pkg.cmd.Flags().IntVarP(&pkg.Opts.MaxRetries, "max-retries", "r", 5, "Maximum number of download retry attempts")
	download.AddCommand(pkg.cmd)

	pkg.cmd.RunE = pkg.RunE
}

func (c *DownloadPackageCmd) DownloadBuild(p portal.Portal, build portal.Build, filename string) error {
	download, err := build.GetBuildForDownload(filename)
	if err != nil {
		return fmt.Errorf("failed to find artifact in package: %w", err)
	}

	fullFilename := strings.ReplaceAll(build.Version, "/", "-") + "-" + filename

	maxRetries := max(c.Opts.MaxRetries, 1)
	retryDelay := 5 * time.Second

	var downloadErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		out, err := c.FileWriter.OpenAppend(fullFilename)
		if err != nil {
			out, err = c.FileWriter.Create(fullFilename)
			if err != nil {
				return fmt.Errorf("failed to create file %s: %w", fullFilename, err)
			}
		}

		// Get current file size for resume functionality
		fileSize := 0
		fileInfo, err := out.Stat()
		if err == nil {
			fileSize = int(fileInfo.Size())
		}

		downloadErr = p.DownloadBuildArtifact("codesphere", download, out, fileSize, c.Opts.Quiet)
		util.CloseFileIgnoreError(out)

		if downloadErr == nil {
			break
		}

		// Determine if error is retryable
		shouldRetry := strings.Contains(downloadErr.Error(), "timeout") ||
			strings.Contains(downloadErr.Error(), "connection") ||
			strings.Contains(downloadErr.Error(), "EOF") ||
			strings.Contains(downloadErr.Error(), "reset by peer") ||
			strings.Contains(downloadErr.Error(), "temporary failure")

		if !shouldRetry || attempt == maxRetries {
			return fmt.Errorf("failed to download build after %d attempts: %w", attempt, downloadErr)
		}

		log.Printf("Download interrupted (attempt %d/%d). Retrying in %v...", attempt, maxRetries, retryDelay)
		time.Sleep(retryDelay)
		retryDelay = time.Duration(float64(retryDelay) * 1.5) // Exponential backoff
	}

	if downloadErr != nil {
		return fmt.Errorf("failed to download build: %w", downloadErr)
	}

	verifyFile, err := c.FileWriter.Open(fullFilename)
	if err != nil {
		return err
	}
	defer util.CloseFileIgnoreError(verifyFile)

	err = p.VerifyBuildArtifactDownload(verifyFile, download)
	if err != nil {
		return fmt.Errorf("failed to verify artifact: %w", err)
	}

	return nil
}
