// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bufio"
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
	DNSProjectID   string
}

type CleanupDeps struct {
	GCPClient     gcp.GCPClientManager
	FileIO        util.FileIO
	StepLogger    *bootstrap.StepLogger
	ConfirmReader io.Reader
	InfraFilePath string
}

// cleanupState holds intermediate state shared across cleanup steps.
type cleanupState struct {
	projectID       string
	infraEnv        gcp.CodesphereEnvironment
	infraFileLoaded bool
	baseDomain      string
	dnsZoneName     string
	dnsProjectID    string
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

// ExecuteCleanup performs the cleanup operation with the provided dependencies.
func (c *BootstrapGcpCleanupCmd) ExecuteCleanup(deps *CleanupDeps) error {
	state, err := c.resolveCleanupConfig(deps)
	if err != nil {
		return fmt.Errorf("failed to resolve cleanup configuration: %w", err)
	}

	err = c.verifyAndConfirm(deps, state)
	if err != nil {
		return err
	}

	if !c.Opts.SkipDNSCleanup && state.baseDomain != "" && state.dnsZoneName != "" {
		err = deps.StepLogger.Step("Clean up DNS records", func() error {
			return deps.GCPClient.DeleteDNSRecordSets(state.dnsProjectID, state.dnsZoneName, state.baseDomain)
		})
		if err != nil {
			log.Printf("Warning: DNS cleanup failed: %v", err)
			log.Printf("You may need to manually delete DNS records for %s in project %s", state.baseDomain, state.dnsProjectID)
		}
	} else if !c.Opts.SkipDNSCleanup {
		log.Printf("Skipping DNS cleanup: missing base domain or DNS zone name (provide --base-domain/--dns-zone-name or use --skip-dns-cleanup)")
	}

	err = deps.StepLogger.Step("Delete GCP project", func() error {
		return deps.GCPClient.DeleteProject(state.projectID)
	})
	if err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}

	c.removeLocalInfraFile(deps, state)

	log.Println("\nGCP project cleanup completed successfully!")
	log.Printf("Project '%s' has been scheduled for deletion.", state.projectID)
	log.Printf("Note: GCP projects are retained for 30 days before permanent deletion. You can restore the project within this period from the GCP Console.")

	return nil
}

// resolveCleanupConfig loads the infra file (if needed) and determines the project ID
// and DNS settings from flags and/or the infra file.
func (c *BootstrapGcpCleanupCmd) resolveCleanupConfig(deps *CleanupDeps) (*cleanupState, error) {
	state := &cleanupState{
		projectID: c.Opts.ProjectID,
	}

	// Only load infra file if we need information from it (project ID or DNS info)
	missingDNSInfo := c.Opts.BaseDomain == "" || c.Opts.DNSZoneName == "" || c.Opts.DNSProjectID == ""
	needsInfraFile := state.projectID == "" || (!c.Opts.SkipDNSCleanup && missingDNSInfo)
	if needsInfraFile {
		infraEnv, infraFileExists, err := gcp.LoadInfraFile(deps.FileIO, deps.InfraFilePath)
		if err != nil {
			if state.projectID == "" {
				return nil, fmt.Errorf("failed to load infra file: %w", err)
			}
			log.Printf("Warning: %v", err)
		} else if infraEnv.ProjectID != "" {
			state.infraEnv = infraEnv
			state.infraFileLoaded = true
		} else if infraFileExists {
			if state.projectID == "" {
				return nil, fmt.Errorf("infra file at %s contains empty project ID", deps.InfraFilePath)
			}
		}
	}

	// Determine project ID
	if state.projectID == "" {
		if state.infraEnv.ProjectID == "" {
			return nil, fmt.Errorf("no project ID provided and no infra file found at %s", deps.InfraFilePath)
		}
		state.projectID = state.infraEnv.ProjectID
		log.Printf("Using project ID from infra file: %s", state.projectID)
	} else if state.infraFileLoaded && state.infraEnv.ProjectID != state.projectID {
		log.Printf("Warning: infra file contains project ID '%s' but deleting '%s'; ignoring infra file for DNS cleanup", state.infraEnv.ProjectID, state.projectID)
		state.infraEnv = gcp.CodesphereEnvironment{}
		state.infraFileLoaded = false
	}

	// Resolve DNS settings from flags with infra file fallback
	state.baseDomain = c.Opts.BaseDomain
	if state.baseDomain == "" {
		state.baseDomain = state.infraEnv.BaseDomain
	}
	state.dnsZoneName = c.Opts.DNSZoneName
	if state.dnsZoneName == "" {
		state.dnsZoneName = state.infraEnv.DNSZoneName
	}
	state.dnsProjectID = c.Opts.DNSProjectID
	if state.dnsProjectID == "" {
		state.dnsProjectID = state.infraEnv.DNSProjectID
	}
	if state.dnsProjectID == "" {
		state.dnsProjectID = state.projectID
	}

	return state, nil
}

// verifyAndConfirm checks that the project is OMS-managed and prompts the user
// for deletion confirmation, unless --force is set.
func (c *BootstrapGcpCleanupCmd) verifyAndConfirm(deps *CleanupDeps, state *cleanupState) error {
	if c.Opts.Force {
		log.Printf("Skipping OMS-managed verification and deletion confirmation (--force flag used)")
		return nil
	}

	isOMSManaged, err := deps.GCPClient.IsOMSManagedProject(state.projectID)
	if err != nil {
		return fmt.Errorf("failed to verify project: %w", err)
	}
	if !isOMSManaged {
		return fmt.Errorf("project %s was not bootstrapped by OMS (missing 'oms-managed' label). Use --force to override this check", state.projectID)
	}

	return c.confirmDeletion(deps, state.projectID)
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

// removeLocalInfraFile removes the local infra file if it matches the deleted project.
func (c *BootstrapGcpCleanupCmd) removeLocalInfraFile(deps *CleanupDeps, state *cleanupState) {
	if !state.infraFileLoaded || state.infraEnv.ProjectID != state.projectID {
		return
	}
	if err := deps.FileIO.Remove(deps.InfraFilePath); err != nil {
		log.Printf("Warning: failed to remove local infra file: %v", err)
	} else {
		log.Printf("Removed local infra file: %s", deps.InfraFilePath)
	}
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
  oms-cli beta bootstrap-gcp cleanup --project-id my-project --base-domain example.com --dns-zone-name my-zone --dns-project-id dns-project`,
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
