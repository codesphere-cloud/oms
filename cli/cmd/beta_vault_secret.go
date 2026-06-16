// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"

	packageio "github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/util"
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
	*GlobalOptions
	VaultFile  string
	AgeKeyPath string
	Namespace  string
	SecretName string
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

	creator := installer.NewVaultSecretCreator(kubeClient)
	return creator.CreateSecretFromFile(c.cmd.Context(), c.Opts.VaultFile, c.Opts.AgeKeyPath, c.Opts.Namespace, c.Opts.SecretName)
}

func AddBetaVaultSecretCmd(parentCmd *cobra.Command, opts *GlobalOptions) {
	cmd := BetaVaultSecretCmd{
		cmd: &cobra.Command{
			Use:   "vault-secret",
			Short: "Create a Kubernetes secret from a SOPS-encrypted vault file",
			Long: packageio.Long(`Create a Kubernetes secret from a SOPS-encrypted prod.vault.yaml file.
				Reads the encrypted vault file, decrypts it using the age key, and creates a Kubernetes secret
				with all the vault entries as key-value pairs in the target cluster.`),
			Example: formatExamples("vault-secret", []packageio.Example{
				{Cmd: "--vault-file prod.vault.yaml --namespace default --secret-name vault-secrets", Desc: "Create secret using default age key location"},
				{Cmd: "--vault-file prod.vault.yaml --age-key /path/to/age_key.txt --namespace kube-system --secret-name cluster-secrets", Desc: "Create secret with explicit age key path"},
			}),
		},
		Opts: BetaVaultSecretOpts{GlobalOptions: opts},
	}

	cmd.cmd.Flags().StringVar(&cmd.Opts.VaultFile, "vault-file", "", "Path to the SOPS-encrypted vault file (required)")
	cmd.cmd.Flags().StringVar(&cmd.Opts.AgeKeyPath, "age-key", "", "Path to the age key file (optional, will use defaults if not provided)")
	cmd.cmd.Flags().StringVar(&cmd.Opts.Namespace, "namespace", "codesphere", "Kubernetes namespace where the secret will be created")
	cmd.cmd.Flags().StringVar(&cmd.Opts.SecretName, "secret-name", "cs-vault", "Name of the Kubernetes secret to create")

	util.MarkFlagRequired(cmd.cmd, "vault-file")

	cmd.cmd.RunE = cmd.RunE
	AddCmd(parentCmd, cmd.cmd)
}
