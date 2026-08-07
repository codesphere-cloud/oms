// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"

	packageio "github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer/docker"
	"github.com/codesphere-cloud/oms/internal/installer/node"
	"github.com/codesphere-cloud/oms/internal/util"
	"github.com/spf13/cobra"
)

// InstallDockerCmd represents the docker install command.
type InstallDockerCmd struct {
	cmd        *cobra.Command
	Opts       InstallDockerOpts
	Env        env.Env
	FileWriter util.FileIO
}

// InstallDockerOpts holds all CLI flags for the docker sub-command.
type InstallDockerOpts struct {
	*GlobalOptions

	// SSH / remote host
	// SSHKeyPath string
	SSHUser string
	SSHHost string
	SSHPort int
}

func (c *InstallDockerCmd) RunE(_ *cobra.Command, args []string) error {
	return c.InstallDocker()
}

func AddInstallDockerCmd(install *cobra.Command, opts *GlobalOptions) {
	pg := InstallDockerCmd{
		cmd: &cobra.Command{
			Use:     "docker",
			Short:   "Install Docker on a remote host",
			Long:    packageio.Long(`Install Docker (if not already present) on a remote host accessed via SSH.`),
			Example: formatExamples("install docker", []packageio.Example{}),
		},
		Opts:       InstallDockerOpts{GlobalOptions: opts},
		Env:        env.NewEnv(),
		FileWriter: util.NewFilesystemWriter(),
	}

	f := pg.cmd.Flags()

	// SSH flags
	f.StringVar(&pg.Opts.SSHHost, "ssh-host", "", "Remote host IP or hostname (required)")
	f.IntVar(&pg.Opts.SSHPort, "ssh-port", 22, "SSH port on the remote host")
	f.StringVar(&pg.Opts.SSHUser, "ssh-user", "root", "SSH username")
	// f.StringVar(&pg.Opts.SSHKeyPath, "ssh-key-path", "", "Path to SSH private key")

	_ = pg.cmd.MarkFlagRequired("ssh-host")

	AddCmd(install, pg.cmd)
	pg.cmd.RunE = pg.RunE
}

func (c *InstallDockerCmd) InstallDocker() error {
	node := &node.Node{
		Name:       "node",
		ExternalIP: c.Opts.SSHHost,

		NodeClient: node.NewSSHNodeClient(true),
	}

	dockerClient := docker.New("root", node)
	if !dockerClient.IsInstalled() {
		err := dockerClient.InstallWithApt()
		if err != nil {
			return fmt.Errorf("failed to install Docker: %w", err)
		}
	}

	return nil
}
