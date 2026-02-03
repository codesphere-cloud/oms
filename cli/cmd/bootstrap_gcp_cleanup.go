// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/codesphere-cloud/cs-go/pkg/io"
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
	ProjectID string
	Force     bool
}

func (c *BootstrapGcpCleanupCmd) RunE(_ *cobra.Command, args []string) error {
	ctx := c.cmd.Context()
	stlog := bootstrap.NewStepLogger(false)
	gcpClient := gcp.NewGCPClient(ctx, stlog, os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"))
	fw := util.NewFilesystemWriter()

	projectID := c.Opts.ProjectID

	// If no project ID provided, try to load from infra file
	if projectID == "" {
		infraFilePath := gcp.GetInfraFilePath()
		if fw.Exists(infraFilePath) {
			envFileContent, err := fw.ReadFile(infraFilePath)
			if err != nil {
				return fmt.Errorf("failed to read gcp infra file: %w", err)
			}

			var codesphereEnv gcp.CodesphereEnvironment
			err = json.Unmarshal(envFileContent, &codesphereEnv)
			if err != nil {
				return fmt.Errorf("failed to unmarshal gcp infra file: %w", err)
			}
			projectID = codesphereEnv.ProjectID
			log.Printf("Using project ID from infra file: %s", projectID)
		} else {
			return fmt.Errorf("no project ID provided and no infra file found at %s", infraFilePath)
		}
	}

	// Verify the project was bootstrapped by OMS (skip if --force is used)
	if !c.Opts.Force {
		isOMSManaged, err := gcpClient.IsOMSManagedProject(projectID)
		if err != nil {
			return fmt.Errorf("failed to verify project: %w", err)
		}

		if !isOMSManaged {
			return fmt.Errorf("project %s was not bootstrapped by OMS (missing 'oms-managed' label). Use --force to override this check", projectID)
		}
	} else {
		log.Printf("Skipping OMS-managed verification (--force flag used)")
	}

	// Confirm deletion unless force flag is set
	if !c.Opts.Force {
		log.Printf("WARNING: This will permanently delete the GCP project '%s' and all its resources.", projectID)
		log.Printf("This action cannot be undone.\n")

		log.Println("Type the project ID to confirm deletion: ")
		reader := bufio.NewReader(os.Stdin)
		confirmation, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read confirmation: %w", err)
		}

		confirmation = strings.TrimSpace(confirmation)
		if confirmation != projectID {
			return fmt.Errorf("confirmation did not match project ID, aborting cleanup")
		}
	}

	// Delete the project
	err := stlog.Step("Deleting GCP project", func() error {
		return gcpClient.DeleteProject(projectID)
	})
	if err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}

	// Remove the local infra file if it exists
	infraFilePath := gcp.GetInfraFilePath()
	if fw.Exists(infraFilePath) {
		err = os.Remove(infraFilePath)
		if err != nil {
			log.Printf("Warning: failed to remove local infra file: %v", err)
		} else {
			log.Printf("Removed local infra file: %s", infraFilePath)
		}
	}

	log.Println("\n🗑️  GCP project cleanup completed successfully!")
	log.Printf("Project '%s' has been scheduled for deletion.", projectID)
	log.Printf("Note: GCP projects are retained for 30 days before permanent deletion. You can restore the project within this period from the GCP Console.")

	return nil
}

func AddBootstrapGcpCleanupCmd(bootstrapGcp *cobra.Command, opts *GlobalOptions) {
	cleanup := BootstrapGcpCleanupCmd{
		cmd: &cobra.Command{
			Use:   "cleanup",
			Short: "Clean up GCP infrastructure created by bootstrap-gcp",
			Long: io.Long(`Deletes a GCP project that was previously created using the bootstrap-gcp command.
				
				This command will:
				* Verify that the project was created by OMS (via the 'oms-managed' label)
				* Request confirmation before proceeding (unless --force is used)
				* Delete the GCP project and all its resources
				* Remove the local infrastructure file
				
				CAUTION: This operation is destructive and cannot be easily undone.
				GCP retains deleted projects for 30 days, during which they can be restored.`),
			Example: `  # Clean up using project ID from the local infra file
  oms beta bootstrap-gcp cleanup

  # Clean up a specific project
  oms beta bootstrap-gcp cleanup --project-id my-project-abc123

  # Force cleanup without confirmation
  oms beta bootstrap-gcp cleanup --project-id my-project-abc123 --force`,
		},
		Opts: &BootstrapGcpCleanupOpts{
			GlobalOptions: opts,
		},
	}

	flags := cleanup.cmd.Flags()
	flags.StringVar(&cleanup.Opts.ProjectID, "project-id", "", "GCP Project ID to delete (optional, will use infra file if not provided)")
	flags.BoolVar(&cleanup.Opts.Force, "force", false, "Skip confirmation prompt and OMS-managed check")

	cleanup.cmd.RunE = cleanup.RunE
	bootstrapGcp.AddCommand(cleanup.cmd)
}
