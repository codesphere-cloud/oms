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

// cleanupExecutor manages state and logic for each cleanup step.
type cleanupExecutor struct {
	opts            *BootstrapGcpCleanupOpts
	deps            *CleanupDeps
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
	exec, err := newCleanupExecutor(c.Opts, deps)
	if err != nil {
		return fmt.Errorf("failed to resolve cleanup configuration: %w", err)
	}

	if err := exec.verifyAndConfirm(); err != nil {
		return err
	}

	if err := exec.cleanupDNSRecords(); err != nil {
		log.Printf("Warning: DNS cleanup failed: %v", err)
		log.Printf("You may need to manually delete DNS records for %s in project %s", exec.baseDomain, exec.dnsProjectID)
	}

	if err := exec.removeDNSIAMBinding(); err != nil {
		log.Printf("Warning: failed to remove cloud-controller IAM binding from DNS project %s: %v", exec.dnsProjectID, err)
		log.Printf("You may need to manually remove the serviceAccount:cloud-controller@%s.iam.gserviceaccount.com binding from project %s", exec.projectID, exec.dnsProjectID)
	}

	if err := exec.deleteProject(); err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}

	exec.removeLocalInfraFile()

	log.Println("\nGCP project cleanup completed successfully!")
	log.Printf("Project '%s' has been scheduled for deletion.", exec.projectID)
	log.Printf("Note: GCP projects are retained for 30 days before permanent deletion. You can restore the project within this period from the GCP Console.")

	return nil
}

// newCleanupExecutor resolves configuration from flags and the infra file,
// returning an executor ready to run the cleanup steps.
func newCleanupExecutor(opts *BootstrapGcpCleanupOpts, deps *CleanupDeps) (*cleanupExecutor, error) {
	exec := &cleanupExecutor{
		opts:      opts,
		deps:      deps,
		projectID: opts.ProjectID,
	}
	if err := exec.loadInfraFileIfNeeded(); err != nil {
		return nil, err
	}
	if err := exec.resolveProjectID(); err != nil {
		return nil, err
	}
	exec.resolveDNSSettings()
	return exec, nil
}

// loadInfraFileIfNeeded loads the infra file when the project ID or DNS project.
func (e *cleanupExecutor) loadInfraFileIfNeeded() error {
	missingDNSProjectID := e.opts.DNSProjectID == ""
	missingDNSInfo := missingDNSProjectID
	if !e.opts.SkipDNSCleanup {
		missingDNSInfo = missingDNSProjectID || e.opts.BaseDomain == "" || e.opts.DNSZoneName == ""
	}
	if e.projectID != "" && !missingDNSInfo {
		return nil
	}

	infraEnv, infraFileExists, err := gcp.LoadInfraFile(e.deps.FileIO, e.deps.InfraFilePath)
	if err != nil {
		if e.projectID == "" {
			return fmt.Errorf("failed to load infra file: %w", err)
		}
		log.Printf("Warning: %v", err)
		return nil
	}

	if infraEnv.ProjectID != "" {
		e.infraEnv = infraEnv
		e.infraFileLoaded = true
		return nil
	}

	if infraFileExists && e.projectID == "" {
		return fmt.Errorf("infra file at %s contains empty project ID", e.deps.InfraFilePath)
	}

	return nil
}

// resolveProjectID determines the project ID from the flag or the infra file
func (e *cleanupExecutor) resolveProjectID() error {
	if e.projectID != "" {
		if e.infraFileLoaded && e.infraEnv.ProjectID != e.projectID {
			log.Printf("Warning: infra file contains project ID '%s' but deleting '%s'; ignoring infra file for DNS cleanup", e.infraEnv.ProjectID, e.projectID)
			e.infraEnv = gcp.CodesphereEnvironment{}
			e.infraFileLoaded = false
		}
		return nil
	}

	if e.infraEnv.ProjectID == "" {
		return fmt.Errorf("no project ID provided and no infra file found at %s", e.deps.InfraFilePath)
	}

	e.projectID = e.infraEnv.ProjectID
	log.Printf("Using project ID from infra file: %s", e.projectID)
	return nil
}

// resolveDNSSettings resolves DNS configuration from flags with infra file fallback.
func (e *cleanupExecutor) resolveDNSSettings() {
	e.baseDomain = e.opts.BaseDomain
	if e.baseDomain == "" {
		e.baseDomain = e.infraEnv.BaseDomain
	}
	e.dnsZoneName = e.opts.DNSZoneName
	if e.dnsZoneName == "" {
		e.dnsZoneName = e.infraEnv.DNSZoneName
	}
	e.dnsProjectID = e.opts.DNSProjectID
	if e.dnsProjectID == "" {
		e.dnsProjectID = e.infraEnv.DNSProjectID
	}
	if e.dnsProjectID == "" {
		e.dnsProjectID = e.projectID
	}
}

// verifyAndConfirm checks that the project is OMS-managed and prompts the user
// for deletion confirmation, unless --force is set.
func (e *cleanupExecutor) verifyAndConfirm() error {
	if e.opts.Force {
		log.Printf("Skipping OMS-managed verification and deletion confirmation (--force flag used)")
		return nil
	}

	isOMSManaged, err := e.deps.GCPClient.IsOMSManagedProject(e.projectID)
	if err != nil {
		return fmt.Errorf("failed to verify project: %w", err)
	}
	if !isOMSManaged {
		return fmt.Errorf("project %s was not bootstrapped by OMS (missing 'oms-managed' label). Use --force to override this check", e.projectID)
	}

	return e.confirmDeletion()
}

func (e *cleanupExecutor) confirmDeletion() error {
	log.Printf("WARNING: This will permanently delete the GCP project '%s' and all its resources.", e.projectID)
	log.Printf("This action cannot be undone.\n")
	log.Println("Type the project ID to confirm deletion: ")

	reader := bufio.NewReader(e.deps.ConfirmReader)
	confirmation, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read confirmation: %w", err)
	}
	if strings.TrimSpace(confirmation) != e.projectID {
		return fmt.Errorf("confirmation did not match project ID, aborting cleanup")
	}
	return nil
}

// cleanupDNSRecords deletes OMS-created DNS records if DNS cleanup is enabled
// and the required DNS information is available.
func (e *cleanupExecutor) cleanupDNSRecords() error {
	if e.opts.SkipDNSCleanup {
		return nil
	}
	if e.baseDomain == "" || e.dnsZoneName == "" {
		log.Printf("Skipping DNS cleanup: missing base domain or DNS zone name (provide --base-domain/--dns-zone-name or use --skip-dns-cleanup)")
		return nil
	}
	return e.deps.StepLogger.Step("Clean up DNS records", func() error {
		return e.deps.GCPClient.DeleteDNSRecordSets(e.dnsProjectID, e.dnsZoneName, e.baseDomain)
	})
}

// removeDNSIAMBinding removes the cloud-controller service account's IAM binding
// from the DNS project. This is independent of --skip-dns-cleanup.
func (e *cleanupExecutor) removeDNSIAMBinding() error {
	if e.dnsProjectID == "" || e.dnsProjectID == e.projectID {
		return nil
	}
	return e.deps.StepLogger.Step("Remove DNS service account IAM binding", func() error {
		return e.deps.GCPClient.RemoveIAMRoleBinding(e.dnsProjectID, "cloud-controller", e.projectID, []string{"roles/dns.admin"})
	})
}

// deleteProject deletes the GCP project.
func (e *cleanupExecutor) deleteProject() error {
	return e.deps.StepLogger.Step("Delete GCP project", func() error {
		return e.deps.GCPClient.DeleteProject(e.projectID)
	})
}

// removeLocalInfraFile removes the local infra file if it matches the deleted project.
func (e *cleanupExecutor) removeLocalInfraFile() {
	if !e.infraFileLoaded || e.infraEnv.ProjectID != e.projectID {
		return
	}
	if err := e.deps.FileIO.Remove(e.deps.InfraFilePath); err != nil {
		log.Printf("Warning: failed to remove local infra file: %v", err)
	} else {
		log.Printf("Removed local infra file: %s", e.deps.InfraFilePath)
	}
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
