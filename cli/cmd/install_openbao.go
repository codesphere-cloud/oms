// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	packageio "github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/spf13/cobra"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/util"
)

// InstallOpenBaoCmd wraps the cobra command and options for 'oms install openbao'.
type InstallOpenBaoCmd struct {
	cmd  *cobra.Command
	Opts *InstallOpenBaoOpts
}

// InstallOpenBaoOpts holds the CLI flags for the OpenBao installer.
type InstallOpenBaoOpts struct {
	*GlobalOptions
	SecretsEngineName string
	BaoUsername       string
	DRBackupPath      string
	Replicas          int
	StorageSize       string
	Timeout           time.Duration
	AgeKeyFile        string
	Yes               bool
}

func (c *InstallOpenBaoCmd) RunE(_ *cobra.Command, _ []string) error {
	if err := validateOpenBaoPrereqs(); err != nil {
		return err
	}

	// If --age-key-file is provided, set SOPS_AGE_KEY_FILE so ResolveAgeKey
	// picks it up. Otherwise, fall back to the normal auto-discovery chain.
	if c.Opts.AgeKeyFile != "" {
		os.Setenv("SOPS_AGE_KEY_FILE", c.Opts.AgeKeyFile)
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("determining user config directory: %w", err)
	}
	fallbackDir := filepath.Join(configDir, "sops", "age")

	recipient, keyPath, err := installer.ResolveAgeKey(fallbackDir)
	if err != nil {
		return fmt.Errorf("resolving age key: %w", err)
	}

	cfg := installer.OpenBaoInstallerConfig{
		SecretsEngineName: c.Opts.SecretsEngineName,
		Username:          c.Opts.BaoUsername,
		DRBackupPath:      c.Opts.DRBackupPath,
		Replicas:          c.Opts.Replicas,
		StorageSize:       c.Opts.StorageSize,
		Timeout:           c.Opts.Timeout,
		AgeRecipient:      recipient,
		AgeKeyPath:        keyPath,
	}

	inst, err := installer.NewOpenBaoInstaller(cfg)
	if err != nil {
		return fmt.Errorf("initializing openbao installer: %w", err)
	}

	inst.ConfirmFunc = func() error {
		if c.Opts.Yes {
			return nil
		}

		fmt.Printf("\nWARNING: No DR backup found at: %s\n", c.Opts.DRBackupPath)
		fmt.Println("This will perform a FRESH OpenBao initialization:")
		fmt.Println("  - Existing Vault CR will be deleted")
		fmt.Println("  - All OpenBao pods will be terminated")
		fmt.Println("  - Persistent volume claims (data) will be deleted")
		fmt.Println("  - Existing unseal keys will be removed")
		fmt.Println("")
		fmt.Println("If you intended to restore from a backup, verify --dr-backup-path is correct.")
		fmt.Print("\nType 'yes' to continue: ")

		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("failed to read confirmation: %w", err)
		}
		if strings.TrimSpace(strings.ToLower(input)) != "yes" {
			return fmt.Errorf("aborted: type 'yes' to continue or pass --yes")
		}
		return nil
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return inst.Install(ctx)
}

// AddInstallOpenBaoCmd registers the openbao subcommand under install.
func AddInstallOpenBaoCmd(install *cobra.Command, opts *GlobalOptions) {
	openbao := InstallOpenBaoCmd{
		cmd: &cobra.Command{
			Use:   "openbao",
			Short: "Bootstrap OpenBao with Bank-Vaults Operator and DR backup",
			Long: packageio.Long(`Bootstrap OpenBao using the Bank-Vaults Operator with a KMS-less Day-0 workflow.

				This command performs the full lifecycle:
				1. Pre-flight DR check (restore from SOPS backup if exists)
				2. Generate a secure password for userpass auth
				3. Deploy the Bank-Vaults Operator via Helm
				4. Apply the Vault CR with desired-state configuration
				5. Wait for initialization to complete
				6. Extract and encrypt unseal keys + password as SOPS DR backup

				The command is idempotent and safe to re-run.`),
			Example: formatExamples("install openbao", []packageio.Example{
				{Cmd: "--dr-backup-path ./backups/cluster-1.enc.json", Desc: "Fresh bootstrap with DR backup saved locally"},
				{Cmd: "--dr-backup-path ./backups/cluster-1.enc.json --secrets-engine my-engine --bao-user myuser", Desc: "Custom engine and user"},
				{Cmd: "--dr-backup-path ./backups/cluster-1.enc.json --timeout 10m", Desc: "Extended timeout for slower clusters"},
			}),
		},
		Opts: &InstallOpenBaoOpts{GlobalOptions: opts},
	}
	openbao.cmd.Flags().StringVar(&openbao.Opts.SecretsEngineName, "secrets-engine", "cs-secrets-engine", "Name of the KV-v2 secrets engine to provision")
	openbao.cmd.Flags().StringVar(&openbao.Opts.BaoUsername, "bao-user", "admin", "Username for the userpass auth method (ignored on restore, uses DR backup value)")
	openbao.cmd.Flags().StringVar(&openbao.Opts.DRBackupPath, "dr-backup-path", "", "Path for SOPS-encrypted DR backup file (required)")
	openbao.cmd.Flags().IntVar(&openbao.Opts.Replicas, "replicas", 1, "Number of OpenBao replicas (1 for single-node, odd number >= 3 for HA)")
	openbao.cmd.Flags().StringVar(&openbao.Opts.StorageSize, "storage-size", "10Gi", "PVC storage size for each OpenBao replica")
	openbao.cmd.Flags().DurationVar(&openbao.Opts.Timeout, "timeout", 5*time.Minute, "Timeout for waiting on initialization")
	openbao.cmd.Flags().StringVarP(&openbao.Opts.AgeKeyFile, "age-key-file", "k", "", "Path to age private key file for SOPS encryption/decryption (auto-detected if not set)")
	openbao.cmd.Flags().BoolVarP(&openbao.Opts.Yes, "yes", "y", false, "Auto-approve fresh initialization when no DR backup is found")

	util.MarkFlagRequired(openbao.cmd, "dr-backup-path")

	AddCmd(install, openbao.cmd)

	openbao.cmd.RunE = openbao.RunE
}

// validateOpenBaoPrereqs checks that required external tools are available.
func validateOpenBaoPrereqs() error {
	if _, err := exec.LookPath("sops"); err != nil {
		return fmt.Errorf("sops not found in PATH — install from https://github.com/getsops/sops")
	}
	if _, err := exec.LookPath("age-keygen"); err != nil {
		return fmt.Errorf("age-keygen not found in PATH — install from https://github.com/FiloSottile/age")
	}
	return nil
}
