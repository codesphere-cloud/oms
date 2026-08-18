// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"

	packageio "github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/codesphere-cloud/oms/internal/installer/vault"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/client/config"
)

type BetaVaultSecretCmd struct {
	cmd  *cobra.Command
	Opts BetaVaultSecretOpts
}

type BetaVaultSecretOpts struct {
	*util.GlobalOptions
	VaultFile  string
	AgeKeyPath string
	Namespace  string
	SecretName string
	VaultType  string
}

func (c *BetaVaultSecretCmd) RunE(_ *cobra.Command, _ []string) error {
	kubeConfig, err := ctrlconfig.GetConfig()
	if err != nil {
		return fmt.Errorf("failed to load kubernetes config: %w", err)
	}

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return fmt.Errorf("failed to add kubernetes scheme: %w", err)
	}

	kubeClient, err := ctrlclient.New(kubeConfig, ctrlclient.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	creator := vault.NewVaultSecretCreator(kubeClient)

	store, err := vault.NewFromString(c.Opts.VaultType, vault.Options{Path: c.Opts.VaultFile, AgeKey: c.Opts.AgeKeyPath})
	if err != nil {
		return fmt.Errorf("failed to load vault: %w", err)
	}

	err = creator.CreateSecretFromStore(c.cmd.Context(), store, c.Opts.Namespace, c.Opts.SecretName)
	if err != nil {
		return fmt.Errorf("failed to create secret: %w", err)
	}

	return nil
}

func AddBetaVaultSecretCmd(parentCmd *cobra.Command, opts *util.GlobalOptions) {
	cmd := BetaVaultSecretCmd{
		cmd: &cobra.Command{
			Use:   "vault-secret",
			Short: "Create a Kubernetes secret from a vault file",
			Long: packageio.Long(`Create a Kubernetes secret from a prod.vault.yaml file.
				Loads the selected vault type and creates a Kubernetes secret
				with all the vault entries as key-value pairs in the target cluster.`),
			Example: util.FormatExamples("vault-secret", []packageio.Example{
				{Cmd: "--vault-file prod.vault.yaml --namespace default --secret-name vault-secrets", Desc: "Create secret using default age key location"},
				{Cmd: "--vault-file prod.vault.yaml --age-key /path/to/age_key.txt --namespace kube-system --secret-name cluster-secrets", Desc: "Create secret with explicit age key path"},
			}),
		},
		Opts: BetaVaultSecretOpts{GlobalOptions: opts},
	}

	cmd.cmd.Flags().StringVar(&cmd.Opts.VaultFile, "vault-file", "", "Path to the vault file (required)")
	cmd.cmd.Flags().StringVar(&cmd.Opts.AgeKeyPath, "age-key", "", "Path to the age key file (required for sops unless an age key environment variable is set)")
	cmd.cmd.Flags().StringVar(&cmd.Opts.VaultType, "vault-type", "sops", "Vault storage type (sops or plain)")
	cmd.cmd.Flags().StringVar(&cmd.Opts.Namespace, "namespace", "codesphere", "Kubernetes namespace where the secret will be created")
	cmd.cmd.Flags().StringVar(&cmd.Opts.SecretName, "secret-name", "cs-vault", "Name of the Kubernetes secret to create")

	util.MarkFlagRequired(cmd.cmd, "vault-file")

	cmd.cmd.RunE = cmd.RunE
	util.AddCmd(parentCmd, cmd.cmd)
}
