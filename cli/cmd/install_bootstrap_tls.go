// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"

	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/vault"
	intutil "github.com/codesphere-cloud/oms/internal/util"
	"github.com/spf13/cobra"
)

// AddInstallBootstrapTLSCmd registers the jumpbox-only DNS-01 bootstrap step.
func AddInstallBootstrapTLSCmd(parent *cobra.Command, _ *util.GlobalOptions) {
	var (
		opts              installer.BootstrapTLSOptions
		vaultPath, ageKey string
	)

	cmd := &cobra.Command{
		Use: "bootstrap-tls", Short: "Issue bootstrap certificates on a connected GCP jumpbox",
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			store, err := vault.New(vault.TypeSOPS, vault.Options{Path: vaultPath, AgeKey: ageKey})
			if err != nil {
				return fmt.Errorf("initializing bootstrap TLS vault: %w", err)
			}

			return installer.EnsureBootstrapTLS(opts, intutil.NewFilesystemWriter(), store, installer.PodmanCertbotRunner{})
		},
	}
	cmd.Flags().StringVar(&opts.ConfigPath, "config", "", "Data center install config")
	cmd.Flags().StringVar(&opts.DNSProject, "dns-project", "", "Project hosting the public Cloud DNS zone")
	cmd.Flags().StringVar(&opts.CacheDir, "cache-dir", "/root/oms-acme", "Persistent ACME account and certificate directory")
	cmd.Flags().StringSliceVar(&opts.Domains, "domain", nil, "Additional certificate DNS names")
	cmd.Flags().StringVar(&vaultPath, "vault", "", "Encrypted data center vault")
	cmd.Flags().StringVar(&ageKey, "vault-priv-key", "", "Age key file for the data center vault")
	_ = cmd.MarkFlagRequired("config")
	_ = cmd.MarkFlagRequired("dns-project")
	_ = cmd.MarkFlagRequired("vault")
	_ = cmd.MarkFlagRequired("vault-priv-key")
	util.AddCmd(parent, cmd)
}
