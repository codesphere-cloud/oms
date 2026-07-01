// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	packageio "github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/util"
	"github.com/spf13/cobra"
)

// InstallPostgresCmd represents the postgres install command.
type InstallPostgresCmd struct {
	cmd        *cobra.Command
	Opts       InstallPostgresOpts
	Env        env.Env
	FileWriter util.FileIO
}

// InstallPostgresOpts holds all CLI flags for the postgres sub-command.
type InstallPostgresOpts struct {
	*GlobalOptions

	// SSH / remote host
	SSHKeyPath string
	SSHUser    string
	SSHHost    string
	SSHPort    int

	// Docker
	DockerVersion string // empty → latest

	// Postgres container
	PostgresVersion  string // Docker image tag, e.g. "16" or "16.3-alpine"
	ContainerName    string
	DataDir          string // host-side bind-mount for pg data
	PostgresPassword string
	PostgresUser     string
	PostgresDB       string
	PublishPort      string // host:container, e.g. "5432:5432"
	RestartPolicy    string // always | unless-stopped | on-failure | no
	ForcePostgres    bool   // stop & recreate the container if it already exists
}

func (c *InstallPostgresCmd) RunE(_ *cobra.Command, _ []string) error {
	// 	// ssh := installer.NewSSHRunner(
	// 	// 	c.Opts.SSHHost,
	// 	// 	c.Opts.SSHPort,
	// 	// 	c.Opts.SSHUser,
	// 	// 	c.Opts.SSHKeyPath,
	// 	// )
	// 	// docker := installer.NewDockerManager(ssh)
	// 	// pg := installer.NewPostgresManager(ssh)

	// 	// return c.InstallPostgres(docker, pg)
	return nil
}

// AddInstallPostgresCmd registers the "install postgres" sub-command under
// the provided parent install command, following the same pattern as
// AddInstallK0sCmd.
func AddInstallPostgresCmd(install *cobra.Command, opts *GlobalOptions) {
	pg := InstallPostgresCmd{
		cmd: &cobra.Command{
			Use:   "postgres",
			Short: "Install PostgreSQL inside Docker on a remote host",
			Long: packageio.Long(`Install Docker (if not already present) and run a
			PostgreSQL container on a remote host accessed via SSH.

			The command will:
			  - Connect to the remote host over SSH
			  - Detect whether Docker is installed; install it if not
			  - Pull the requested PostgreSQL Docker image
			  - Start (or recreate) a named Postgres container with the
			    specified credentials, data directory, and port mapping`),
			Example: formatExamples("install postgres", []packageio.Example{
				{Cmd: "--ssh-host 10.0.0.5 --ssh-key-path ~/.ssh/id_rsa", Desc: "Minimal invocation using defaults"},
				{Cmd: "--postgres-version 16-alpine", Desc: "Use a specific Postgres image tag"},
				{Cmd: "--data-dir /mnt/pgdata --publish-port 15432:5432", Desc: "Custom data directory and host port"},
				{Cmd: "--force", Desc: "Recreate the container and reinstall Docker if present"},
				{Cmd: "--docker-version 26.1.4", Desc: "Pin the Docker engine version"},
			}),
		},
		Opts:       InstallPostgresOpts{GlobalOptions: opts},
		Env:        env.NewEnv(),
		FileWriter: util.NewFilesystemWriter(),
	}

	f := pg.cmd.Flags()

	// SSH flags
	f.StringVar(&pg.Opts.SSHHost, "ssh-host", "", "Remote host IP or hostname (required)")
	f.IntVar(&pg.Opts.SSHPort, "ssh-port", 22, "SSH port on the remote host")
	f.StringVar(&pg.Opts.SSHUser, "ssh-user", "root", "SSH username")
	f.StringVar(&pg.Opts.SSHKeyPath, "ssh-key-path", "", "Path to SSH private key")

	// Docker flags
	f.StringVar(&pg.Opts.DockerVersion, "docker-version", "", "Docker Engine version to install (empty = latest)")
	// f.BoolVar(&pg.Opts.ForceDocker, "force-docker", false, "Reinstall Docker even if already present")

	// Postgres flags
	f.StringVar(&pg.Opts.PostgresVersion, "postgres-version", "16", "PostgreSQL Docker image tag (e.g. 16, 16.3-alpine)")
	f.StringVar(&pg.Opts.ContainerName, "container-name", "postgres", "Docker container name")
	f.StringVar(&pg.Opts.DataDir, "data-dir", "/var/lib/postgresql/data", "Host path for Postgres data bind-mount")
	f.StringVar(&pg.Opts.PostgresPassword, "password", "changeme", "POSTGRES_PASSWORD environment variable")
	f.StringVar(&pg.Opts.PostgresUser, "user", "postgres", "POSTGRES_USER environment variable")
	f.StringVar(&pg.Opts.PostgresDB, "db", "postgres", "POSTGRES_DB environment variable")
	f.StringVar(&pg.Opts.PublishPort, "publish-port", "5432:5432", "Port mapping host:container")
	f.StringVar(&pg.Opts.RestartPolicy, "restart", "unless-stopped", "Container restart policy")
	f.BoolVarP(&pg.Opts.ForcePostgres, "force", "f", false, "Stop and recreate the Postgres container if it exists")

	_ = pg.cmd.MarkFlagRequired("ssh-host")

	AddCmd(install, pg.cmd)
	pg.cmd.RunE = pg.RunE
}

// // ---------------------------------------------------------------------------
// // Orchestration
// // ---------------------------------------------------------------------------

// // InstallPostgres is the top-level orchestrator; it mirrors InstallK0s in
// // structure so it is easy to test with mock implementations.
// func (c *InstallPostgresCmd) InstallPostgres(docker installer.DockerManager, pg installer.PostgresManager) error {
// 	if err := c.ensureDocker(docker); err != nil {
// 		return err
// 	}

// 	if err := c.pullPostgresImage(pg); err != nil {
// 		return err
// 	}

// 	if err := c.startPostgresContainer(pg); err != nil {
// 		return err
// 	}

// 	log.Printf("PostgreSQL is running in container %q on %s:%d",
// 		c.Opts.ContainerName, c.Opts.SSHHost, c.Opts.SSHPort)
// 	log.Printf("Connect with: psql -h %s -p %s -U %s -d %s",
// 		c.Opts.SSHHost, hostPort(c.Opts.PublishPort), c.Opts.PostgresUser, c.Opts.PostgresDB)

// 	return nil
// }

// // ensureDocker checks whether Docker is installed on the remote host and
// // installs it when it is absent (or when --force-docker is given).
// func (c *InstallPostgresCmd) ensureDocker(docker installer.DockerManager) error {
// 	installed, err := docker.IsInstalled()
// 	if err != nil {
// 		return fmt.Errorf("failed to check Docker installation: %w", err)
// 	}

// 	if installed && !c.Opts.ForceDocker {
// 		ver, _ := docker.Version()
// 		log.Printf("Docker already installed on remote host (version: %s), skipping", ver)
// 		return nil
// 	}

// 	log.Println("Installing Docker on remote host...")
// 	if err := docker.Install(c.Opts.DockerVersion); err != nil {
// 		return fmt.Errorf("failed to install Docker: %w", err)
// 	}

// 	ver, _ := docker.Version()
// 	log.Printf("Docker installed successfully (version: %s)", ver)
// 	return nil
// }

// // pullPostgresImage pulls the requested Postgres image on the remote host.
// func (c *InstallPostgresCmd) pullPostgresImage(pg installer.PostgresManager) error {
// 	image := postgresImage(c.Opts.PostgresVersion)
// 	log.Printf("Pulling Docker image %s on remote host...", image)

// 	if err := pg.PullImage(image); err != nil {
// 		return fmt.Errorf("failed to pull Postgres image %s: %w", image, err)
// 	}

// 	log.Printf("Image %s pulled successfully", image)
// 	return nil
// }

// // startPostgresContainer stops any existing container (when --force is set)
// // and starts a fresh one with the configured options.
// func (c *InstallPostgresCmd) startPostgresContainer(pg installer.PostgresManager) error {
// 	exists, running, err := pg.ContainerStatus(c.Opts.ContainerName)
// 	if err != nil {
// 		return fmt.Errorf("failed to inspect container %q: %w", c.Opts.ContainerName, err)
// 	}

// 	if exists {
// 		if !c.Opts.ForcePostgres {
// 			if running {
// 				log.Printf("Container %q is already running; use --force to recreate", c.Opts.ContainerName)
// 				return nil
// 			}
// 			log.Printf("Container %q exists but is stopped; starting it...", c.Opts.ContainerName)
// 			if err := pg.StartContainer(c.Opts.ContainerName); err != nil {
// 				return fmt.Errorf("failed to start existing container %q: %w", c.Opts.ContainerName, err)
// 			}
// 			return nil
// 		}

// 		log.Printf("Removing existing container %q (--force)...", c.Opts.ContainerName)
// 		if err := pg.RemoveContainer(c.Opts.ContainerName); err != nil {
// 			return fmt.Errorf("failed to remove container %q: %w", c.Opts.ContainerName, err)
// 		}
// 	}

// 	cfg := installer.PostgresContainerConfig{
// 		Image:         postgresImage(c.Opts.PostgresVersion),
// 		ContainerName: c.Opts.ContainerName,
// 		DataDir:       c.Opts.DataDir,
// 		Password:      c.Opts.PostgresPassword,
// 		User:          c.Opts.PostgresUser,
// 		DB:            c.Opts.PostgresDB,
// 		PublishPort:   c.Opts.PublishPort,
// 		RestartPolicy: c.Opts.RestartPolicy,
// 	}

// 	log.Printf("Starting Postgres container %q...", c.Opts.ContainerName)
// 	if err := pg.RunContainer(cfg); err != nil {
// 		return fmt.Errorf("failed to start Postgres container: %w", err)
// 	}

// 	return nil
// }

// // ---------------------------------------------------------------------------
// // Helpers
// // ---------------------------------------------------------------------------

// func postgresImage(tag string) string {
// 	return fmt.Sprintf("postgres:%s", tag)
// }

// // hostPort extracts the host-side port from a "host:container" mapping.
// func hostPort(publish string) string {
// 	for i, ch := range publish {
// 		if ch == ':' {
// 			return publish[:i]
// 		}
// 	}
// 	return publish
// }
