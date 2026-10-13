// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package k0s

import (
	"fmt"
	"log"
	"strings"

	packageio "github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/spf13/cobra"

	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/vault"
	"github.com/codesphere-cloud/oms/internal/portal"
	intutil "github.com/codesphere-cloud/oms/internal/util"
)

// InstallK0sCmd represents the k0s download command
type InstallK0sCmd struct {
	cmd        *cobra.Command
	Opts       InstallK0sOpts
	Env        env.Env
	FileWriter intutil.FileIO
}

type InstallK0sOpts struct {
	*util.GlobalOptions
	K0sctlConfigOpts
	K0sctlVersion string
	Vault         string
	VaultPrivKey  string
	VaultType     string
}

func (c *InstallK0sCmd) RunE(_ *cobra.Command, args []string) error {
	hw := portal.NewHttpWrapper()
	env := c.Env
	pm := installer.NewPackage(env.GetOmsWorkdir(), c.Opts.Package)
	k0s := installer.NewK0s(hw, env, c.FileWriter)
	k0sctl := installer.NewK0sctl(hw, env, c.FileWriter)

	return c.InstallK0s(pm, k0s, k0sctl)
}

func AddInstallCmd(install *cobra.Command, opts *util.GlobalOptions) {
	k0s := InstallK0sCmd{
		cmd: &cobra.Command{
			Use:   "k0s",
			Short: "Install k0s Kubernetes distribution",
			Long: packageio.Long(`Install k0s either from the package or by downloading it.
			This command uses k0sctl to deploy k0s clusters from a Codesphere install-config.
			
			You must provide a Codesphere install-config file, which will:
			- Generate a k0s configuration from the install-config
			- Generate a k0sctl configuration for cluster deployment
			- Deploy k0s to all nodes defined in the install-config using k0sctl`),
			Example: util.FormatExamples("install k0s", []packageio.Example{
				{Cmd: "--install-config <path>", Desc: "Path to Codesphere install-config file to generate k0s config from"},
				{Cmd: "--version <version>", Desc: "Version of k0s to install (e.g., v1.31.14+k0s.0)"},
				{Cmd: "--k0sctl-version <version>", Desc: "Version of k0sctl to use (e.g., v0.17.4)"},
				{Cmd: "--package <file>", Desc: "Package file (e.g. codesphere-v1.2.3-installer-lite.tar.gz) to load k0s from"},
				{Cmd: "--ssh-key-path <path>", Desc: "SSH private key path for remote installation"},
				{Cmd: "--force", Desc: "Force new download and installation"},
				{Cmd: "--no-download", Desc: "Skip downloading k0s binary (expects it to be on remote nodes)"},
			}),
		},
		Opts:       InstallK0sOpts{GlobalOptions: opts},
		Env:        env.NewEnv(),
		FileWriter: intutil.NewFilesystemWriter(),
	}
	k0s.cmd.Flags().StringVarP(&k0s.Opts.Version, "version", "v", installer.DefaultK0sVersion, "Version of k0s to install")
	k0s.cmd.Flags().StringVar(&k0s.Opts.K0sctlVersion, "k0sctl-version", installer.DefaultK0sctlVersion, "Version of k0sctl to use")
	k0s.cmd.Flags().StringVarP(&k0s.Opts.Package, "package", "p", "", "Package file (e.g. codesphere-v1.2.3-installer-lite.tar.gz) to load k0s from")
	k0s.cmd.Flags().StringVar(&k0s.Opts.InstallConfig, "install-config", "", "Path to Codesphere install-config file (required)")
	k0s.cmd.Flags().StringVar(&k0s.Opts.SSHKeyPath, "ssh-key-path", "", "SSH private key path for remote installation")
	k0s.cmd.Flags().BoolVarP(&k0s.Opts.Force, "force", "f", false, "Force new download and installation")
	k0s.cmd.Flags().BoolVar(&k0s.Opts.NoDownload, "no-download", false, "Skip downloading k0s binary")

	k0s.cmd.Flags().StringVar(&k0s.Opts.Vault, "vault", "", "Path to prod.vault.yaml to save the kubeconfig into (optional)")
	k0s.cmd.Flags().StringVar(&k0s.Opts.VaultPrivKey, "vault-priv-key", "", "Path to the age private key to decrypt the vault (optional, for SOPS-encrypted vaults)")
	k0s.cmd.Flags().StringVar(&k0s.Opts.VaultType, "vault-type", "sops", "Vault storage type (sops or plain)")

	_ = k0s.cmd.MarkFlagRequired("install-config")

	util.AddCmd(install, k0s.cmd)

	k0s.cmd.RunE = k0s.RunE
}

const vaultSecretNameKubeconfig = "kubeConfig"

// InstallK0s generates a k0sctl config from the command options and installs k0s.
func (c *InstallK0sCmd) InstallK0s(pm installer.PackageManager, k0s installer.K0sManager, k0sctl installer.K0sctlManager) error {
	if err := c.FileWriter.MkdirAll(c.Env.GetOmsWorkdir(), 0755); err != nil {
		return fmt.Errorf("failed to create oms workdir: %w", err)
	}

	generator := k0sctlConfigGenerator{
		Opts:       c.Opts.K0sctlConfigOpts,
		Verbose:    c.Opts.Verbose,
		Env:        c.Env,
		FileWriter: c.FileWriter,
	}

	k0sctlConfigPath, err := generator.Generate(pm, k0s, "")
	if err != nil {
		return err
	}

	k0sctlPath, err := c.downloadK0sctl(k0sctl)
	if err != nil {
		return err
	}

	if err := c.deployK0sCluster(k0sctl, k0sctlPath, k0sctlConfigPath); err != nil {
		return fmt.Errorf("failed to deploy k0s cluster: %w", err)
	}

	if c.Opts.Vault != "" {
		if err := c.saveKubeconfigToVault(k0sctl, k0sctlConfigPath, k0sctlPath); err != nil {
			return fmt.Errorf("failed to save kubeconfig to vault: %w", err)
		}
	}

	return nil
}

func (c *InstallK0sCmd) downloadK0sctl(k0sctl installer.K0sctlManager) (string, error) {
	log.Println("Downloading k0sctl...")

	k0sctlPath, err := k0sctl.Download(c.Opts.K0sctlVersion, installer.DownloadOptions{Force: c.Opts.Force})
	if err != nil {
		return "", fmt.Errorf("failed to download k0sctl: %w", err)
	}
	return k0sctlPath, nil
}

func (c *InstallK0sCmd) deployK0sCluster(k0sctl installer.K0sctlManager, k0sctlPath string, k0sctlConfigPath string) error {
	log.Println("Applying k0sctl configuration to deploy k0s cluster...")
	if err := k0sctl.Apply(k0sctlConfigPath, k0sctlPath, c.Opts.Force); err != nil {
		return fmt.Errorf("failed to apply k0sctl config: %w", err)
	}

	log.Println("k0s cluster deployed successfully!")
	log.Printf("To manage your cluster, use: %s kubeconfig --config %s", k0sctlPath, k0sctlConfigPath)

	return nil
}

func (c *InstallK0sCmd) saveKubeconfigToVault(k0sctl installer.K0sctlManager, k0sctlConfigPath, k0sctlPath string) error {
	log.Println("Retrieving kubeconfig from k0sctl for vault...")
	kubeconfigContent, err := k0sctl.GetKubeconfig(k0sctlConfigPath, k0sctlPath)
	if err != nil {
		return fmt.Errorf("failed to retrieve kubeconfig from k0sctl: %w", err)
	}
	kubeconfigContent = strings.TrimRight(kubeconfigContent, "\n\r")

	store, err := c.vaultStore()
	if err != nil {
		return err
	}

	vaultData, err := store.LoadOrCreate()
	if err != nil {
		return fmt.Errorf("failed to load vault: %w", err)
	}

	if vaultData.GetSecret(vaultSecretNameKubeconfig) != nil {
		log.Printf("Updating existing %s secret in vault", vaultSecretNameKubeconfig)
	}

	vaultData.SetSecret(files.SecretEntry{
		Name: vaultSecretNameKubeconfig,
		File: &files.SecretFile{
			Name:    vaultSecretNameKubeconfig,
			Content: kubeconfigContent,
		},
	})

	if err := store.Save(vaultData); err != nil {
		return err
	}

	log.Printf("Saved kubeconfig to %s", c.Opts.Vault)
	return nil
}

func (c *InstallK0sCmd) vaultStore() (vault.Vault, error) {
	vault, err := vault.NewFromString(c.Opts.VaultType, vault.Options{Path: c.Opts.Vault, AgeKey: c.Opts.VaultPrivKey})
	if err != nil {
		return nil, fmt.Errorf("failed to load vault: %w", err)
	}
	return vault, nil
}
