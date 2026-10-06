// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package k0s

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

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
	Version       string
	K0sctlVersion string
	Package       string
	InstallConfig string
	SSHKeyPath    string
	Force         bool
	NoDownload    bool
	Single        bool
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
			With --single, install a controller and worker on this machine without k0sctl or an install-config.
			
			For multi-node deployment, provide a Codesphere install-config file, which will:
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
				{Cmd: "--single --package <file>", Desc: "Install a single-node cluster on this machine using bundled k0s"},
			}),
		},
		Opts:       InstallK0sOpts{GlobalOptions: opts},
		Env:        env.NewEnv(),
		FileWriter: intutil.NewFilesystemWriter(),
	}
	k0s.cmd.Flags().StringVarP(&k0s.Opts.Version, "version", "v", installer.DefaultK0sVersion, "Version of k0s to install")
	k0s.cmd.Flags().StringVar(&k0s.Opts.K0sctlVersion, "k0sctl-version", installer.DefaultK0sctlVersion, "Version of k0sctl to use")
	k0s.cmd.Flags().StringVarP(&k0s.Opts.Package, "package", "p", "", "Package file (e.g. codesphere-v1.2.3-installer-lite.tar.gz) to load k0s from")
	k0s.cmd.Flags().StringVar(&k0s.Opts.InstallConfig, "install-config", "", "Path to Codesphere install-config file (required unless --single)")
	k0s.cmd.Flags().StringVar(&k0s.Opts.SSHKeyPath, "ssh-key-path", "", "SSH private key path for remote installation")
	k0s.cmd.Flags().BoolVarP(&k0s.Opts.Force, "force", "f", false, "Force new download and installation")
	k0s.cmd.Flags().BoolVar(&k0s.Opts.NoDownload, "no-download", false, "Skip downloading k0s binary")
	k0s.cmd.Flags().BoolVar(&k0s.Opts.Single, "single", false, "Install a single-node controller and worker on this machine")

	k0s.cmd.Flags().StringVar(&k0s.Opts.Vault, "vault", "", "Path to prod.vault.yaml to save the kubeconfig into (optional)")
	k0s.cmd.Flags().StringVar(&k0s.Opts.VaultPrivKey, "vault-priv-key", "", "Path to the age private key to decrypt the vault (optional, for SOPS-encrypted vaults)")
	k0s.cmd.Flags().StringVar(&k0s.Opts.VaultType, "vault-type", "sops", "Vault storage type (sops or plain)")

	util.AddCmd(install, k0s.cmd)

	k0s.cmd.RunE = k0s.RunE
}

const (
	defaultK0sPath            = "kubernetes/files/k0s"
	vaultSecretNameKubeconfig = "kubeConfig"
)

func (c *InstallK0sCmd) InstallK0s(pm installer.PackageManager, k0s installer.K0sManager, k0sctl installer.K0sctlManager) error {
	if c.Opts.Single && c.Opts.InstallConfig != "" {
		return fmt.Errorf("--install-config cannot be used with --single")
	}

	if !c.Opts.Single && c.Opts.InstallConfig == "" {
		return fmt.Errorf("--install-config is required unless --single is set")
	}
	if err := c.FileWriter.MkdirAll(c.Env.GetOmsWorkdir(), 0755); err != nil {
		return fmt.Errorf("failed to create oms workdir: %w", err)
	}

	if c.Opts.Single {
		return c.installSingle(pm, k0s)
	}

	config, err := c.loadInstallConfig()
	if err != nil {
		return err
	}

	k0sVersion, err := c.determineK0sVersion(k0s)
	if err != nil {
		return err
	}

	k0sBinaryPath, err := c.getK0sBinaryPath(pm, k0s, k0sVersion)
	if err != nil {
		return err
	}

	k0sctlPath, err := c.downloadK0sctl(k0sctl)
	if err != nil {
		return err
	}

	k0sctlConfigPath, err := c.generateK0sctlConfig(config, k0sVersion, k0sBinaryPath)
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

func (c *InstallK0sCmd) installSingle(pm installer.PackageManager, k0s installer.K0sManager) error {
	if c.Opts.SSHKeyPath != "" || c.Opts.Vault != "" {
		return fmt.Errorf("--ssh-key-path and --vault are not supported with --single")
	}

	if os.Geteuid() != 0 {
		return fmt.Errorf("--single requires root privileges")
	}

	version, err := c.determineK0sVersion(k0s)
	if err != nil {
		return err
	}

	path, err := c.getK0sBinaryPath(pm, k0s, version)
	if err != nil {
		return err
	}

	const installedPath = "/usr/local/bin/k0s"
	if path != "" && path != installedPath {
		stagedPath := installedPath + ".new"
		if out, err := exec.Command("install", "-m", "0755", path, stagedPath).CombinedOutput(); err != nil {
			return fmt.Errorf("failed to install k0s binary: %w: %s", err, strings.TrimSpace(string(out)))
		}

		if err := os.Rename(stagedPath, installedPath); err != nil {
			return fmt.Errorf("failed to activate k0s binary: %w", err)
		}
	}

	if _, err := os.Stat(installedPath); err != nil {
		return fmt.Errorf("k0s binary is unavailable at %s: %w", installedPath, err)
	}

	if _, err := os.Stat("/etc/systemd/system/k0scontroller.service"); os.IsNotExist(err) {
		if out, err := exec.Command(installedPath, "install", "controller", "--single", "--enable-worker", "--no-taints=true").CombinedOutput(); err != nil {
			return fmt.Errorf("failed to install k0s controller: %w: %s", err, strings.TrimSpace(string(out)))
		}
	} else if err != nil {
		return fmt.Errorf("failed to inspect k0s service: %w", err)
	}

	if err := exec.Command("systemctl", "is-active", "--quiet", "k0scontroller").Run(); err != nil {
		if out, err := exec.Command(installedPath, "start").CombinedOutput(); err != nil {
			return fmt.Errorf("failed to start k0s: %w: %s", err, strings.TrimSpace(string(out)))
		}
	}

	statusCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	if err := waitForK0sReady(statusCtx, 2*time.Second, func(ctx context.Context) ([]byte, error) {
		return exec.CommandContext(ctx, installedPath, "status", "-o", "json").CombinedOutput()
	}); err != nil {
		return fmt.Errorf("failed to wait for k0s installation: %w", err)
	}

	nodeCtx, cancelNodeWait := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancelNodeWait()
	node, err := waitForK0sNode(nodeCtx, 2*time.Second, func(ctx context.Context) ([]byte, error) {
		return exec.CommandContext(ctx, installedPath, "kubectl", "get", "nodes", "-o", "name").CombinedOutput()
	})
	if err != nil {
		return fmt.Errorf("failed to wait for k0s node registration: %w", err)
	}

	if out, err := exec.CommandContext(nodeCtx, installedPath, "kubectl", "wait", "--for=condition=Ready", node, "--timeout=30m").CombinedOutput(); err != nil {
		return fmt.Errorf("failed to wait for k0s node: %w: %s", err, strings.TrimSpace(string(out)))
	}

	kubeconfig, err := exec.Command(installedPath, "kubeconfig", "admin").Output()
	if err != nil {
		return fmt.Errorf("failed to get k0s kubeconfig: %w", err)
	}

	kubeDir := "/root/.kube"
	if err := os.MkdirAll(kubeDir, 0700); err != nil {
		return fmt.Errorf("failed to create kubeconfig directory: %w", err)
	}

	if err := os.WriteFile(filepath.Join(kubeDir, "config"), kubeconfig, 0600); err != nil {
		return fmt.Errorf("failed to write kubeconfig: %w", err)
	}

	return nil
}

func waitForK0sNode(ctx context.Context, interval time.Duration, listNodes func(context.Context) ([]byte, error)) (string, error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var lastResult string
	for {
		out, err := listNodes(ctx)
		if err == nil {
			for _, node := range strings.Fields(string(out)) {
				if strings.HasPrefix(node, "node/") {
					return node, nil
				}
			}
			lastResult = "no nodes registered: " + strings.TrimSpace(string(out))
		} else {
			lastResult = fmt.Sprintf("%v: %s", err, strings.TrimSpace(string(out)))
		}

		select {
		case <-ctx.Done():
			return "", fmt.Errorf("%w (last node query: %s)", ctx.Err(), lastResult)
		case <-ticker.C:
		}
	}
}

func waitForK0sReady(ctx context.Context, interval time.Duration, status func(context.Context) ([]byte, error)) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var lastStatus string

	for {
		out, err := status(ctx)
		if err == nil {
			var state struct {
				Pid                         int
				Role                        string
				WorkerToAPIConnectionStatus struct {
					Success bool
					Message string
				}
			}
			if err = json.Unmarshal(out, &state); err == nil && state.Pid > 0 && state.Role == "controller" && state.WorkerToAPIConnectionStatus.Success {
				return nil
			}

			if err == nil {
				lastStatus = fmt.Sprintf("pid=%d role=%q apiReady=%t: %s", state.Pid, state.Role, state.WorkerToAPIConnectionStatus.Success, state.WorkerToAPIConnectionStatus.Message)
			}
		}

		if err != nil {
			lastStatus = fmt.Sprintf("%v: %s", err, strings.TrimSpace(string(out)))
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("%w (last k0s status: %s)", ctx.Err(), lastStatus)
		case <-ticker.C:
		}
	}
}

func (c *InstallK0sCmd) loadInstallConfig() (*files.RootConfig, error) {
	config, err := installer.NewConfig().ParseConfigYaml(c.Opts.InstallConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to load install-config: %w", err)
	}

	if !config.Kubernetes.ManagedByCodesphere {
		return nil, fmt.Errorf("install-config specifies external Kubernetes, k0s installation is only supported for Codesphere-managed Kubernetes")
	}

	return &config, nil
}

func (c *InstallK0sCmd) determineK0sVersion(k0s installer.K0sManager) (string, error) {
	k0sVersion := c.Opts.Version
	if k0sVersion == "" {
		var err error
		k0sVersion, err = k0s.GetLatestVersion()
		if err != nil {
			return "", fmt.Errorf("failed to get latest k0s version: %w", err)
		}
		log.Printf("Using latest k0s version: %s", k0sVersion)
	}
	return k0sVersion, nil
}

func (c *InstallK0sCmd) getK0sBinaryPath(pm installer.PackageManager, k0s installer.K0sManager, k0sVersion string) (string, error) {
	if c.Opts.NoDownload {
		return "", nil
	}

	if c.Opts.Package != "" {
		if err := pm.ExtractDependency(defaultK0sPath, c.Opts.Force, c.Opts.Verbose); err != nil {
			return "", fmt.Errorf("failed to extract k0s from package: %w", err)
		}
		return pm.GetDependencyPath(defaultK0sPath), nil
	}

	k0sBinaryPath, err := k0s.Download(k0sVersion, c.Opts.Force, false)
	if err != nil {
		return "", fmt.Errorf("failed to download k0s: %w", err)
	}
	return k0sBinaryPath, nil
}

func (c *InstallK0sCmd) downloadK0sctl(k0sctl installer.K0sctlManager) (string, error) {
	log.Println("Downloading k0sctl...")
	k0sctlPath, err := k0sctl.Download(c.Opts.K0sctlVersion, c.Opts.Force, false)
	if err != nil {
		return "", fmt.Errorf("failed to download k0sctl: %w", err)
	}
	return k0sctlPath, nil
}

func (c *InstallK0sCmd) generateK0sctlConfig(config *files.RootConfig, k0sVersion string, k0sBinaryPath string) (string, error) {
	log.Println("Generating k0sctl configuration from install-config...")
	k0sctlConfig, err := installer.GenerateK0sctlConfig(config, k0sVersion, c.Opts.SSHKeyPath, k0sBinaryPath)
	if err != nil {
		return "", fmt.Errorf("failed to generate k0sctl config: %w", err)
	}

	k0sctlConfigData, err := k0sctlConfig.Marshal()
	if err != nil {
		return "", fmt.Errorf("failed to marshal k0sctl config: %w", err)
	}

	k0sctlConfigPath := filepath.Join(c.Env.GetOmsWorkdir(), fmt.Sprintf("k0sctl-config-%s.yaml", config.Datacenter.Name))
	if err := c.FileWriter.WriteFile(k0sctlConfigPath, k0sctlConfigData, 0644); err != nil {
		return "", fmt.Errorf("failed to write k0sctl config: %w", err)
	}

	log.Printf("Generated k0sctl configuration at %s", k0sctlConfigPath)
	return k0sctlConfigPath, nil
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

	vault, err := c.loadOrCreateVault()
	if err != nil {
		return fmt.Errorf("failed to load vault: %w", err)
	}

	for _, s := range vault.Secrets {
		if s.Name == vaultSecretNameKubeconfig {
			log.Printf("Updating existing %s secret in vault", vaultSecretNameKubeconfig)
			break
		}
	}

	vault.SetSecret(files.SecretEntry{
		Name: vaultSecretNameKubeconfig,
		File: &files.SecretFile{
			Name:    vaultSecretNameKubeconfig,
			Content: kubeconfigContent,
		},
	})

	store, err := c.vaultStore()
	if err != nil {
		return err
	}

	if err := store.Save(vault); err != nil {
		return err
	}

	log.Printf("Saved kubeconfig to %s", c.Opts.Vault)
	return nil
}

func (c *InstallK0sCmd) loadOrCreateVault() (*files.InstallVault, error) {
	store, err := c.vaultStore()
	if err != nil {
		return nil, err
	}

	data, err := store.LoadOrCreate()
	if err != nil {
		return nil, fmt.Errorf("failed to load vault: %w", err)
	}

	return data, nil
}

func (c *InstallK0sCmd) vaultStore() (vault.Vault, error) {
	vault, err := vault.NewFromString(c.Opts.VaultType, vault.Options{Path: c.Opts.Vault, AgeKey: c.Opts.VaultPrivKey})
	if err != nil {
		return nil, fmt.Errorf("failed to load vault: %w", err)
	}
	return vault, nil
}
