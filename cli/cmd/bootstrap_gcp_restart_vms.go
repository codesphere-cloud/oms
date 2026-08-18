// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"log"
	"os"

	csio "github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/codesphere-cloud/oms/internal/bootstrap"
	"github.com/codesphere-cloud/oms/internal/bootstrap/gcp"
	intutil "github.com/codesphere-cloud/oms/internal/util"
	"github.com/spf13/cobra"
)

type BootstrapGcpRestartVMsCmd struct {
	cmd  *cobra.Command
	Opts *BootstrapGcpRestartVMsOpts
}

type BootstrapGcpRestartVMsOpts struct {
	*util.GlobalOptions
	ProjectID string
	Zone      string
	Name      string
}

// resolveEnvironment returns the environment to restart VMs in. Project ID and zone come from
// the flags or, when neither is set, from the infra file. Providing only one of
// --project-id / --zone is an error.
//
// The data center layout always comes from the infra file, since it determines the VM names. It
// is read best-effort when the flags supply project and zone, in which case a missing file just
// means single-data-center names.
func (c *BootstrapGcpRestartVMsCmd) resolveEnvironment(fw intutil.FileIO) (*gcp.CodesphereEnvironment, error) {
	projectID := c.Opts.ProjectID
	zone := c.Opts.Zone

	if (projectID == "") != (zone == "") {
		return nil, fmt.Errorf("--project-id and --zone must be provided together")
	}

	infraFilePath := gcp.GetInfraFilePath()
	infraEnv, exists, err := gcp.LoadInfraFile(fw, infraFilePath)
	if err != nil {
		if projectID == "" {
			return nil, fmt.Errorf("failed to load infra file: %w", err)
		}

		log.Printf("Warning: %v", err)
	}

	if projectID == "" {
		if !exists {
			return nil, fmt.Errorf("infra file not found at %s; use --project-id and --zone flags", infraFilePath)
		}

		if infraEnv.ProjectID == "" || infraEnv.Zone == "" {
			return nil, fmt.Errorf("infra file is missing project ID or zone; use --project-id and --zone flags")
		}

		projectID, zone = infraEnv.ProjectID, infraEnv.Zone
	}

	return &gcp.CodesphereEnvironment{
		ProjectID:   projectID,
		Zone:        zone,
		MultiDC:     infraEnv.MultiDC,
		DataCenters: infraEnv.DataCenters,
	}, nil
}

func (c *BootstrapGcpRestartVMsCmd) RunE(_ *cobra.Command, _ []string) error {
	ctx := c.cmd.Context()
	stlog := bootstrap.NewStepLogger(false)
	fw := intutil.NewFilesystemWriter()

	csEnv, err := c.resolveEnvironment(fw)
	if err != nil {
		return err
	}

	projectID, zone := csEnv.ProjectID, csEnv.Zone

	gcpClient := gcp.NewGCPClient(ctx, stlog, os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"))

	bs, err := gcp.NewGCPBootstrapper(
		ctx,
		nil, stlog, csEnv, nil, gcpClient, fw, nil, nil, intutil.NewTime(), nil,
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

func AddBootstrapGcpRestartVMsCmd(bootstrapGcp *cobra.Command, opts *util.GlobalOptions) {
	restartVMs := BootstrapGcpRestartVMsCmd{
		cmd: &cobra.Command{
			Use:   "restart-vms",
			Short: "Restart stopped or terminated GCP VMs",
			Long: csio.Long(`Restarts GCP compute instances that were stopped or terminated,
				for example after spot VM preemption.
				By default, restarts all VMs defined in the infrastructure.
				Use --name to restart a single VM.
				Project ID and zone are read from the local infra file if available`),
			Example: util.FormatExamples("beta bootstrap-gcp restart-vms", []csio.Example{
				{Desc: "Restart all VMs using project info from the local infra file"},
				{Cmd: "--name jumpbox", Desc: "Restart only the jumpbox VM"},
				{Cmd: "--name k0s-1", Desc: "Restart a specific k0s node"},
				{Cmd: "--name k0s-1-dc2", Desc: "Restart a node of the second data center of a --multi-dc bootstrap"},
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
