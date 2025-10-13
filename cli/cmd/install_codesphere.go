// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"slices"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
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
	Package string
	Force   bool
}

func (c *InstallCodesphereCmd) RunE(_ *cobra.Command, args []string) error {
	if c.Opts.Package == "" {
		return errors.New("required option package not set")
	}

	workdir := c.Env.GetOmsWorkdir()
	p := installer.NewPackage(workdir, c.Opts.Package)

	err := c.ExtractAndInstall(p, args)
	if err != nil {
		return fmt.Errorf("failed to extend baseimage: %w", err)
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
	codesphere.cmd.Flags().StringVarP(&codesphere.Opts.Package, "package", "p", "", "Package file (e.g. codesphere-v1.2.3-installer.tar.gz) to load base image from")
	codesphere.cmd.Flags().BoolVarP(&codesphere.Opts.Force, "force", "f", false, "Enforce package extraction")
	install.AddCommand(codesphere.cmd)
	codesphere.cmd.RunE = codesphere.RunE
}

func (c *InstallCodesphereCmd) ExtractAndInstall(p *installer.Package, args []string) error {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		return fmt.Errorf("codesphere installation is only supported on Linux amd64. Current platform: %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	err := p.Extract(c.Opts.Force)
	if err != nil {
		return fmt.Errorf("failed to extract package to workdir: %w", err)
	}

	foundFiles, err := c.ListPackageContents(p)
	if err != nil {
		return fmt.Errorf("failed to list available files: %w", err)
	}

	if !slices.Contains(foundFiles, "private-cloud-installer.js") {
		return fmt.Errorf("private-cloud-installer.js not found in package")
	}
	if !slices.Contains(foundFiles, "node") {
		return fmt.Errorf("node executable not found in package")
	}

	nodeDir := "./" + p.GetWorkDir() + "/node"
	err = os.Chmod(nodeDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to make node executable: %w", err)
	}

	log.Printf("Using Node.js executable: %s", nodeDir)
	log.Println("Starting private cloud installer script...")
	out, err := exec.Command(nodeDir, args...).Output()
	if err != nil {
		return fmt.Errorf("failed to run installer script: %w", err)
	}
	fmt.Println(string(out))
	fmt.Println("Private cloud installer script finished.")

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
