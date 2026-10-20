// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package k0s

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	packageio "github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/spf13/cobra"

	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/node"
	intutil "github.com/codesphere-cloud/oms/internal/util"
)

// CleanupK0sCmd represents the k0s cleanup command
type CleanupK0sCmd struct {
	cmd           *cobra.Command
	Opts          CleanupK0sOpts
	FileWriter    intutil.FileIO
	ConfirmReader io.Reader
}

// CleanupK0sOpts holds the flags of the k0s cleanup command
type CleanupK0sOpts struct {
	*util.GlobalOptions
	InstallConfig string
	SSHKeyPath    string
	SkipReboot    bool
	Force         bool
}

func (c *CleanupK0sCmd) RunE(_ *cobra.Command, args []string) error {
	config, err := loadManagedInstallConfig(c.Opts.InstallConfig)
	if err != nil {
		return err
	}

	nodeClient := node.NewSSHNodeClient(!c.Opts.Verbose)

	var nodes []*node.Node
	for _, ip := range installer.K0sCleanupNodeIPs(config) {
		nodes = append(nodes, &node.Node{
			FileIO:     c.FileWriter,
			KeyPath:    c.Opts.SSHKeyPath,
			Name:       ip,
			ExternalIP: ip,
			NodeClient: nodeClient,
		})
	}

	return c.CleanupK0s(nodes)
}

// AddCleanupCmd adds the k0s cleanup command to the given cleanup command
func AddCleanupCmd(cleanup *cobra.Command, opts *util.GlobalOptions) {
	k0s := CleanupK0sCmd{
		cmd: &cobra.Command{
			Use:   "k0s",
			Short: "Remove k0s from all nodes of a Codesphere install-config",
			Long: packageio.Long(`Remove k0s completely from all control plane and worker nodes of a Codesphere install-config.
			Use this to recover from a failed or corrupted k0s installation without recreating the machines.

			On every node this will:
			- Stop and disable the k0sworker and k0scontroller services
			- Run k0s reset
			- Delete the k0s service units, /var/lib/k0s and /etc/k0s
			- Reboot the node (skip with --skip-reboot)

			The nodes are accessed as root via SSH.`),
			Example: util.FormatExamples("cleanup k0s", []packageio.Example{
				{Cmd: "--install-config <path>", Desc: "Remove k0s from all nodes of the install-config"},
				{Cmd: "--install-config <path> --ssh-key-path <path>", Desc: "Use the given SSH private key to access the nodes"},
				{Cmd: "--install-config <path> --skip-reboot", Desc: "Remove k0s without rebooting the nodes"},
				{Cmd: "--install-config <path> --force", Desc: "Skip the confirmation prompt"},
			}),
		},
		Opts:          CleanupK0sOpts{GlobalOptions: opts},
		FileWriter:    intutil.NewFilesystemWriter(),
		ConfirmReader: os.Stdin,
	}
	k0s.cmd.Flags().StringVar(&k0s.Opts.InstallConfig, "install-config", "", "Path to Codesphere install-config file (required)")
	k0s.cmd.Flags().StringVar(&k0s.Opts.SSHKeyPath, "ssh-key-path", "", "SSH private key path to access the nodes")
	k0s.cmd.Flags().BoolVar(&k0s.Opts.SkipReboot, "skip-reboot", false, "Do not reboot the nodes after the cleanup")
	k0s.cmd.Flags().BoolVarP(&k0s.Opts.Force, "force", "f", false, "Skip the confirmation prompt")

	_ = k0s.cmd.MarkFlagRequired("install-config")

	util.AddCmd(cleanup, k0s.cmd)

	k0s.cmd.RunE = k0s.RunE
}

// CleanupK0s removes k0s from all given nodes and reboots them afterwards. A failing node does
// not stop the cleanup of the others; all failures are returned together and failed nodes are
// not rebooted.
func (c *CleanupK0sCmd) CleanupK0s(nodes []*node.Node) error {
	if len(nodes) == 0 {
		return fmt.Errorf("install-config does not contain any Kubernetes nodes")
	}

	if err := c.confirmCleanup(nodes); err != nil {
		return err
	}

	var (
		errs    []error
		cleaned []*node.Node
	)

	for _, n := range nodes {
		log.Printf("Cleaning up k0s on %s...", n.GetName())

		if err := n.RunSSHCommand("root", installer.K0sCleanupScript()); err != nil {
			errs = append(errs, fmt.Errorf("failed to clean up k0s on %s: %w", n.GetName(), err))
			continue
		}

		cleaned = append(cleaned, n)
	}

	if !c.Opts.SkipReboot {
		for _, n := range cleaned {
			log.Printf("Rebooting %s...", n.GetName())

			if err := n.RunSSHCommand("root", installer.K0sRebootCommand); err != nil {
				errs = append(errs, fmt.Errorf("failed to reboot %s: %w", n.GetName(), err))
			}
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	log.Printf("k0s removed from %d node(s)", len(nodes))

	if c.Opts.SkipReboot {
		log.Println("Reboot the nodes before installing k0s again")
	}

	return nil
}

func (c *CleanupK0sCmd) confirmCleanup(nodes []*node.Node) error {
	if c.Opts.Force {
		return nil
	}

	log.Println("WARNING: This will completely remove k0s and all cluster data from the following nodes:")

	for _, n := range nodes {
		log.Printf("  - %s", n.GetName())
	}

	if !c.Opts.SkipReboot {
		log.Println("The nodes will be rebooted afterwards.")
	}

	log.Println("Type 'yes' to continue: ")

	confirmation, err := bufio.NewReader(c.ConfirmReader).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("failed to read confirmation: %w", err)
	}

	if strings.TrimSpace(confirmation) != "yes" {
		return fmt.Errorf("cleanup not confirmed, aborting")
	}

	return nil
}
