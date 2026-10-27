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
	"github.com/codesphere-cloud/oms/internal/prompt"
	intutil "github.com/codesphere-cloud/oms/internal/util"
)

// InitK0sConfigCmd represents the command that generates a k0sctl config without installing k0s.
type InitK0sConfigCmd struct {
	cmd        *cobra.Command
	Opts       InitK0sConfigOpts
	Env        env.Env
	FileWriter intutil.FileIO
	Prompter   prompt.Prompter
}

// InitK0sConfigOpts are the options of the init k0s-config command.
type InitK0sConfigOpts struct {
	*util.GlobalOptions
	installer.K0sctlConfigOpts
	// Output is the path the generated k0sctl config is written to.
	Output string
	// Yes overwrites an existing k0sctl config without asking.
	Yes bool
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
			Long: packageio.Long(`Generate a k0sctl configuration from a Codesphere install-config without
			installing k0s. Nothing is downloaded: the k0s binary is taken from the package or
			the OMS cache, see 'oms download k0s'.

			An existing k0sctl config is only overwritten after confirmation or with --yes.

			With --airgapped, the generated config makes k0sctl upload the k0s airgap image
			bundle to every node that runs a worker and stops k0s from pulling images.`),
			Example: util.FormatExamples("init k0s-config", []packageio.Example{
				{Cmd: "--install-config <path>", Desc: "Generate a k0sctl config into the OMS workdir"},
				{Cmd: "--install-config <path> --output <path>", Desc: "Generate a k0sctl config at the given path"},
				{Cmd: "--install-config <path> --yes", Desc: "Regenerate the k0sctl config, overwriting an existing one without asking"},
				{Cmd: "--install-config <path> --airgapped", Desc: "Generate an airgap k0sctl config using the airgap bundle from the OMS cache"},
				{Cmd: "--install-config <path> --airgapped --path <bundle>", Desc: "Generate an airgap k0sctl config using the given airgap bundle"},
			}),
		},
		Opts:       InitK0sConfigOpts{GlobalOptions: opts},
		Env:        env.NewEnv(),
		FileWriter: intutil.NewFilesystemWriter(),
		Prompter:   prompt.NewPrompter(true),
	}
	flags := k0sConfig.cmd.Flags()
	flags.StringVar(&k0sConfig.Opts.InstallConfig, "install-config", "", "Path to Codesphere install-config file (required)")
	flags.StringVarP(&k0sConfig.Opts.Output, "output", "o", "", "Output file path for the k0sctl config (default: k0sctl-config-<datacenter>.yaml in the OMS workdir)")
	flags.StringVarP(&k0sConfig.Opts.Version, "version", "v", installer.DefaultK0sVersion, "Version of k0s to install")
	flags.StringVarP(&k0sConfig.Opts.Package, "package", "p", "", "Package file (e.g. codesphere-v1.2.3-installer-lite.tar.gz) to load k0s from")
	flags.StringVar(&k0sConfig.Opts.SSHKeyPath, "ssh-key-path", "", "SSH private key path for remote installation")
	flags.BoolVarP(&k0sConfig.Opts.Force, "force", "f", false, "Force extracting k0s from the package even if it was extracted before")
	flags.BoolVar(&k0sConfig.Opts.NoDownload, "no-download", false, "Do not upload a k0s binary, the nodes download k0s themselves")
	flags.BoolVar(&k0sConfig.Opts.Airgap, "airgapped", false, "Generate a config for installing k0s without internet access from the airgap image bundle")
	flags.StringVar(&k0sConfig.Opts.AirgapBundlePath, "path", "", "Path to the k0s airgap image bundle (requires --airgapped; default: the bundle in the OMS cache)")
	flags.BoolVarP(&k0sConfig.Opts.Yes, "yes", "y", false, "Overwrite an existing k0sctl config without asking")

	_ = k0sConfig.cmd.MarkFlagRequired("install-config")

	util.AddCmd(init, k0sConfig.cmd)

	k0sConfig.cmd.RunE = k0sConfig.RunE
}

// InitK0sConfig generates the k0sctl config from the command options. It only uses local
// files and fails if the k0s binary or the airgap bundle have not been downloaded yet.
func (c *InitK0sConfigCmd) InitK0sConfig(pm installer.PackageManager, k0s installer.K0sManager) error {
	if c.Opts.AirgapBundlePath != "" && !c.Opts.Airgap {
		return fmt.Errorf("--path requires --airgapped")
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
		Overwrite:  c.Opts.Yes,
		Prompter:   c.Prompter,
	}

	if _, err := generator.Generate(pm, k0s, c.Opts.Output); err != nil {
		return fmt.Errorf("failed to generate k0sctl config: %w", err)
	}

	return nil
}
