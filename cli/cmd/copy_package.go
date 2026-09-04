// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	csio "github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/codesphere-cloud/oms/internal/prompt"
	intutil "github.com/codesphere-cloud/oms/internal/util"
	"github.com/google/go-containerregistry/pkg/logs"
	"github.com/spf13/cobra"
)

const defaultInstallerPackageArtifact = "installer-lite.tar.gz"

// CopyPackageCmd represents the copy package command.
type CopyPackageCmd struct {
	cmd        *cobra.Command
	Opts       CopyPackageOpts
	Env        env.Env
	FileWriter intutil.FileIO
	Prompter   prompt.Prompter
}

// CopyPackageOpts holds the flags accepted by the copy package command.
type CopyPackageOpts struct {
	*util.GlobalOptions
	Package       string
	Version       string
	Hash          string
	Filename      string
	Dest          string
	Yes           bool
	Force         bool
	Insecure      bool
	Verbose       bool
	ShowArtifacts bool
}

// RunE resolves the installer package and executes its artifact transfers.
func (c *CopyPackageCmd) RunE(cmd *cobra.Command, _ []string) error {
	packagePath, err := c.resolvePackage(portal.NewPortalClient())
	if err != nil {
		return err
	}

	packageManager := installer.NewPackage(c.Env.GetOmsWorkdir(), packagePath)

	if c.Opts.Verbose {
		logs.Debug.SetOutput(os.Stderr)
	}

	return c.CopyPackage(cmd.Context(), packageManager, &installer.CraneArtifactCopier{
		Insecure: c.Opts.Insecure,
	})
}

// AddCopyPackageCmd adds the package subcommand to the copy command.
func AddCopyPackageCmd(parent *cobra.Command, opts *util.GlobalOptions) {
	c := &CopyPackageCmd{
		cmd: &cobra.Command{
			Use:   "package",
			Short: "Copy all images and Helm charts from an installer package",
			Long: csio.Long(`Read all container images and OCI Helm charts from an installer package BOM
				and copy them to another registry.

				Use --package for a local installer package or --version to download one
				from the OMS portal. The source repository paths are preserved below --dest.`),
			Args: cobra.NoArgs,
			Example: util.FormatExamples("copy package", []csio.Example{
				{Cmd: "--package codesphere-v1.70.0-installer-lite.tar.gz --dest registry.example.com/mirror", Desc: "Copy artifacts from a local package"},
				{Cmd: "--version codesphere-v1.70.0 --dest registry.example.com/mirror --yes", Desc: "Download an upstream package and copy without prompting"},
			}),
		},
		Opts:       CopyPackageOpts{GlobalOptions: opts, Filename: defaultInstallerPackageArtifact},
		Env:        env.NewEnv(),
		FileWriter: intutil.NewFilesystemWriter(),
		Prompter:   prompt.NewPrompter(true),
	}
	c.cmd.PreRunE = func(_ *cobra.Command, _ []string) error {
		if (c.Opts.Package == "") == (c.Opts.Version == "") {
			return fmt.Errorf("exactly one of --package or --version must be specified")
		}

		return nil
	}

	flags := c.cmd.Flags()
	flags.StringVarP(&c.Opts.Package, "package", "p", "", "Path to a local installer package")
	flags.StringVarP(&c.Opts.Version, "version", "V", "", "Codesphere package version to download from the OMS portal")
	flags.StringVarP(&c.Opts.Hash, "hash", "H", "", "Build hash used to disambiguate an upstream package version")
	flags.StringVarP(&c.Opts.Filename, "file", "f", defaultInstallerPackageArtifact, "Installer artifact to download for an upstream package")
	flags.StringVar(&c.Opts.Dest, "dest", "", "Destination registry or repository prefix")
	flags.BoolVar(&c.Opts.Insecure, "insecure", false, "Allow image references to be fetched without TLS")
	flags.BoolVarP(&c.Opts.Yes, "yes", "y", false, "Copy without prompting for confirmation")
	flags.BoolVarP(&c.Opts.Verbose, "verbose", "v", false, "Enable debug logs")
	flags.BoolVar(&c.Opts.Force, "force", false, "Re-extract the installer package")
	flags.BoolVar(&c.Opts.ShowArtifacts, "show-artifacts", false, "Print the source and destination of every artifact to copy")
	util.MarkFlagRequired(c.cmd, "dest")

	util.AddCmd(parent, c.cmd)
	c.cmd.RunE = c.RunE
}

// CopyPackage extracts the package, prints the complete transfer plan, asks
// for confirmation, and then copies each artifact.
func (c *CopyPackageCmd) CopyPackage(ctx context.Context, packageManager installer.PackageManager, copier installer.ArtifactCopier) error {
	if err := packageManager.Extract(c.Opts.Force); err != nil {
		return fmt.Errorf("failed to extract package: %w", err)
	}

	artifacts, err := installer.ReadPackageArtifacts(packageManager.GetDependencyPath("bom.json"), c.Opts.Dest)
	if err != nil {
		return fmt.Errorf("failed to read package BOM: %w", err)
	}

	if len(artifacts) == 0 {
		return fmt.Errorf("package BOM contains no container images or OCI Helm charts")
	}

	if c.Opts.ShowArtifacts {
		log.Printf("Artifacts to copy (%d):", len(artifacts))

		for _, artifact := range artifacts {
			log.Printf("  %s -> %s", artifact.Source, artifact.Destination)
		}
	} else {
		log.Printf("Artifacts to copy: %d", len(artifacts))
	}

	if !c.Opts.Yes && !c.Prompter.Bool("Copy these artifacts?", false) {
		return fmt.Errorf("transfer cancelled")
	}

	log.Printf("Copying %d package artifacts...", len(artifacts))

	if err := installer.CopyPackageArtifacts(ctx, copier, artifacts); err != nil {
		return fmt.Errorf("failed to copy package artifacts: %w", err)
	}

	log.Printf("Successfully copied %d package artifacts to %s", len(artifacts), c.Opts.Dest)

	return nil
}

func (c *CopyPackageCmd) resolvePackage(portalClient portal.Portal) (string, error) {
	if c.Opts.Package != "" {
		return c.Opts.Package, nil
	}

	workdir := c.Env.GetOmsWorkdir()
	if err := os.MkdirAll(workdir, 0755); err != nil {
		return "", fmt.Errorf("failed to create OMS workdir: %w", err)
	}

	build, err := portalClient.GetBuild(portal.CodesphereProduct, c.Opts.Version, c.Opts.Hash)
	if err != nil {
		return "", fmt.Errorf("failed to get upstream package: %w", err)
	}

	destination := filepath.Join(workdir, build.BuildPackageFilename(c.Opts.Filename))

	if err := portal.DownloadAndVerifyBuild(portalClient, c.FileWriter, portal.CodesphereProduct, build, c.Opts.Filename, destination, portal.DownloadOptions{}); err != nil {
		return "", fmt.Errorf("failed to download upstream package: %w", err)
	}

	return destination, nil
}
