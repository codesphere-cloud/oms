// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	packageio "github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/spf13/cobra"

	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/vault/sops"
)

// BetaBootstrapOpenBaoCmd wraps the cobra command and options for 'oms beta bootstrap-openbao'.
type BetaBootstrapOpenBaoCmd struct {
	cmd  *cobra.Command
	Opts *BetaBootstrapOpenBaoOpts
}

// BetaBootstrapOpenBaoOpts holds the CLI flags for the OpenBao bootstrap.
type BetaBootstrapOpenBaoOpts struct {
	*util.GlobalOptions
	Namespace    string
	DRBackupPath string
	Restore      bool
	Timeout      time.Duration
	AgeKeyFile   string
	StepTimer    bool
}

func (c *BetaBootstrapOpenBaoCmd) RunE(_ *cobra.Command, _ []string) error {
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
	recipient, keyPath, err := sops.ResolveAgeKey(c.Opts.AgeKeyFile, fallbackDir)
	if err != nil {
		return fmt.Errorf("resolving age key: %w", err)
	}

	b, err := installer.NewOpenBaoBootstrapper(installer.OpenBaoBootstrapConfig{
		Namespace:    c.Opts.Namespace,
		DRBackupPath: c.Opts.DRBackupPath,
		Restore:      c.Opts.Restore,
		Timeout:      c.Opts.Timeout,
		AgeRecipient: recipient,
		AgeKeyPath:   keyPath,
		StepTimer:    c.Opts.StepTimer,
	})
	if err != nil {
		return fmt.Errorf("initializing openbao bootstrapper: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return b.Bootstrap(ctx)
}

// AddBetaBootstrapOpenBaoCmd registers the bootstrap-openbao subcommand under beta.
func AddBetaBootstrapOpenBaoCmd(beta *cobra.Command, opts *util.GlobalOptions) {
	openbao := BetaBootstrapOpenBaoCmd{
		cmd: &cobra.Command{
			Use:   "bootstrap-openbao",
			Short: "Export or restore the unseal keys of the ArgoCD-deployed OpenBao",
			Long: packageio.Long(`Handle the OpenBao steps ArgoCD cannot do for the instance deployed by the openbao chart.

				By default, waits for the bank-vaults operator to initialize the instance and exports
				the openbao-unseal-keys Secret to a SOPS-encrypted DR backup. Sync the OpenBao ArgoCD
				app first; the command fails if the Vault CR does not exist yet.

				With --restore, seeds openbao-unseal-keys from the DR backup so bank-vaults unseals
				with the original keys instead of re-initializing. Run it before the first sync of the
				OpenBao ArgoCD app, then sync while the command waits for the instance to become ready.`),
			Example: util.FormatExamples("beta bootstrap-openbao", []packageio.Example{
				{Cmd: "--dr-backup-path ./backups/cluster-1.enc.json", Desc: "Export the unseal keys to a new DR backup"},
				{Cmd: "--dr-backup-path ./backups/cluster-1.enc.json --restore", Desc: "Restore the unseal keys from an existing DR backup"},
				{Cmd: "--dr-backup-path ./backups/cluster-1.enc.json --timeout 10m", Desc: "Extended timeout for slower clusters"},
			}),
		},
		Opts: &BetaBootstrapOpenBaoOpts{GlobalOptions: opts},
	}
	openbao.cmd.Flags().StringVarP(&openbao.Opts.Namespace, "namespace", "n", installer.DefaultOpenBaoNamespace, "Kubernetes namespace of the OpenBao deployment")
	openbao.cmd.Flags().StringVar(&openbao.Opts.DRBackupPath, "dr-backup-path", "", "Path of the SOPS-encrypted DR backup file (required)")
	openbao.cmd.Flags().BoolVar(&openbao.Opts.Restore, "restore", false, "Seed the unseal keys from the DR backup instead of exporting them")
	openbao.cmd.Flags().DurationVar(&openbao.Opts.Timeout, "timeout", 5*time.Minute, "Timeout for waiting on the Vault CR and initialization")
	openbao.cmd.Flags().StringVarP(&openbao.Opts.AgeKeyFile, "age-key-file", "k", "", "Path to age private key file for SOPS encryption/decryption (auto-detected if not set)")
	openbao.cmd.Flags().BoolVar(&openbao.Opts.StepTimer, util.TimerFlag, false, util.ShowStepDurationHelp)

	util.MarkFlagRequired(openbao.cmd, "dr-backup-path")

	util.AddCmd(beta, openbao.cmd)

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
