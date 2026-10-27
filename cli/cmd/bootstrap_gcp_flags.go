// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

func (c *BootstrapGcpCmd) addBootstrapConfigFlags() {
	f := c.cmd.Flags()
	f.StringVar(&c.CodesphereEnv.InstallConfigTemplatePath, "install-config-template", "", "Initial config.yaml template, applied before --config files")
	f.StringArrayVar(&c.CodesphereEnv.InstallConfigs, "config", nil, "Local config.yaml file; repeat to merge in order before VM settings")
	f.StringVar(&c.CodesphereEnv.InstallVaultPath, "vault", "", "SOPS vault for config templating and initial install secrets")
	f.StringVar(&c.CodesphereEnv.InstallVaultPrivKey, "priv-key", "", "Age private key for --vault (or use the age key environment variable)")
	f.StringVar(&c.CodesphereEnv.MachineType, "machine-type", "", "GCP machine type override for all VMs (default: role-specific sizes; e2-standard-8 for a single VM)")
}
