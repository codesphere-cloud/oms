// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"fmt"
	"path"
	"strings"

	"github.com/codesphere-cloud/oms/internal/installer/node"
)

// The package proxy runs on the jumpbox and lets air-gapped nodes install the OS packages they need
// from the internet. It only passes requests to an allowlist of package repository hosts, so the
// nodes stay cut off from everything else. It runs as a container, so it is the same on every host OS.
const (
	packageProxyContainer = "package-proxy"
	packageProxyImage     = "docker.io/ubuntu/squid:6.6-24.04_beta"
	packageProxyPort      = "3128"
	packageProxyDir       = "/etc/oms/package-proxy"
)

// packageProxyConfig is the squid configuration of the package proxy. It accepts connections from the
// VPC only. HTTPS is tunnelled to port 443 of the allowed hosts, which end-to-end TLS keeps the proxy
// from reading or altering. Plain HTTP is passed to port 80 of the allowed hosts, as the Ubuntu
// archives are served over it and apt verifies their signatures itself.
var packageProxyConfig = []string{
	"http_port " + packageProxyPort,
	"acl vpc src " + vpcSubnetCIDR,
	"acl package_repos dstdomain \"/etc/squid/allowed-domains\"",
	"acl https_port port 443",
	"acl http_port port 80",
	"acl CONNECT method CONNECT",
	"http_access allow vpc CONNECT https_port package_repos",
	"http_access allow vpc !CONNECT http_port package_repos",
	"http_access deny all",
}

// nodePackageProxy points the package manager of a node at the package proxy, or back at the
// repositories themselves.
type nodePackageProxy interface {
	// configureCommand makes the package manager use the proxy at proxyURL. It replaces a proxy
	// configured before, and fails if the setting is not in place afterwards.
	configureCommand(proxyURL string) string
	// removeCommand makes the package manager reach the repositories directly again.
	removeCommand() string
}

// dnfPackageProxy sets the proxy in dnf's main configuration, which has no drop-in directory.
type dnfPackageProxy struct {
	configFile string
}

func (p dnfPackageProxy) configureCommand(proxyURL string) string {
	// The setting goes right below the [main] header rather than at the end of the file, where it
	// would join the last line of a file without a final newline or land in a later section.
	return p.removeCommand() +
		" && sed -i " + shellQuote(`/^\[main\]/a proxy=`+proxyURL) + " " + p.configFile +
		" && grep -qx " + shellQuote("proxy="+proxyURL) + " " + p.configFile
}

func (p dnfPackageProxy) removeCommand() string {
	return "sed -i '/^proxy=/d' " + p.configFile
}

// aptPackageProxy keeps the proxy in a drop-in file of its own, so the image's apt configuration is
// left alone.
type aptPackageProxy struct {
	configFile string
}

func (p aptPackageProxy) configureCommand(proxyURL string) string {
	return "printf '%s\\n' " + shellQuoteAll([]string{
		`Acquire::http::Proxy "` + proxyURL + `";`,
		`Acquire::https::Proxy "` + proxyURL + `";`,
	}) + " > " + p.configFile
}

func (p aptPackageProxy) removeCommand() string {
	return "rm -f " + p.configFile
}

// packageProxyURL returns the address of the package proxy on the jumpbox.
func packageProxyURL(jumpbox *node.Node) string {
	return "http://" + jumpbox.GetInternalIP() + ":" + packageProxyPort
}

// needsPackageProxy reports whether the nodes install their packages through the package proxy,
// which is the case for air-gapped installations.
func (b *GCPBootstrapper) needsPackageProxy(hostOSSettings hostOSSettings) bool {
	return b.Env.Airgapped && hostOSSettings.packageProxy != nil
}

// EnsurePackageProxy runs the package proxy on the jumpbox when the air-gapped nodes need it. The
// jumpbox boots the same image as the nodes, so the repositories it ships are the ones the nodes
// install from, and their hosts are allowed through together with the extra repositories of the
// host OS. The proxy is recreated on every run, so it picks up changed repositories.
func (b *GCPBootstrapper) EnsurePackageProxy() error {
	hostOSSettings, err := b.Env.HostOS.settings()
	if err != nil {
		return fmt.Errorf("failed to get the host OS settings for the package proxy: %w", err)
	}

	if !b.needsPackageProxy(hostOSSettings) {
		return nil
	}

	jumpbox, err := b.registryNode()
	if err != nil {
		return err
	}

	for _, step := range packageProxySteps(hostOSSettings) {
		b.stlog.Logf("Package proxy: %s", step.description)

		err := jumpbox.RunSSHCommand("root", step.command)
		if err != nil {
			return fmt.Errorf("failed to set up the package proxy on the jumpbox while %s: %w", step.description, err)
		}
	}

	return nil
}

// packageProxySteps returns the commands that start the package proxy on the jumpbox.
func packageProxySteps(hostOSSettings hostOSSettings) []jumpboxStep {
	configFile := path.Join(packageProxyDir, "squid.conf")
	domainsFile := path.Join(packageProxyDir, "allowed-domains")

	steps := []jumpboxStep{}
	for _, cmd := range hostOSSettings.installCommands([]string{"podman"}) {
		steps = append(steps, jumpboxStep{"installing podman", cmd})
	}

	return append(steps, []jumpboxStep{
		{"creating the configuration directory", "mkdir -p " + packageProxyDir},
		{"writing the squid configuration", "printf '%s\\n' " + shellQuoteAll(packageProxyConfig) + " > " + configFile},
		{"allowing the package repository hosts", allowedDomainsCommand(hostOSSettings) + " > " + domainsFile},
		{"removing a previous proxy container", "podman rm -f " + packageProxyContainer + " || true"},
		{
			"starting the proxy container",
			// :Z relabels the mounted files for the container, which SELinux would otherwise deny
			// access to. Hosts without SELinux ignore it.
			"podman run -d --restart=always --name " + packageProxyContainer + " --net=host" +
				" -v " + configFile + ":/etc/squid/squid.conf:Z" +
				" -v " + domainsFile + ":/etc/squid/allowed-domains:Z " +
				packageProxyImage,
		},
		// Squid answers a plain request with an error page, which is enough to know it is serving.
		{
			"waiting for the proxy to answer",
			"timeout 60 sh -c 'until curl -s -o /dev/null http://127.0.0.1:" + packageProxyPort + "; do sleep 2; done'",
		},
	}...)
}

// allowedDomainsCommand prints the hosts the package proxy lets through, one per line: the extra
// domains of the host OS and the hosts of the image's repositories.
func allowedDomainsCommand(hostOSSettings hostOSSettings) string {
	cmd := repoHostsCommand(hostOSSettings.packageRepoFiles)
	if len(hostOSSettings.packageProxyDomains) > 0 {
		cmd = "printf '%s\\n' " + shellQuoteAll(hostOSSettings.packageProxyDomains) + "; " + cmd
	}

	return "{ " + cmd + "; } | sort -u"
}

// repoHostsCommand prints the host of every http(s) URL in the uncommented lines of the repository
// definitions, e.g. example.com for "baseurl=https://example.com/repo" or
// "deb http://example.com/ubuntu jammy main". Patterns that match no file are skipped.
func repoHostsCommand(repoFiles []string) string {
	return "grep -hv '^[[:space:]]*#' " + strings.Join(repoFiles, " ") + " 2>/dev/null" +
		" | grep -oE 'https?://[^/:[:space:]]+' | sed -E 's#^https?://##'"
}

// ensureNodePackageProxy points the node's package manager at the package proxy on the jumpbox
// when the installation is air-gapped, and back at the repositories themselves otherwise, so a
// re-run that switches air-gapping off installs directly again.
func (b *GCPBootstrapper) ensureNodePackageProxy(n *node.Node, hostOSSettings hostOSSettings) error {
	if hostOSSettings.packageProxy == nil {
		return nil
	}

	cmd := hostOSSettings.packageProxy.removeCommand()
	if b.needsPackageProxy(hostOSSettings) {
		cmd = hostOSSettings.packageProxy.configureCommand(packageProxyURL(b.Env.Jumpbox))
	}

	err := n.RunSSHCommand("root", cmd)
	if err != nil {
		return fmt.Errorf("failed to configure the package proxy on %s: %w", n.GetName(), err)
	}

	return nil
}
