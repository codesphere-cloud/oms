// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/codesphere-cloud/oms/internal/bootstrap"
	"github.com/codesphere-cloud/oms/internal/bootstrap/gcp"
	"github.com/codesphere-cloud/oms/internal/bootstrap/local"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/github"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/node"
	"github.com/codesphere-cloud/oms/internal/portal"
	intutil "github.com/codesphere-cloud/oms/internal/util"
	"github.com/spf13/cobra"
)

// BootstrapGcpSingleVMCmd holds the options for the single VM bootstrap command.
type BootstrapGcpSingleVMCmd struct {
	cmd              *cobra.Command
	Env              env.Env
	CodesphereEnv    *gcp.CodesphereEnvironment
	RegistryUser     string
	CephDeviceFilter string
	StorageEngine    string
	SSHQuiet         bool
}

// AddBootstrapGcpSingleVMCmd registers the all-in-one test installation separately from
// bootstrap-gcp, which retains its multi-VM layout.
func AddBootstrapGcpSingleVMCmd(parent *cobra.Command, opts *util.GlobalOptions) {
	c := &BootstrapGcpSingleVMCmd{
		Env:              env.NewEnv(),
		CodesphereEnv:    &gcp.CodesphereEnvironment{SpotVMs: true},
		CephDeviceFilter: "^sdb$",
	}
	c.cmd = &cobra.Command{
		Use:   "bootstrap-gcp-single-vm",
		Short: "Bootstrap Codesphere on one GCP Spot VM",
		Long:  "Create a single GCP Spot VM, install k0s, and run the local Codesphere bootstrap on it. For testing only.",
		Args:  cobra.NoArgs,
		RunE:  c.RunE,
	}
	f := c.cmd.Flags()
	f.StringVar(&c.CodesphereEnv.ProjectName, "project-name", "", "Unique GCP project name")
	f.StringVar(&c.CodesphereEnv.ProjectTTL, "project-ttl", "2h", "Project lifetime before cleanup")
	f.StringVar(&c.CodesphereEnv.BillingAccount, "billing-account", "", "GCP billing account ID")
	f.StringVar(&c.CodesphereEnv.FolderID, "folder-id", "", "GCP folder ID")
	f.StringVar(&c.CodesphereEnv.BaseDomain, "base-domain", "", "Base domain (Codesphere uses cs.<base-domain>)")
	f.StringVar(&c.CodesphereEnv.DNSProjectID, "dns-project-id", "", "GCP project containing the Cloud DNS zone (default: bootstrap project)")
	f.StringVar(&c.CodesphereEnv.DNSZoneName, "dns-zone-name", "oms-testing", "Cloud DNS zone name")
	f.StringVar(&c.CodesphereEnv.Region, "region", "europe-west4", "GCP region")
	f.StringVar(&c.CodesphereEnv.Zone, "zone", "europe-west4-a", "GCP zone")
	f.StringVar(&c.CodesphereEnv.SSHPublicKeyPath, "ssh-public-key-path", "~/.ssh/id_rsa.pub", "SSH public key path")
	f.StringVar(&c.CodesphereEnv.SSHPrivateKeyPath, "ssh-private-key-path", "~/.ssh/id_rsa", "SSH private key path")
	f.StringVar(&c.CodesphereEnv.InstallLocal, "install-local", "", "Local installer-lite archive for the Codesphere installation")
	f.StringVar(&c.CodesphereEnv.InstallVersion, "install-version", "", "Codesphere version to download and install")
	f.StringVar(&c.CodesphereEnv.InstallConfigTemplatePath, "install-config-template", "", "Local config.yaml template to use as the base for the single VM install config")
	f.StringArrayVar(&c.CodesphereEnv.InstallConfigs, "config", nil, "Local config.yaml file; repeat to merge in order before VM settings")
	f.StringVar(&c.CodesphereEnv.InstallVaultPath, "vault", "", "SOPS vault for config templating and initial install secrets")
	f.StringVar(&c.CodesphereEnv.InstallVaultPrivKey, "priv-key", "", "Age private key for --vault (or use the age key environment variable)")
	f.StringVar(&c.CodesphereEnv.RemoteOmsBinaryPath, "remote-oms-binary", "", "Local Linux amd64 OMS binary to run on the VM")
	f.StringVar(&c.CodesphereEnv.SingleVMMachineType, "machine-type", "e2-standard-8", "GCP machine type")
	f.Int64Var(&c.CodesphereEnv.RootDiskSize, "root-disk-size", 100, "Boot disk size in GB; an additional 100 GB Ceph disk is created")
	f.StringVar(&c.RegistryUser, "registry-user", "", "GHCR username for the local Codesphere installer")
	f.StringVar(&c.CephDeviceFilter, "ceph-device-filter", "^sdb$", "Rook device name filter for the extra disk")
	f.StringVar(&c.StorageEngine, "storage-engine", local.StorageEngineRookCeph, "Storage engine for the local installer (supported: rook-ceph, local)")
	f.BoolVar(&c.SSHQuiet, "ssh-quiet", false, "Suppress SSH command output")
	util.MarkFlagRequired(c.cmd, "project-name")
	util.MarkFlagRequired(c.cmd, "billing-account")
	util.MarkFlagRequired(c.cmd, "base-domain")
	util.MarkFlagRequired(c.cmd, "remote-oms-binary")
	util.MarkFlagRequired(c.cmd, "registry-user")
	util.AddCmd(parent, c.cmd)
	AddBootstrapGcpCleanupCmd(c.cmd, opts)
	AddBootstrapGcpRestartVMsCmd(c.cmd, opts)
}

// RunE validates the command options and bootstraps Codesphere on one GCP VM.
func (c *BootstrapGcpSingleVMCmd) RunE(_ *cobra.Command, _ []string) error {
	if err := validateLocalStorageEngine(c.StorageEngine); err != nil {
		return err
	}
	if os.Getenv("OMS_REGISTRY_PASSWORD") == "" {
		return fmt.Errorf("OMS_REGISTRY_PASSWORD must be set for the local Codesphere installer")
	}

	stlog := bootstrap.NewStepLogger(false)

	icg, err := installer.NewInstallConfigManager("plain", "")
	if err != nil {
		return fmt.Errorf("failed to initialize config manager: %w", err)
	}

	ctx := c.cmd.Context()

	githubClient, err := github.NewGitHubClient(ctx, "")
	if err != nil {
		return fmt.Errorf("failed to create GitHub client: %w", err)
	}

	c.CodesphereEnv.OmsWorkdir = c.Env.GetOmsWorkdir()
	sshClient := node.NewSSHNodeClient(c.SSHQuiet)
	sshClient.ForwardRegistryPassword = true

	bs, err := gcp.NewGCPBootstrapper(ctx, c.Env, stlog, c.CodesphereEnv, icg,
		gcp.NewGCPClient(ctx, stlog, os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")),
		intutil.NewFilesystemWriter(), sshClient, portal.NewPortalClient(),
		intutil.NewTime(), githubClient)
	if err != nil {
		return fmt.Errorf("failed to create GCP bootstrapper: %w", err)
	}

	if err := bs.BootstrapSingleVM(c.RegistryUser, c.CephDeviceFilter, c.StorageEngine); err != nil {
		return fmt.Errorf("failed to bootstrap single VM: %w", err)
	}

	log.Printf("Codesphere installed on %s (ssh root@%s)", bs.Env.Jumpbox.GetName(), bs.Env.Jumpbox.GetExternalIP())
	log.Printf("Codesphere URL: https://cs.%s", bs.Env.BaseDomain)

	return nil
}
