// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/util"
	"github.com/spf13/cobra"
)

// InstallCodesphereCmd represents the codesphere command
type InstallCodesphereCmd struct {
	cmd  *cobra.Command
	Opts *InstallCodesphereOpts
	Env  env.Env
}

type InstallCodesphereOpts struct {
	*GlobalOptions
	Package  string
	Force    bool
	Config   string
	PrivKey  string
	SkipStep string
}

func (c *InstallCodesphereCmd) RunE(_ *cobra.Command, args []string) error {
	workdir := c.Env.GetOmsWorkdir()
	p := installer.NewPackage(workdir, c.Opts.Package)

	err := c.ExtractAndInstall(p, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return fmt.Errorf("failed to extract and install package: %w", err)
	}

	return nil
}

func AddInstallCodesphereCmd(install *cobra.Command, opts *GlobalOptions) {
	codesphere := InstallCodesphereCmd{
		cmd: &cobra.Command{
			Use:   "codesphere",
			Short: "Coming soon: Install a Codesphere instance",
			Long:  io.Long(`Coming soon: Install a Codesphere instance`),
		},
		Opts: &InstallCodesphereOpts{GlobalOptions: opts},
		Env:  env.NewEnv(),
	}
	codesphere.cmd.Flags().StringVarP(&codesphere.Opts.Package, "package", "p", "", "Package file (e.g. codesphere-v1.2.3-installer.tar.gz) to load binaries, installer etc. from")
	codesphere.cmd.Flags().BoolVarP(&codesphere.Opts.Force, "force", "f", false, "Enforce package extraction")
	codesphere.cmd.Flags().StringVarP(&codesphere.Opts.Config, "config", "c", "", "Configuration file for the private cloud installer")
	codesphere.cmd.Flags().StringVarP(&codesphere.Opts.PrivKey, "priv-key", "k", "", "Private key file for the installation")
	codesphere.cmd.Flags().StringVarP(&codesphere.Opts.SkipStep, "skip-step", "s", "", "Skip specific installation steps")

	util.MarkFlagRequired(codesphere.cmd, "package")
	util.MarkFlagRequired(codesphere.cmd, "config")
	util.MarkFlagRequired(codesphere.cmd, "priv-key")

	install.AddCommand(codesphere.cmd)
	codesphere.cmd.RunE = codesphere.RunE
}

func (c *InstallCodesphereCmd) ExtractAndInstall(p *installer.Package, goos string, goarch string) error {
	if goos != "linux" || goarch != "amd64" {
		return fmt.Errorf("codesphere installation is only supported on Linux amd64. Current platform: %s/%s", goos, goarch)
	}

	err := p.Extract(c.Opts.Force)
	if err != nil {
		return fmt.Errorf("failed to extract package to workdir: %w", err)
	}

	foundFiles, err := c.ListPackageContents(p)
	if err != nil {
		return fmt.Errorf("failed to list available files: %w", err)
	}

	if !slices.Contains(foundFiles, "deps.tar.gz") {
		return fmt.Errorf("deps.tar.gz not found in package")
	}
	if !slices.Contains(foundFiles, "private-cloud-installer.js") {
		return fmt.Errorf("private-cloud-installer.js not found in package")
	}
	if !slices.Contains(foundFiles, "node") {
		return fmt.Errorf("node executable not found in package")
	}

	nodePath := filepath.Join(".", p.GetWorkDir(), "node")
	err = os.Chmod(nodePath, 0755)
	if err != nil {
		return fmt.Errorf("failed to make node executable: %w", err)
	}

	log.Printf("Using Node.js executable: %s", nodePath)
	log.Println("Starting private cloud installer script...")
	installerPath := filepath.Join(".", p.GetWorkDir(), "private-cloud-installer.js")
	archivePath := filepath.Join(".", p.GetWorkDir(), "deps.tar.gz")

	// Build command
	cmdArgs := []string{installerPath, "--archive", archivePath, "--config", c.Opts.Config, "--privKey", c.Opts.PrivKey}
	if c.Opts.SkipStep != "" {
		cmdArgs = append(cmdArgs, "--skipStep", c.Opts.SkipStep)
	}

	cmd := exec.Command(nodePath, cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to run installer script: %w", err)
	}
	log.Println("Private cloud installer script finished.")

	return nil
}

func (c *InstallCodesphereCmd) ListPackageContents(p *installer.Package) ([]string, error) {
	packageDir := p.GetWorkDir()
	if !p.FileIO.Exists(packageDir) {
		return nil, fmt.Errorf("work dir not found: %s", packageDir)
	}

	entries, err := p.FileIO.ReadDir(packageDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory contents: %w", err)
	}

	log.Printf("Listing contents of %s", packageDir)
	var foundFiles []string
	for _, entry := range entries {
		filename := entry.Name()
		log.Printf("- %s", filename)
		foundFiles = append(foundFiles, filename)
	}

	return foundFiles, nil
}
