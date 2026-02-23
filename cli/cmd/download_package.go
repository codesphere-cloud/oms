// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"log"
	"strings"

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
	Version  string
	Hash     string
	Filename string
	Quiet    bool
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
			Example: formatExamples("download package", []io.Example{
				{Cmd: "codesphere-v1.55.0", Desc: "Download Codesphere version 1.55.0"},
				{Cmd: "--version codesphere-v1.55.0", Desc: "Download Codesphere version 1.55.0"},
				{Cmd: "--version codesphere-v1.55.0 --file installer-lite.tar.gz", Desc: "Download lite package of Codesphere version 1.55.0"},
			}),
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
	download.AddCommand(pkg.cmd)

	pkg.cmd.RunE = pkg.RunE
}

func (c *DownloadPackageCmd) DownloadBuild(p portal.Portal, build portal.Build, filename string) error {
	download, err := build.GetBuildForDownload(filename)
	if err != nil {
		return fmt.Errorf("failed to find artifact in package: %w", err)
	}

	fullFilename := strings.ReplaceAll(build.Version, "/", "-") + "-" + filename
	out, err := c.FileWriter.OpenAppend(fullFilename)
	if err != nil {
		out, err = c.FileWriter.Create(fullFilename)
		if err != nil {
			return fmt.Errorf("failed to create file %s: %w", fullFilename, err)
		}
	}
	defer util.CloseFileIgnoreError(out)

	// get already downloaded file size of fullFilename
	fileSize := 0
	fileInfo, err := out.Stat()
	if err == nil {
		fileSize = int(fileInfo.Size())
	}

	err = p.DownloadBuildArtifact("codesphere", download, out, fileSize, c.Opts.Quiet)
	if err != nil {
		return fmt.Errorf("failed to download build: %w", err)
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
