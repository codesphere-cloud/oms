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

type BootstrapGcpRestartVMsCmd struct {
	cmd  *cobra.Command
	Opts *BootstrapGcpRestartVMsOpts
}

type BootstrapGcpRestartVMsOpts struct {
	*GlobalOptions
	ProjectID string
	Zone      string
	Name      string
}

// resolveProjectAndZone returns the project ID and zone from flags or the infra file.
// If both flags are set they are used directly; if neither is set, the infra file is read.
// Providing only one of --project-id / --zone is an error.
func (c *BootstrapGcpRestartVMsCmd) resolveProjectAndZone(fw util.FileIO) (string, string, error) {
	projectID := c.Opts.ProjectID
	zone := c.Opts.Zone

	if (projectID == "") != (zone == "") {
		return "", "", fmt.Errorf("--project-id and --zone must be provided together")
	}
	if projectID != "" {
		return projectID, zone, nil
	}

	infraFilePath := gcp.GetInfraFilePath()
	infraEnv, exists, err := gcp.LoadInfraFile(fw, infraFilePath)
	if err != nil {
		return "", "", fmt.Errorf("failed to load infra file: %w", err)
	}
	if !exists {
		return "", "", fmt.Errorf("infra file not found at %s; use --project-id and --zone flags", infraFilePath)
	}
	if infraEnv.ProjectID == "" || infraEnv.Zone == "" {
		return "", "", fmt.Errorf("infra file is missing project ID or zone; use --project-id and --zone flags")
	}
	return infraEnv.ProjectID, infraEnv.Zone, nil
}

func (c *BootstrapGcpRestartVMsCmd) RunE(_ *cobra.Command, _ []string) error {
	ctx := c.cmd.Context()
	stlog := bootstrap.NewStepLogger(false)
	fw := util.NewFilesystemWriter()

	projectID, zone, err := c.resolveProjectAndZone(fw)
	if err != nil {
		return err
	}

	gcpClient := gcp.NewGCPClient(ctx, stlog, os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"))

	csEnv := &gcp.CodesphereEnvironment{
		ProjectID: projectID,
		Zone:      zone,
	}

	bs, err := gcp.NewGCPBootstrapper(
		ctx,
		nil, stlog, csEnv, nil, gcpClient, fw, nil, nil, util.NewTime(), nil,
	)
	if err != nil {
		return fmt.Errorf("failed to create bootstrapper: %w", err)
	}

	if c.Opts.Name != "" {
		log.Printf("Restarting VM %s in project %s (zone %s)...", c.Opts.Name, projectID, zone)
		if err := bs.RestartVM(c.Opts.Name); err != nil {
			return fmt.Errorf("failed to restart VM: %w", err)
		}
		log.Printf("VM %s restarted successfully.", c.Opts.Name)
	} else {
		log.Printf("Restarting all VMs in project %s (zone %s)...", projectID, zone)
		if err := bs.RestartVMs(); err != nil {
			return fmt.Errorf("failed to restart VMs: %w", err)
		}
		log.Printf("All VMs restarted successfully.")
	}

	return nil
}

func AddBootstrapGcpRestartVMsCmd(bootstrapGcp *cobra.Command, opts *GlobalOptions) {
	restartVMs := BootstrapGcpRestartVMsCmd{
		cmd: &cobra.Command{
			Use:   "restart-vms",
			Short: "Restart stopped or terminated GCP VMs",
			Long: csio.Long(`Restarts GCP compute instances that were stopped or terminated,
				for example after spot VM preemption.
				By default, restarts all VMs defined in the infrastructure.
				Use --name to restart a single VM.
				Project ID and zone are read from the local infra file if available,
				or can be specified via flags.`),
			Example: formatExamples("beta bootstrap-gcp restart-vms", []csio.Example{
				{Desc: "Restart all VMs using project info from the local infra file"},
				{Cmd: "--name jumpbox", Desc: "Restart only the jumpbox VM"},
				{Cmd: "--name k0s-1", Desc: "Restart a specific k0s node"},
				{Cmd: "--project-id my-project --zone us-central1-a", Desc: "Restart all VMs with explicit project and zone"},
				{Cmd: "--project-id my-project --zone us-central1-a --name ceph-1", Desc: "Restart a specific VM with explicit project and zone"},
			}),
		},
		Opts: &BootstrapGcpRestartVMsOpts{
			GlobalOptions: opts,
		},
	}

	flags := restartVMs.cmd.Flags()
	flags.StringVar(&restartVMs.Opts.ProjectID, "project-id", "", "GCP Project ID (optional, will use infra file if not provided)")
	flags.StringVar(&restartVMs.Opts.Zone, "zone", "", "GCP Zone (optional, will use infra file if not provided)")
	flags.StringVar(&restartVMs.Opts.Name, "name", "", "Name of a specific VM to restart (e.g. jumpbox, postgres, ceph-1, k0s-1). Restarts all VMs if not specified.")

	restartVMs.cmd.RunE = restartVMs.RunE
	bootstrapGcp.AddCommand(restartVMs.cmd)
}
