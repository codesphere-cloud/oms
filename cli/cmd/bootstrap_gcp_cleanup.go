// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"log"
	"os"

	csio "github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/internal/bootstrap"
	"github.com/codesphere-cloud/oms/internal/bootstrap/gcp"
	"github.com/codesphere-cloud/oms/internal/util"
	"github.com/spf13/cobra"
)

type BootstrapGcpCleanupCmd struct {
	cmd  *cobra.Command
	Opts *BootstrapGcpCleanupOpts
}

type BootstrapGcpCleanupOpts struct {
	*GlobalOptions
	ProjectID      string
	Force          bool
	SkipDNSCleanup bool
	BaseDomain     string
	DNSZoneName    string
	DNSProjectID   string
}

func (c *BootstrapGcpCleanupCmd) RunE(_ *cobra.Command, args []string) error {
	ctx := c.cmd.Context()
	stlog := bootstrap.NewStepLogger(false)
	gcpClient := gcp.NewGCPClient(ctx, stlog, os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"))
	fw := util.NewFilesystemWriter()

	deps := &gcp.CleanupDeps{
		GCPClient:     gcpClient,
		FileIO:        fw,
		StepLogger:    stlog,
		ConfirmReader: os.Stdin,
		InfraFilePath: gcp.GetInfraFilePath(),
	}

	return c.ExecuteCleanup(deps)
}

// ExecuteCleanup performs the cleanup operation with the provided dependencies.
func (c *BootstrapGcpCleanupCmd) ExecuteCleanup(deps *gcp.CleanupDeps) error {
	exec, err := gcp.NewCleanupExecutor(&gcp.CleanupOpts{
		ProjectID:      c.Opts.ProjectID,
		Force:          c.Opts.Force,
		SkipDNSCleanup: c.Opts.SkipDNSCleanup,
		BaseDomain:     c.Opts.BaseDomain,
		DNSZoneName:    c.Opts.DNSZoneName,
		DNSProjectID:   c.Opts.DNSProjectID,
	}, deps)
	if err != nil {
		return fmt.Errorf("failed to resolve cleanup configuration: %w", err)
	}

	if err := exec.VerifyAndConfirm(); err != nil {
		return err
	}

	if err := deps.StepLogger.Step("Clean up DNS records", exec.CleanupDNSRecords); err != nil {
		log.Printf("Warning: DNS cleanup failed: %v", err)
		log.Printf("You may need to manually delete DNS records for %s in project %s", exec.BaseDomain, exec.DNSProjectID)
	}

	if err := deps.StepLogger.Step("Remove DNS service account IAM binding", exec.RemoveDNSIAMBinding); err != nil {
		log.Printf("Warning: failed to remove cloud-controller IAM binding from DNS project %s: %v", exec.DNSProjectID, err)
		log.Printf("You may need to manually remove the serviceAccount:cloud-controller@%s.iam.gserviceaccount.com binding from project %s", exec.ProjectID, exec.DNSProjectID)
	}

	if err := deps.StepLogger.Step("Delete GCP project", exec.DeleteProject); err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}

	exec.RemoveLocalInfraFile()

	log.Println("\nGCP project cleanup completed successfully!")
	log.Printf("Project '%s' has been scheduled for deletion.", exec.ProjectID)
	log.Printf("Note: GCP projects are retained for 30 days before permanent deletion. You can restore the project within this period from the GCP Console.")

	return nil
}

func AddBootstrapGcpCleanupCmd(bootstrapGcp *cobra.Command, opts *GlobalOptions) {
	cleanup := BootstrapGcpCleanupCmd{
		cmd: &cobra.Command{
			Use:   "cleanup",
			Short: "Clean up GCP infrastructure created by bootstrap-gcp",
			Long:  csio.Long(`Deletes a GCP project that was previously created using the bootstrap-gcp command.`),
			Example: formatExamples("beta bootstrap-gcp cleanup", []csio.Example{
				{Desc: "Clean up using project ID from the local infra file"},
				{Cmd: "--project-id my-project-abc123", Desc: "Clean up a specific project"},
				{Cmd: "--project-id my-project-abc123 --force", Desc: "Force cleanup without confirmation (skips OMS-managed check)"},
				{Cmd: "--skip-dns-cleanup", Desc: "Skip DNS record cleanup"},
				{Cmd: "--project-id my-project --base-domain example.com --dns-zone-name my-zone --dns-project-id dns-project", Desc: "Clean up with manual DNS settings (when infra file is not available)"},
			}),
		},
		Opts: &BootstrapGcpCleanupOpts{
			GlobalOptions: opts,
		},
	}

	flags := cleanup.cmd.Flags()
	flags.StringVar(&cleanup.Opts.ProjectID, "project-id", "", "GCP Project ID to delete (optional, will use infra file if not provided)")
	flags.BoolVar(&cleanup.Opts.Force, "force", false, "Skip confirmation prompt and OMS-managed check")
	flags.BoolVar(&cleanup.Opts.SkipDNSCleanup, "skip-dns-cleanup", false, "Skip cleaning up DNS records")
	flags.StringVar(&cleanup.Opts.BaseDomain, "base-domain", "", "Base domain for DNS cleanup (optional, will use infra file if not provided)")
	flags.StringVar(&cleanup.Opts.DNSZoneName, "dns-zone-name", "", "DNS zone name for DNS cleanup (optional, will use infra file if not provided)")
	flags.StringVar(&cleanup.Opts.DNSProjectID, "dns-project-id", "", "GCP Project ID for DNS zone (optional, will use infra file if not provided)")

	cleanup.cmd.RunE = cleanup.RunE
	bootstrapGcp.AddCommand(cleanup.cmd)
}
