// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package k0s

import (
	"fmt"

	packageio "github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/spf13/cobra"

	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/portal"
	intutil "github.com/codesphere-cloud/oms/internal/util"
)

// InitK0sConfigCmd represents the command that generates a k0sctl config without installing k0s.
type InitK0sConfigCmd struct {
	cmd        *cobra.Command
	Opts       InitK0sConfigOpts
	Env        env.Env
	FileWriter intutil.FileIO
}

// InitK0sConfigOpts are the options of the init k0s-config command.
type InitK0sConfigOpts struct {
	*util.GlobalOptions
	installer.K0sctlConfigOpts
	// Output is the path the generated k0sctl config is written to.
	Output string
}

// RunE generates the k0sctl config with the real package, k0s and filesystem implementations.
func (c *InitK0sConfigCmd) RunE(_ *cobra.Command, _ []string) error {
	hw := portal.NewHttpWrapper()
	pm := installer.NewPackage(c.Env.GetOmsWorkdir(), c.Opts.Package)
	k0s := installer.NewK0s(hw, c.Env, c.FileWriter)

	return c.InitK0sConfig(pm, k0s)
}

// AddInitK0sConfigCmd registers the k0s-config command under init.
func AddInitK0sConfigCmd(init *cobra.Command, opts *util.GlobalOptions) {
	k0sConfig := InitK0sConfigCmd{
		cmd: &cobra.Command{
			Use:   "k0s-config",
			Short: "Generate a k0sctl config for installing k0s",
			Long: packageio.Long(`Generate a k0sctl configuration without installing k0s. Nothing is
			downloaded: the k0s binary is taken from the package or the OMS cache, see
			'oms download k0s'.

			The cluster is defined by a Codesphere install-config, an Ansible inventory and flags,
			with increasing precedence: install-config < Ansible inventory < flags. Each source
			only replaces the control planes or workers it defines, so --worker alone keeps the
			control planes of the install-config or the Ansible inventory. Without an
			install-config, --datacenter-name is required. A node given as both control plane and
			worker runs both roles.

			With --airgapped, the generated config makes k0sctl upload the k0s airgap image
			bundle to every node that runs a worker and stops k0s from pulling images.`),
			Example: util.FormatExamples("init k0s-config", []packageio.Example{
				{Cmd: "--install-config <path>", Desc: "Generate a k0sctl config into the OMS workdir"},
				{Cmd: "--install-config <path> --output <path>", Desc: "Generate a k0sctl config at the given path"},
				{Cmd: "--ansible-inventory <path> --datacenter-name <name>", Desc: "Generate a k0sctl config for the Kubernetes hosts of an Ansible inventory"},
				{Cmd: "--control-plane 10.0.0.1 --worker 10.0.0.2,10.0.0.3 --datacenter-name <name>", Desc: "Generate a k0sctl config for the given node IPs"},
				{Cmd: "--install-config <path> --ansible-inventory <path>", Desc: "Generate a k0sctl config from an install-config with the Kubernetes hosts of an Ansible inventory"},
				{Cmd: "--install-config <path> --airgapped", Desc: "Generate an airgap k0sctl config using the airgap bundle from the OMS cache"},
				{Cmd: "--install-config <path> --airgapped --path <bundle>", Desc: "Generate an airgap k0sctl config using the given airgap bundle"},
			}),
		},
		Opts:       InitK0sConfigOpts{GlobalOptions: opts},
		Env:        env.NewEnv(),
		FileWriter: intutil.NewFilesystemWriter(),
	}
	flags := k0sConfig.cmd.Flags()
	flags.StringVar(&k0sConfig.Opts.InstallConfig, "install-config", "", "Path to Codesphere install-config file")
	flags.StringVar(&k0sConfig.Opts.AnsibleInventory, "ansible-inventory", "", "Path to Ansible inventory file whose k8s-cp and k8s-workers hosts replace the ones of the install-config")
	flags.StringSliceVar(&k0sConfig.Opts.ControlPlanes, "control-plane", nil, "IP addresses of the Kubernetes control plane nodes, replacing the ones of the install-config and the Ansible inventory")
	flags.StringSliceVar(&k0sConfig.Opts.Workers, "worker", nil, "IP addresses of the Kubernetes worker nodes, replacing the ones of the install-config and the Ansible inventory")
	flags.StringVar(&k0sConfig.Opts.DatacenterName, "datacenter-name", "", "Name of the data center, overriding the one of the install-config (required without --install-config)")
	flags.StringVar(&k0sConfig.Opts.APIServerHost, "api-server-host", "", "External host name or address of the Kubernetes API server, overriding the one of the install-config")
	flags.StringVarP(&k0sConfig.Opts.Output, "output", "o", "", "Output file path for the k0sctl config (default: k0sctl-config-<datacenter>.yaml in the OMS workdir)")
	flags.StringVarP(&k0sConfig.Opts.Version, "version", "v", installer.DefaultK0sVersion, "Version of k0s to install")
	flags.StringVarP(&k0sConfig.Opts.Package, "package", "p", "", "Package file (e.g. codesphere-v1.2.3-installer-lite.tar.gz) to load k0s from")
	flags.StringVar(&k0sConfig.Opts.SSHKeyPath, "ssh-key-path", "", "SSH private key path for remote installation")
	flags.BoolVarP(&k0sConfig.Opts.Force, "force", "f", false, "Force extracting k0s from the package even if it was extracted before")
	flags.BoolVar(&k0sConfig.Opts.NoDownload, "no-download", false, "Do not upload a k0s binary, the nodes download k0s themselves")
	flags.BoolVar(&k0sConfig.Opts.Airgap, "airgapped", false, "Generate a config for installing k0s without internet access from the airgap image bundle")
	flags.StringVar(&k0sConfig.Opts.AirgapBundlePath, "path", "", "Path to the k0s airgap image bundle (requires --airgapped; default: the bundle in the OMS cache)")

	k0sConfig.cmd.MarkFlagsOneRequired("install-config", "ansible-inventory", "control-plane")

	util.AddCmd(init, k0sConfig.cmd)

	k0sConfig.cmd.RunE = k0sConfig.RunE
}

// InitK0sConfig generates the k0sctl config from the command options. It only uses local
// files and fails if the k0s binary or the airgap bundle have not been downloaded yet.
func (c *InitK0sConfigCmd) InitK0sConfig(pm installer.PackageManager, k0s installer.K0sManager) error {
	if c.Opts.AirgapBundlePath != "" && !c.Opts.Airgap {
		return fmt.Errorf("--path requires --airgapped")
	}

	if c.Opts.InstallConfig == "" && c.Opts.DatacenterName == "" {
		return fmt.Errorf("--datacenter-name is required without --install-config")
	}

	if err := c.FileWriter.MkdirAll(c.Env.GetOmsWorkdir(), 0755); err != nil {
		return fmt.Errorf("failed to create oms workdir: %w", err)
	}

	generator := installer.K0sctlConfigGenerator{
		Opts:       c.Opts.K0sctlConfigOpts,
		Verbose:    c.Opts.Verbose,
		Env:        c.Env,
		FileWriter: c.FileWriter,
		CacheOnly:  true,
	}

	if _, err := generator.Generate(pm, k0s, c.Opts.Output); err != nil {
		return fmt.Errorf("failed to generate k0sctl config: %w", err)
	}

	return nil
}
