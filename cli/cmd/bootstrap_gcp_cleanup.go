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
	BaseDomain     string
	DNSZoneName    string
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

func (c *BootstrapGcpCleanupCmd) loadInfraFile(deps *CleanupDeps) (gcp.CodesphereEnvironment, bool, error) {
	if !deps.FileIO.Exists(deps.InfraFilePath) {
		return gcp.CodesphereEnvironment{}, false, nil
	}

	content, err := deps.FileIO.ReadFile(deps.InfraFilePath)
	if err != nil {
		return gcp.CodesphereEnvironment{}, true, fmt.Errorf("failed to read gcp infra file: %w", err)
	}

	var env gcp.CodesphereEnvironment
	if err := json.Unmarshal(content, &env); err != nil {
		return gcp.CodesphereEnvironment{}, true, fmt.Errorf("failed to unmarshal gcp infra file: %w", err)
	}
	return env, true, nil
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
	projectID := c.Opts.ProjectID
	infraFileLoaded := false
	infraFileExists := false
	var infraEnv gcp.CodesphereEnvironment

	// Only load infra file if we need information from it (project ID or DNS info)
	needsInfraFile := projectID == "" || (!c.Opts.SkipDNSCleanup && c.Opts.BaseDomain == "")
	if needsInfraFile {
		var err error
		infraEnv, infraFileExists, err = c.loadInfraFile(deps)
		if err != nil {
			if projectID == "" {
				return fmt.Errorf("failed to load infra file: %w", err)
			}
			log.Printf("Warning: %v", err)
			infraEnv = gcp.CodesphereEnvironment{}
		} else if infraEnv.ProjectID != "" {
			infraFileLoaded = true
		}
	}

	// Determine project ID to use
	if projectID == "" {
		if infraFileExists && infraEnv.ProjectID == "" {
			return fmt.Errorf("infra file at %s contains empty project ID", deps.InfraFilePath)
		}
		if infraEnv.ProjectID == "" {
			return fmt.Errorf("no project ID provided and no infra file found at %s", deps.InfraFilePath)
		}
		projectID = infraEnv.ProjectID
		log.Printf("Using project ID from infra file: %s", projectID)
	} else if infraFileLoaded && infraEnv.ProjectID != projectID {
		log.Printf("Warning: infra file contains project ID '%s' but deleting '%s'; ignoring infra file for DNS cleanup", infraEnv.ProjectID, projectID)
		infraEnv = gcp.CodesphereEnvironment{}
		infraFileLoaded = false
	}

	// Apply command-line overrides for DNS settings
	baseDomain := c.Opts.BaseDomain
	if baseDomain == "" {
		baseDomain = infraEnv.BaseDomain
	}
	dnsZoneName := c.Opts.DNSZoneName
	if dnsZoneName == "" {
		dnsZoneName = infraEnv.DNSZoneName
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
			return fmt.Errorf("deletion confirmation failed: %w", err)
		}
	}

	// Clean up DNS records
	if !c.Opts.SkipDNSCleanup && baseDomain != "" && dnsZoneName != "" {
		dnsProjectID := infraEnv.DNSProjectID
		if dnsProjectID == "" {
			dnsProjectID = projectID
		}
		if err := deps.StepLogger.Step("Cleaning up DNS records", func() error {
			return deps.GCPClient.DeleteDNSRecordSets(dnsProjectID, dnsZoneName, baseDomain)
		}); err != nil {
			log.Printf("Warning: failed to clean up DNS records: %v", err)
			log.Printf("You may need to manually delete DNS records for %s in project %s", baseDomain, dnsProjectID)
		}
	} else if !c.Opts.SkipDNSCleanup && baseDomain == "" {
		log.Printf("Skipping DNS cleanup: no base domain available (provide --base-domain or infra file, or use --skip-dns-cleanup)")
	}

	// Delete the project
	if err := deps.StepLogger.Step("Deleting GCP project", func() error {
		return deps.GCPClient.DeleteProject(projectID)
	}); err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}

	// Clean up local infra file only if it matches the deleted project
	if infraFileLoaded && infraEnv.ProjectID == projectID {
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
  oms-cli beta bootstrap-gcp cleanup --skip-dns-cleanup

  # Clean up with manual DNS settings (when infra file is not available)
  oms-cli beta bootstrap-gcp cleanup --project-id my-project --base-domain example.com --dns-zone-name my-zone`,
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

	cleanup.cmd.RunE = cleanup.RunE
	bootstrapGcp.AddCommand(cleanup.cmd)
}
