// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// package docker installs docker on a remote host
package docker

import (
	"fmt"
	"log"

	"github.com/codesphere-cloud/oms/internal/installer/node"
)

//mockery:generate: true
type DockerManager interface {
	// IsInstalled checks whether the docker binary is available on the remote host.
	IsInstalled() bool

	// Install installs Docker Engine on the remote host using Docker's official apt repository.
	InstallWithApt() error
}

type dockerManager struct {
	remoteUser string
	remoteNode *node.Node
}

func New(user string, node *node.Node) DockerManager {
	return &dockerManager{
		remoteUser: user,
		remoteNode: node,
	}
}

// IsInstalled checks whether the docker binary is available on the remote host.
func (d *dockerManager) IsInstalled() bool {
	err := d.remoteNode.RunSSHCommand(d.remoteUser, "command -v docker")

	return err == nil
}

// InstallWithApt installs Docker Engine on the remote host using Docker's official apt repository
// see https://docs.docker.com/engine/install/ubuntu/#install-using-the-repository
func (d *dockerManager) InstallWithApt() error {
	log.Println("Installing Docker on remote host via apt...")

	if err := d.removeConflictingPackages(); err != nil {
		return fmt.Errorf("failed to remove conflicting docker packages")
	}

	if err := d.installAptPrerequisites(); err != nil {
		return fmt.Errorf("failed to install docker apt prequisites")
	}

	if err := d.addDockerRepository(); err != nil {
		return fmt.Errorf("failed to add docker apt repository")
	}

	if err := d.installDockerPackages(); err != nil {
		return fmt.Errorf("failed to install docker packages")
	}

	if err := d.startDaemon(); err != nil {
		return fmt.Errorf("failed to start docker daemon")
	}

	return nil
}

// removeConflictingPackages removes any unofficial Docker packages that may
// conflict with the official Docker Engine packages. The list matches what the
// official docs specify.
func (d *dockerManager) removeConflictingPackages() error {
	cmd := "apt-get remove -y docker.io docker-compose docker-compose-v2 docker-doc podman-docker containerd runc 2>/dev/null || true"
	if err := d.remoteNode.RunSSHCommand(d.remoteUser, cmd); err != nil {
		return fmt.Errorf("failed to remove conflicting packages: %w", err)
	}

	return nil
}

// installAptPrerequisites ensures ca-certificates and curl are present;
// these are required before the Docker GPG key and repo can be added.
func (d *dockerManager) installAptPrerequisites() error {
	log.Println("Installing Docker apt prerequisites...")
	for _, cmd := range []string{
		"apt-get update -qq",
		"apt-get install -y -qq ca-certificates curl",
	} {
		if err := d.remoteNode.RunSSHCommand(d.remoteUser, cmd); err != nil {
			return fmt.Errorf("failed to install apt prerequisites (%q): %w", cmd, err)
		}
	}

	return nil
}

// addDockerRepository adds Docker's official GPG key and apt repository,
// exactly as described in the official Ubuntu install docs.
func (d *dockerManager) addDockerRepository() error {
	log.Println("Adding Docker apt repository...")
	dockerAddRepoCmd := fmt.Sprintf(
		"sudo install -m 0755 -d /etc/apt/keyrings && " +
			"sudo curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc && " +
			"sudo chmod a+r /etc/apt/keyrings/docker.asc && " +
			"SUITE=$(. /etc/os-release && echo \"${UBUNTU_CODENAME:-$VERSION_CODENAME}\") && " +
			"ARCH=$(dpkg --print-architecture) && " +
			"sudo tee /etc/apt/sources.list.d/docker.sources > /dev/null <<EOF\n" +
			"Types: deb\n" +
			"URIs: https://download.docker.com/linux/ubuntu\n" +
			"Suites: $SUITE\n" +
			"Components: stable\n" +
			"Architectures: $ARCH\n" +
			"Signed-By: /etc/apt/keyrings/docker.asc\n" +
			"EOF\n",
	)

	cmds := []string{
		dockerAddRepoCmd,
		"apt-get update -qq",
	}

	for _, cmd := range cmds {
		if err := d.remoteNode.RunSSHCommand(d.remoteUser, cmd); err != nil {
			return fmt.Errorf("failed to add Docker apt repository (%q): %w", cmd, err)
		}
	}

	return nil
}

// installDockerPackages installs docker and related packages using apt-get.
func (d *dockerManager) installDockerPackages() error {
	cmd := fmt.Sprintf(
		"apt-get install -y -qq " +
			"docker-ce " +
			"docker-ce-cli " +
			"containerd.io " +
			"docker-buildx-plugin " +
			"docker-compose-plugin",
	)

	if err := d.remoteNode.RunSSHCommand(d.remoteUser, cmd); err != nil {
		return fmt.Errorf("failed to install Docker packages: %w", err)
	}

	return nil
}

// startDaemon starts and enables the Docker daemon via systemctl so it
// survives reboots.
func (d *dockerManager) startDaemon() error {
	for _, cmd := range []string{
		"systemctl start docker",
		"systemctl enable docker",
	} {
		if err := d.remoteNode.RunSSHCommand(d.remoteUser, cmd); err != nil {
			return fmt.Errorf("failed to run %q: %w", cmd, err)
		}
	}
	return nil
}
