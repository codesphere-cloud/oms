// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
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
	"github.com/codesphere-cloud/oms/internal/installer/vault"
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
	Namespace         string
	SecretsEngineName string
	BaoUsername       string
	DRBackupPath      string
	Replicas          int
	StorageSize       string
	Timeout           time.Duration
	AgeKeyFile        string
	Yes               bool
	OpenBaoImage      string
	BankVaultsImage   string
	OperatorImage     string
	OperatorChartRepo string
}

func (c *InstallOpenBaoCmd) RunE(_ *cobra.Command, _ []string) error {
	if err := validateOpenBaoPrereqs(); err != nil {
		return err
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("determining user config directory: %w", err)
	}
	fallbackDir := filepath.Join(configDir, "sops", "age")

	// Pass --age-key-file explicitly so ResolveAgeKey prefers it without
	// mutating the process environment. When empty, the normal
	// auto-discovery chain (env vars, default location, generation) applies.
	recipient, keyPath, err := vault.ResolveAgeKey(c.Opts.AgeKeyFile, fallbackDir)
	if err != nil {
		return fmt.Errorf("resolving age key: %w", err)
	}

	cfg := installer.OpenBaoInstallerConfig{
		Namespace:         c.Opts.Namespace,
		SecretsEngineName: c.Opts.SecretsEngineName,
		Username:          c.Opts.BaoUsername,
		DRBackupPath:      c.Opts.DRBackupPath,
		Replicas:          c.Opts.Replicas,
		StorageSize:       c.Opts.StorageSize,
		Timeout:           c.Opts.Timeout,
		AgeRecipient:      recipient,
		AgeKeyPath:        keyPath,
		// Optional registry credentials for the private OpenBao/bank-vaults image
		// mirror (any OCI registry, not just ghcr.io). When both are set the
		// installer creates a pull secret and wires it onto the openbao
		// ServiceAccount; when unset, behavior is unchanged.
		RegistryUser:     os.Getenv("OMS_REGISTRY_USER"),
		RegistryPassword: os.Getenv("OMS_REGISTRY_PASSWORD"),
		// Image/chart overrides for mirrored OCI registries. Defaults are set on
		// the flags (the installer's Default* values).
		OpenBaoImage:      c.Opts.OpenBaoImage,
		BankVaultsImage:   c.Opts.BankVaultsImage,
		OperatorImage:     c.Opts.OperatorImage,
		OperatorChartRepo: c.Opts.OperatorChartRepo,
	}

	inst, err := installer.NewOpenBaoInstaller(cfg)
	if err != nil {
		return fmt.Errorf("initializing openbao installer: %w", err)
	}

	inst.ConfirmFunc = func() error {
		if c.Opts.Yes {
			return nil
		}

		log.Printf("\nWARNING: No DR backup found at: %s", c.Opts.DRBackupPath)
		log.Println("This will perform a FRESH OpenBao initialization:")
		log.Println("  - Existing Vault CR will be deleted")
		log.Println("  - All OpenBao pods will be terminated")
		log.Println("  - Persistent volume claims (data) will be deleted")
		log.Println("  - Existing unseal keys will be removed")
		log.Println("If you intended to restore from a backup, verify --dr-backup-path is correct.")
		log.Print("Type 'yes' to continue: ")

		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("failed to read confirmation: %w", err)
		}
		if strings.TrimSpace(strings.ToLower(input)) != "yes" {
			return fmt.Errorf("installation cancelled: confirmation not given (type 'yes' or pass --yes to proceed)")
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

				The command is idempotent and safe to re-run.

				By default the OpenBao, bank-vaults, and operator images and the operator
				Helm chart are pulled from the private Codesphere registry mirror. Use the
				--openbao-image, --bank-vaults-image, --operator-image and
				--operator-chart-repo flags to repoint them at your own mirrored OCI
				registry.

				Because the default registry is private, set both environment variables
				below: the installer creates an image pull secret (with an entry for every
				registry host the configured images live on), attaches it to the openbao
				ServiceAccount and operator pod, and uses the credentials to authenticate
				the operator chart pull. Leave them unset only on clusters with node-level
				registry access or fully public images.

				Environment variables:
				  OMS_REGISTRY_USER      Registry username (e.g. GitHub user for ghcr.io)
				  OMS_REGISTRY_PASSWORD  Registry token/PAT (read:packages for ghcr.io)`),
			Example: formatExamples("install openbao", []packageio.Example{
				{Cmd: "--dr-backup-path ./backups/cluster-1.enc.json", Desc: "Fresh bootstrap with DR backup saved locally"},
				{Cmd: "--dr-backup-path ./backups/cluster-1.enc.json --secrets-engine my-engine --bao-user myuser", Desc: "Custom engine and user"},
				{Cmd: "--dr-backup-path ./backups/cluster-1.enc.json --timeout 10m", Desc: "Extended timeout for slower clusters"},
				{Cmd: "--dr-backup-path ./backups/cluster-1.enc.json --openbao-image my-mirror.example.com/openbao/openbao:2.5.5 --operator-chart-repo oci://my-mirror.example.com/bank-vaults/helm-charts", Desc: "Use a mirrored OCI registry (set OMS_REGISTRY_USER/OMS_REGISTRY_PASSWORD)"},
			}),
		},
		Opts: &InstallOpenBaoOpts{GlobalOptions: opts},
	}
	openbao.cmd.Flags().StringVarP(&openbao.Opts.Namespace, "namespace", "n", installer.DefaultOpenBaoNamespace, "Kubernetes namespace for OpenBao deployment")
	openbao.cmd.Flags().StringVar(&openbao.Opts.SecretsEngineName, "secrets-engine", "cs-secrets-engine", "Name of the KV-v2 secrets engine to provision")
	openbao.cmd.Flags().StringVar(&openbao.Opts.BaoUsername, "bao-user", "admin", "Username for the userpass auth method (ignored on restore, uses DR backup value)")
	openbao.cmd.Flags().StringVar(&openbao.Opts.DRBackupPath, "dr-backup-path", "", "Path for SOPS-encrypted DR backup file (required)")
	openbao.cmd.Flags().IntVar(&openbao.Opts.Replicas, "replicas", 3, "Number of OpenBao replicas (1 for single-node, odd number >= 3 for HA)")
	openbao.cmd.Flags().StringVar(&openbao.Opts.StorageSize, "storage-size", "10Gi", "PVC storage size for each OpenBao replica")
	openbao.cmd.Flags().DurationVar(&openbao.Opts.Timeout, "timeout", 5*time.Minute, "Timeout for waiting on initialization")
	openbao.cmd.Flags().StringVarP(&openbao.Opts.AgeKeyFile, "age-key-file", "k", "", "Path to age private key file for SOPS encryption/decryption (auto-detected if not set)")
	openbao.cmd.Flags().BoolVarP(&openbao.Opts.Yes, "yes", "y", false, "Auto-approve re-initialization of an existing deployment when no DR backup is found")
	openbao.cmd.Flags().StringVar(&openbao.Opts.OpenBaoImage, "openbao-image", installer.DefaultOpenBaoImage, "OpenBao server image (override for a mirrored OCI registry)")
	openbao.cmd.Flags().StringVar(&openbao.Opts.BankVaultsImage, "bank-vaults-image", installer.DefaultBankVaultsImage, "Bank-Vaults configurer image (override for a mirrored OCI registry)")
	openbao.cmd.Flags().StringVar(&openbao.Opts.OperatorImage, "operator-image", installer.DefaultOperatorImage, "Bank-Vaults operator pod image (override for a mirrored OCI registry)")
	openbao.cmd.Flags().StringVar(&openbao.Opts.OperatorChartRepo, "operator-chart-repo", installer.DefaultBankVaultsChartRepo, "OCI repo hosting the vault-operator Helm chart (override for a mirrored OCI registry)")

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
