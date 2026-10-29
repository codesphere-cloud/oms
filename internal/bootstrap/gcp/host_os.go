// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"fmt"
	"path"
	"slices"
	"strings"
)

// HostOS selects the operating system every VM of the bootstrap boots.
type HostOS string

// The host operating systems the GCP bootstrapper supports. HostOSRHEL9 boots the RHEL 9.6
// image with Extended Update Support, so the hosts stay on the minor release Codesphere supports.
const (
	HostOSUbuntu HostOS = "ubuntu"
	HostOSRHEL9  HostOS = "rhel-9"
)

// hostOSSettings holds what differs between the host operating systems of the VMs.
type hostOSSettings struct {
	sourceImage string
	// updatePackageIndexCommand refreshes the package index before packages are installed. It is
	// empty where the installation command refreshes the index itself.
	updatePackageIndexCommand string
	// installPackagesCommand installs the packages passed to it as arguments.
	installPackagesCommand string
	// registryPackages provide podman and htpasswd for the local container registry.
	registryPackages []string
	// trustedCADir is where the system trust store picks up additional CA certificates.
	trustedCADir         string
	updateCATrustCommand string
	// hostPackages are installed on every node Codesphere is installed on. Ubuntu images ship
	// what the installer needs; RHEL images lack the LVM tools Ceph creates its OSDs with and
	// the SELinux policy the Docker packages require.
	hostPackages []string
}

var hostOSes = map[HostOS]hostOSSettings{
	HostOSUbuntu: {
		sourceImage:               "projects/ubuntu-os-cloud/global/images/family/ubuntu-2204-lts",
		updatePackageIndexCommand: "apt-get update",
		installPackagesCommand:    "apt-get install -y",
		registryPackages:          []string{"podman", "apache2-utils"},
		trustedCADir:              "/usr/local/share/ca-certificates",
		updateCATrustCommand:      "update-ca-certificates",
	},
	HostOSRHEL9: {
		sourceImage:            "projects/rhel-cloud/global/images/family/rhel-9-6-eus",
		installPackagesCommand: "dnf install -y",
		registryPackages:       []string{"podman", "httpd-tools"},
		trustedCADir:           "/etc/pki/ca-trust/source/anchors",
		updateCATrustCommand:   "update-ca-trust",
		hostPackages:           []string{"lvm2", "container-selinux"},
	},
}

// SupportedHostOSes returns the names of the supported host operating systems, sorted.
func SupportedHostOSes() []string {
	names := make([]string, 0, len(hostOSes))
	for hostOS := range hostOSes {
		names = append(names, string(hostOS))
	}

	slices.Sort(names)

	return names
}

// settings returns what is specific to the host OS. An empty host OS selects Ubuntu, which
// infra files written before the host OS was selectable used.
func (o HostOS) settings() (hostOSSettings, error) {
	if o == "" {
		o = HostOSUbuntu
	}

	settings, ok := hostOSes[o]
	if !ok {
		return hostOSSettings{}, fmt.Errorf("unsupported host OS %q (supported: %s)", o, strings.Join(SupportedHostOSes(), ", "))
	}

	return settings, nil
}

// SourceImage returns the image the VMs boot from.
func (o HostOS) SourceImage() (string, error) {
	settings, err := o.settings()
	if err != nil {
		return "", fmt.Errorf("failed to get the settings to resolve the source image: %w", err)
	}

	return settings.sourceImage, nil
}

// installCommands returns the commands that install the given packages.
func (s hostOSSettings) installCommands(packages []string) []string {
	commands := []string{}
	if s.updatePackageIndexCommand != "" {
		commands = append(commands, s.updatePackageIndexCommand)
	}

	return append(commands, s.installPackagesCommand+" "+strings.Join(packages, " "))
}

// trustedCAPath returns where a CA certificate file of the given name joins the system trust store.
func (s hostOSSettings) trustedCAPath(fileName string) string {
	return path.Join(s.trustedCADir, fileName)
}
