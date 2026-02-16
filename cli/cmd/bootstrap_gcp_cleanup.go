// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

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
}

type CleanupDeps struct {
	GCPClient     gcp.GCPClientManager
	FileIO        util.FileIO
	StepLogger    *bootstrap.StepLogger
	ConfirmReader io.Reader
	InfraFilePath string
}

func (c *BootstrapGcpCleanupCmd) RunE(_ *cobra.Command, args []string) error {
	ctx := c.cmd.Context()
	stlog := bootstrap.NewStepLogger(false)
	gcpClient := gcp.NewGCPClient(ctx, stlog, os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"))
	fw := util.NewFilesystemWriter()

	deps := &CleanupDeps{
		GCPClient:     gcpClient,
		FileIO:        fw,
		StepLogger:    stlog,
		ConfirmReader: os.Stdin,
		InfraFilePath: gcp.GetInfraFilePath(),
	}

	return c.ExecuteCleanup(deps)
}

func (c *BootstrapGcpCleanupCmd) loadInfraFile(deps *CleanupDeps) (gcp.CodesphereEnvironment, error) {
	if !deps.FileIO.Exists(deps.InfraFilePath) {
		return gcp.CodesphereEnvironment{}, nil
	}

	content, err := deps.FileIO.ReadFile(deps.InfraFilePath)
	if err != nil {
		return gcp.CodesphereEnvironment{}, fmt.Errorf("failed to read infra file: %w", err)
	}

	var env gcp.CodesphereEnvironment
	if err := json.Unmarshal(content, &env); err != nil {
		return gcp.CodesphereEnvironment{}, fmt.Errorf("failed to parse infra file: %w", err)
	}
	return env, nil
}

func (c *BootstrapGcpCleanupCmd) confirmDeletion(deps *CleanupDeps, projectID string) error {
	log.Printf("WARNING: This will permanently delete the GCP project '%s' and all its resources.", projectID)
	log.Printf("This action cannot be undone.\n")
	log.Println("Type the project ID to confirm deletion: ")

	reader := bufio.NewReader(deps.ConfirmReader)
	confirmation, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read confirmation: %w", err)
	}
	if strings.TrimSpace(confirmation) != projectID {
		return fmt.Errorf("confirmation did not match project ID, aborting cleanup")
	}
	return nil
}

// ExecuteCleanup performs the cleanup operation with the provided dependencies.
func (c *BootstrapGcpCleanupCmd) ExecuteCleanup(deps *CleanupDeps) error {
	infraEnv, err := c.loadInfraFile(deps)
	if err != nil && c.Opts.ProjectID == "" {
		return err
	} else if err != nil {
		log.Printf("Warning: %v", err)
		infraEnv = gcp.CodesphereEnvironment{}
	}

	projectID := c.Opts.ProjectID
	useInfraFile := false
	if projectID == "" {
		if infraEnv.ProjectID == "" {
			return fmt.Errorf("no project ID provided and no infra file found at %s", deps.InfraFilePath)
		}
		projectID = infraEnv.ProjectID
		useInfraFile = true
		log.Printf("Using project ID from infra file: %s", projectID)
	} else if infraEnv.ProjectID != "" && infraEnv.ProjectID != projectID {
		log.Printf("Warning: infra file contains project ID '%s' but deleting '%s'; ignoring infra file for DNS cleanup", infraEnv.ProjectID, projectID)
		infraEnv = gcp.CodesphereEnvironment{}
	} else if infraEnv.ProjectID == projectID {
		useInfraFile = true
	}

	// Verify project is OMS-managed
	if c.Opts.Force {
		log.Printf("Skipping OMS-managed verification (--force flag used)")
	} else {
		isOMSManaged, err := deps.GCPClient.IsOMSManagedProject(projectID)
		if err != nil {
			return fmt.Errorf("failed to verify project: %w", err)
		}
		if !isOMSManaged {
			return fmt.Errorf("project %s was not bootstrapped by OMS (missing 'oms-managed' label). Use --force to override this check", projectID)
		}

		if err := c.confirmDeletion(deps, projectID); err != nil {
			return err
		}
	}

	// Clean up DNS records
	if !c.Opts.SkipDNSCleanup && infraEnv.BaseDomain != "" && infraEnv.DNSZoneName != "" {
		dnsProjectID := infraEnv.DNSProjectID
		if dnsProjectID == "" {
			dnsProjectID = projectID
		}
		if err := deps.StepLogger.Step("Cleaning up DNS records", func() error {
			return deps.GCPClient.DeleteDNSRecordSets(dnsProjectID, infraEnv.DNSZoneName, infraEnv.BaseDomain)
		}); err != nil {
			log.Printf("Warning: failed to clean up DNS records: %v", err)
			log.Printf("You may need to manually delete DNS records for %s in project %s", infraEnv.BaseDomain, dnsProjectID)
		}
	} else if !c.Opts.SkipDNSCleanup && infraEnv.BaseDomain == "" {
		log.Printf("Skipping DNS cleanup: no infrastructure information available (provide infra file or use --skip-dns-cleanup)")
	}

	// Delete the project
	if err := deps.StepLogger.Step("Deleting GCP project", func() error {
		return deps.GCPClient.DeleteProject(projectID)
	}); err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}

	// Clean up local infra file
	if useInfraFile && deps.FileIO.Exists(deps.InfraFilePath) {
		if err := os.Remove(deps.InfraFilePath); err != nil {
			log.Printf("Warning: failed to remove local infra file: %v", err)
		} else {
			log.Printf("Removed local infra file: %s", deps.InfraFilePath)
		}
	}

	log.Println("\nGCP project cleanup completed successfully!")
	log.Printf("Project '%s' has been scheduled for deletion.", projectID)
	log.Printf("Note: GCP projects are retained for 30 days before permanent deletion. You can restore the project within this period from the GCP Console.")

	return nil
}

func AddBootstrapGcpCleanupCmd(bootstrapGcp *cobra.Command, opts *GlobalOptions) {
	cleanup := BootstrapGcpCleanupCmd{
		cmd: &cobra.Command{
			Use:   "cleanup",
			Short: "Clean up GCP infrastructure created by bootstrap-gcp",
			Long:  csio.Long(`Deletes a GCP project that was previously created using the bootstrap-gcp command.`),
			Example: `  # Clean up using project ID from the local infra file
  oms-cli beta bootstrap-gcp cleanup

  # Clean up a specific project
  oms-cli beta bootstrap-gcp cleanup --project-id my-project-abc123

  # Force cleanup without confirmation (skips OMS-managed check)
  oms-cli beta bootstrap-gcp cleanup --project-id my-project-abc123 --force

  # Skip DNS record cleanup
  oms-cli beta bootstrap-gcp cleanup --skip-dns-cleanup`,
		},
		Opts: &BootstrapGcpCleanupOpts{
			GlobalOptions: opts,
		},
	}

	flags := cleanup.cmd.Flags()
	flags.StringVar(&cleanup.Opts.ProjectID, "project-id", "", "GCP Project ID to delete (optional, will use infra file if not provided)")
	flags.BoolVar(&cleanup.Opts.Force, "force", false, "Skip confirmation prompt and OMS-managed check")
	flags.BoolVar(&cleanup.Opts.SkipDNSCleanup, "skip-dns-cleanup", false, "Skip cleaning up DNS records")

	cleanup.cmd.RunE = cleanup.RunE
	bootstrapGcp.AddCommand(cleanup.cmd)
}
