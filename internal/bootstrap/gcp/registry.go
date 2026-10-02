// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/node"
	"github.com/codesphere-cloud/oms/internal/util"
	"github.com/lithammer/shortuuid"
)

// RegistryType selects which container registry the installation pulls its images from.
type RegistryType string

// The registry types supported by the GCP bootstrapper. RegistryTypeLocalContainer runs a
// local registry, RegistryTypeArtifactRegistry uses a GCP Artifact Registry
// repository, and RegistryTypeGitHub pulls straight from ghcr.io.
const (
	RegistryTypeLocalContainer   RegistryType = "local-container"
	RegistryTypeArtifactRegistry RegistryType = "artifact-registry"
	RegistryTypeGitHub           RegistryType = "github"
)

// jumpboxRegistryAuthFile holds the registry credentials of the jumpbox's root user. OMS and the
// crane library it copies images with read this Docker config first and never merge it with
// podman's own auth file, so every login on the jumpbox is pointed here explicitly.
const jumpboxRegistryAuthFile = "/root/.docker/config.json"

// validateGitHubParams checks if the GitHub credentials are fully specified if GitHub registry is selected
func (b *GCPBootstrapper) validateGitHubParams() error {
	if b.Env.GitHubTeamSlug != "" && b.Env.GitHubTeamOrg != "" && b.Env.GitHubPAT == "" {
		return fmt.Errorf("GitHub PAT is required to extract public keys of GitHub team members")
	}

	ghTeamParams := []string{b.Env.GitHubTeamSlug, b.Env.GitHubTeamOrg}
	if slices.Contains(ghTeamParams, "") && strings.Join(ghTeamParams, "") != "" {
		return fmt.Errorf("GitHub team parameters are not fully specified (all or none of GitHubTeamSlug, GitHubTeamOrg must be set)")
	}

	ghAppParams := []string{b.Env.GitHubAppName, b.Env.GitHubAppClientID, b.Env.GitHubAppClientSecret}
	if slices.Contains(ghAppParams, "") && strings.Join(ghAppParams, "") != "" {
		return fmt.Errorf("GitHub app credentials are not fully specified (all or none of GitHubAppName, GitHubAppClientID, GitHubAppClientSecret must be set)")
	}

	return nil
}

// validateRegistryParams checks that the registry type is supported and that the credentials
// the selected type requires are set.
func (b *GCPBootstrapper) validateRegistryParams() error {
	switch b.Env.RegistryType {
	case RegistryTypeLocalContainer, RegistryTypeArtifactRegistry:
		return nil
	case RegistryTypeGitHub:
		if b.Env.GitHubPAT == "" {
			return fmt.Errorf("github-pat must be set when using GitHub registry type")
		}

		if b.Env.RegistryUser == "" {
			return fmt.Errorf("registry-user must be set when using GitHub registry type")
		}

		return nil
	default:
		return fmt.Errorf("unsupported registry type %q (supported: local-container, artifact-registry, github)", b.Env.RegistryType)
	}
}

// EnsureArtifactRegistry ensures the project's GCP Artifact Registry repository exists and
// resolves it as the registry all data centers pull their images from.
func (b *GCPBootstrapper) EnsureArtifactRegistry() error {
	repoName := "codesphere-registry"

	repo, err := b.GCPClient.GetArtifactRegistry(b.Env.ProjectID, b.Env.Region, repoName)
	if err == nil && repo != nil {
		b.Env.ContainerRegistryURL = repo.GetRegistryUri()
		return nil
	}

	repo, err = b.GCPClient.CreateArtifactRegistry(b.Env.ProjectID, b.Env.Region, repoName)
	if err != nil || repo == nil {
		return fmt.Errorf("failed to create artifact registry: %w, repo: %v", err, repo)
	}

	b.Env.ContainerRegistryURL = repo.GetRegistryUri()

	return nil
}

// EnsureLocalContainerRegistry runs a container registry on the jumpbox for installations whose
// clusters cannot pull from a public registry, such as air-gapped ones, and makes every node that
// pulls from it trust its certificate.
func (b *GCPBootstrapper) EnsureLocalContainerRegistry() error {
	registryNode, err := b.registryNode()
	if err != nil {
		return err
	}

	if err := b.ensureDataCenters(); err != nil {
		return err
	}

	registryServer, created, err := b.ensureRegistryRunning(registryNode)
	if err != nil {
		return err
	}

	b.Env.ContainerRegistryURL = registryServer

	err = b.ensureJumpboxRegistryAccess(registryNode, registryServer, b.Env.RegistryUsername, b.Env.RegistryPassword)
	if err != nil {
		return err
	}

	nodes := b.registryClientNodes()
	// A running registry keeps its certificate, so on a re-run only the nodes that lack it, such
	// as those of an added data center, get it: distributing restarts Docker, and with it Postgres
	// and the Ceph daemons.
	if !created {
		nodes = nodesWithoutRegistryCert(nodes)
	}

	return b.distributeRegistryCert(registryNode, nodes)
}

// registryCertPath is where a node keeps the local registry's certificate among its trusted CAs.
const registryCertPath = "/usr/local/share/ca-certificates/registry.crt"

// registryClientNodes returns every node that pulls images from the local registry: the cluster
// nodes of all data centers and the shared Postgres node, which runs Postgres from a registry image.
func (b *GCPBootstrapper) registryClientNodes() []*node.Node {
	nodes := b.clusterNodes()
	if b.Env.PostgreSQLNode != nil {
		nodes = append(nodes, b.Env.PostgreSQLNode)
	}

	return nodes
}

// nodesWithoutRegistryCert returns the nodes that do not have the registry certificate yet.
func nodesWithoutRegistryCert(nodes []*node.Node) []*node.Node {
	missing := []*node.Node{}

	for _, n := range nodes {
		if !n.NodeClient.HasFile(n, registryCertPath) {
			missing = append(missing, n)
		}
	}

	return missing
}

// registryNode returns the jumpbox the local container registry runs on. It is shared by all
// data centers, so a single registry serves every cluster.
func (b *GCPBootstrapper) registryNode() (*node.Node, error) {
	registryNode := b.Env.Jumpbox
	if registryNode == nil {
		return nil, fmt.Errorf("jumpbox not found in bootstrap environment")
	}

	if registryNode.GetInternalIP() == "" {
		return nil, fmt.Errorf("jumpbox has no internal IP")
	}

	return registryNode, nil
}

// ensureRegistryRunning starts the container registry on the registry node and generates its
// credentials when it is not already serving. Returns the registry server address and whether the
// registry, and with it its certificate, was created by this call.
func (b *GCPBootstrapper) ensureRegistryRunning(registryNode *node.Node) (string, bool, error) {
	// The registry serves on the HTTPS port so that its address carries no port number. Helm
	// charts that split an image reference at its first colon to find the tag would otherwise
	// mistake the port separator for it and pull from the registry's host name alone.
	localRegistryServer := registryNode.GetInternalIP()

	// Figure out if registry is already running
	b.stlog.Logf("Checking if local container registry is already running on the jumpbox")

	checkCommand := `test "$(podman ps --filter 'name=registry' --format '{{.Names}}' | wc -l)" -eq "1"`
	err := registryNode.RunSSHCommand("root", checkCommand)
	vault := b.primaryDC().ConfigManager.GetVault()
	registryUsername := ""
	registryPassword := ""

	if s := vault.GetSecret(files.SecretRegistryUsername); s != nil && s.Fields != nil {
		registryUsername = s.Fields.Password
	}

	if s := vault.GetSecret(files.SecretRegistryPassword); s != nil && s.Fields != nil {
		registryPassword = s.Fields.Password
	}

	// After a switch from another registry type the vault still holds that registry's
	// credentials, so a running registry is only reused when the config already points at it.
	if err == nil && b.configuredRegistryServer() == localRegistryServer &&
		registryUsername != "" && registryPassword != "" {
		b.stlog.Logf("Local container registry already running on the jumpbox")
		b.Env.RegistryUsername = registryUsername
		b.Env.RegistryPassword = registryPassword

		return localRegistryServer, false, nil
	}

	registryUsername = "custom-registry"
	registryPassword = shortuuid.New()
	b.Env.RegistryUsername = registryUsername
	b.Env.RegistryPassword = registryPassword

	commands := []string{
		"apt-get update",
		"apt-get install -y podman apache2-utils",
		"htpasswd -bBc /root/registry.password " + registryUsername + " " + registryPassword,
		"openssl req -newkey rsa:4096 -nodes -sha256 -keyout /root/registry.key -x509 -days 365 -out /root/registry.crt -subj \"/C=DE/ST=BW/L=Karlsruhe/O=Codesphere/CN=" + registryNode.GetInternalIP() + "\" -addext \"subjectAltName = DNS:" + registryNode.GetName() + ",IP:" + registryNode.GetInternalIP() + "\"",
		"podman rm -f registry || true",
		`podman run -d \
		--restart=always --name registry --net=host\
		--env REGISTRY_HTTP_ADDR=0.0.0.0:443 \
		--env REGISTRY_AUTH=htpasswd \
		--env REGISTRY_AUTH_HTPASSWD_REALM='Registry Realm' \
		--env REGISTRY_AUTH_HTPASSWD_PATH=/auth/registry.password \
		-v /root/registry.password:/auth/registry.password \
		--env REGISTRY_HTTP_TLS_CERTIFICATE=/certs/registry.crt \
		--env REGISTRY_HTTP_TLS_KEY=/certs/registry.key \
		-v /root/registry.crt:/certs/registry.crt \
		-v /root/registry.key:/certs/registry.key \
		registry:3`,
		`mkdir -p /etc/docker/certs.d/` + localRegistryServer,
		`cp /root/registry.crt /etc/docker/certs.d/` + localRegistryServer + `/ca.crt`,
	}
	for _, cmd := range commands {
		b.stlog.Logf("Running command on the jumpbox: %s", util.Truncate(cmd, 12))

		err := registryNode.RunSSHCommand("root", cmd)
		if err != nil {
			return "", false, fmt.Errorf("failed to run command on the jumpbox: %w", err)
		}
	}

	return localRegistryServer, true, nil
}

// configuredRegistryServer returns the registry server the primary data center's config points
// at, or an empty string if it has none yet.
func (b *GCPBootstrapper) configuredRegistryServer() string {
	config := b.primaryDC().InstallConfig
	if config == nil || config.Registry == nil {
		return ""
	}

	return config.Registry.Server
}

// distributeRegistryCert installs the local registry's self-signed certificate on the given
// nodes. It is idempotent, so it is safe to re-run for an additional data center.
func (b *GCPBootstrapper) distributeRegistryCert(registryNode *node.Node, nodes []*node.Node) error {
	for _, node := range nodes {
		b.stlog.Logf("Configuring node '%s' to trust local registry certificate", node.GetName())

		err := registryNode.RunSSHCommand("root", "scp -o StrictHostKeyChecking=no /root/registry.crt root@"+node.GetInternalIP()+":"+registryCertPath)
		if err != nil {
			return fmt.Errorf("failed to copy registry certificate to node %s: %w", node.GetInternalIP(), err)
		}

		err = node.RunSSHCommand("root", "update-ca-certificates")
		if err != nil {
			return fmt.Errorf("failed to update CA certificates on node %s: %w", node.GetInternalIP(), err)
		}

		err = node.RunSSHCommand("root", "systemctl restart docker.service || true") // docker is probably not yet installed
		if err != nil {
			return fmt.Errorf("failed to restart docker service on node %s: %w", node.GetInternalIP(), err)
		}
	}

	return nil
}

// ensureJumpboxRegistryAccess lets the jumpbox itself talk to the registries a local container
// registry setup involves. Go tools such as OMS and the crane library it copies images with read
// the system trust store and the Docker config instead of the podman locations the registry
// installation writes to, so the registry CA and the credentials are installed where they can
// find them. The upstream login is what allows 'oms copy package' to mirror the Codesphere
// images into the local registry.
func (b *GCPBootstrapper) ensureJumpboxRegistryAccess(registryNode *node.Node, server, username, password string) error {
	commands := []string{
		"cp /root/registry.crt " + registryCertPath,
		"update-ca-certificates",
		"mkdir -p " + path.Dir(jumpboxRegistryAuthFile),
		// A freshly started registry container is not serving yet when podman returns, so wait
		// for it to answer before logging in.
		fmt.Sprintf("timeout 120 sh -c 'until curl -s -o /dev/null https://%s/v2/; do sleep 2; done'", server),
		registryLoginCommand(server, username, password),
	}

	if b.Env.RegistryUser != "" && b.Env.GitHubPAT != "" {
		commands = append(commands, registryLoginCommand("ghcr.io", b.Env.RegistryUser, b.Env.GitHubPAT))
	} else {
		b.stlog.Logf("Skipping ghcr.io login on the jumpbox, set --registry-user and --github-pat to mirror the Codesphere images into the local registry")
	}

	for _, cmd := range commands {
		b.stlog.Logf("Running command on the jumpbox: %s", util.Truncate(cmd, 12))

		err := registryNode.RunSSHCommand("root", cmd)
		if err != nil {
			return fmt.Errorf("failed to configure registry access on the jumpbox: %w", err)
		}
	}

	return nil
}

// registryLoginCommand stores a registry's credentials where OMS looks for them. The password is
// piped in from the shell built-in printf instead of being passed as an argument, which keeps it
// out of podman's own process arguments, though the shell running the command still carries it.
func registryLoginCommand(server, username, password string) string {
	return fmt.Sprintf("printf '%%s' '%s' | podman login --authfile %s --username '%s' --password-stdin %s",
		password, jumpboxRegistryAuthFile, username, server)
}

// EnsureGitHubAccessConfigured resolves ghcr.io as the registry all data centers pull from. The
// credentials are written into every data center's vault by updateInstallConfig.
func (b *GCPBootstrapper) EnsureGitHubAccessConfigured() error {
	if b.Env.GitHubPAT == "" {
		return fmt.Errorf("GitHub PAT is not set")
	}

	b.Env.ContainerRegistryURL = "ghcr.io"
	b.Env.RegistryUsername = b.Env.RegistryUser
	b.Env.RegistryPassword = b.Env.GitHubPAT

	return nil
}
