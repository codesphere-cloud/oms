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
	// dockerInstalledCommand succeeds on a node that already runs the Docker installed by
	// dockerInstallCommands, so a re-run does not fetch and install it again.
	dockerInstalledCommand string
	// dockerInstallCommands install and start Docker on a node, after the host packages. They are
	// empty where the installer provides Docker itself. They reach the internet through the
	// package manager only, so air-gapped nodes fetch Docker through the package proxy as well.
	dockerInstallCommands []string
	// packageProxy points the package manager of air-gapped nodes at the package proxy on the jumpbox.
	packageProxy nodePackageProxy
	// packageRepoFiles are the file patterns of the repository definitions the image ships. The
	// package proxy lets the hosts they point at through.
	packageRepoFiles []string
	// packageProxyDomains are let through the package proxy in addition to the hosts of the
	// image's repositories.
	packageProxyDomains []string
}

// dockerRHELRepoFile is where Docker's RHEL package repository definition is kept on a host.
const dockerRHELRepoFile = "/etc/yum.repos.d/docker-ce.repo"

// dockerRHELRepoHost serves Docker's RHEL package repository.
const dockerRHELRepoHost = "download.docker.com"

// dockerRHELRepo is Docker's stable RHEL repository as Docker's own repository file defines it.
// Docker publishes one repository per major release, so it is pinned to 9 rather than $releasever,
// which carries the minor release on EUS images. It is written on the host rather than downloaded,
// so only dnf reaches the internet, through the package proxy where the node is air-gapped.
var dockerRHELRepo = []string{
	"[docker-ce-stable]",
	"name=Docker CE Stable - $basearch",
	"baseurl=https://" + dockerRHELRepoHost + "/linux/rhel/9/$basearch/stable",
	"enabled=1",
	"gpgcheck=1",
	"gpgkey=https://" + dockerRHELRepoHost + "/linux/rhel/gpg",
}

var hostOSes = map[HostOS]hostOSSettings{
	HostOSUbuntu: {
		sourceImage:               "projects/ubuntu-os-cloud/global/images/family/ubuntu-2204-lts",
		updatePackageIndexCommand: "apt-get update",
		installPackagesCommand:    "apt-get install -y",
		registryPackages:          []string{"podman", "apache2-utils"},
		trustedCADir:              "/usr/local/share/ca-certificates",
		updateCATrustCommand:      "update-ca-certificates",
		packageProxy:              aptPackageProxy{configFile: "/etc/apt/apt.conf.d/99oms-package-proxy"},
		// Ubuntu 22.04 lists its repositories one per line, later releases in deb822 .sources files.
		packageRepoFiles: []string{"/etc/apt/sources.list", "/etc/apt/sources.list.d/*.list", "/etc/apt/sources.list.d/*.sources"},
	},
	HostOSRHEL9: {
		sourceImage:            "projects/rhel-cloud/global/images/family/rhel-9-6-eus",
		installPackagesCommand: "dnf install -y",
		registryPackages:       []string{"podman", "httpd-tools"},
		trustedCADir:           "/etc/pki/ca-trust/source/anchors",
		updateCATrustCommand:   "update-ca-trust",
		hostPackages:           []string{"lvm2", "container-selinux"},
		dockerInstalledCommand: "rpm -q docker-ce >/dev/null 2>&1 && systemctl is-active --quiet docker.service",
		dockerInstallCommands: []string{
			// podman-docker provides /usr/bin/docker and conflicts with docker-ce.
			"if rpm -q podman-docker >/dev/null 2>&1; then dnf remove -y podman-docker; fi",
			"printf '%s\\n' " + shellQuoteAll(dockerRHELRepo) + " > " + dockerRHELRepoFile,
			"dnf install -y docker-ce docker-ce-cli containerd.io",
			"systemctl enable --now docker.service",
		},
		packageProxy:        dnfPackageProxy{configFile: "/etc/dnf/dnf.conf"},
		packageRepoFiles:    []string{"/etc/yum.repos.d/*.repo"},
		packageProxyDomains: []string{dockerRHELRepoHost},
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
