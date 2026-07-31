// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"fmt"
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

// EnsureLocalContainerRegistry installs a container registry on the jumpbox to speed up image
// loading time, and makes every cluster node of every data center trust its certificate.
func (b *GCPBootstrapper) EnsureLocalContainerRegistry() error {
	registryNode, err := b.registryNode()
	if err != nil {
		return err
	}

	if err := b.ensureDataCenters(); err != nil {
		return err
	}

	registryServer, err := b.ensureRegistryRunning(registryNode)
	if err != nil {
		return err
	}

	b.Env.ContainerRegistryURL = registryServer

	// The certificate must be distributed on every run, not only when the registry was just
	// created: a re-run that adds a data center finds the registry already up, and that data
	// center's nodes would otherwise not trust it.
	return b.distributeRegistryCert(registryNode, b.clusterNodes())
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
// credentials when it is not already serving. Returns the registry server address.
func (b *GCPBootstrapper) ensureRegistryRunning(registryNode *node.Node) (string, error) {
	localRegistryServer := registryNode.GetInternalIP() + ":5000"

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

	if err == nil && registryUsername != "" && registryPassword != "" {
		b.stlog.Logf("Local container registry already running on the jumpbox")
		b.Env.RegistryUsername = registryUsername
		b.Env.RegistryPassword = registryPassword

		return localRegistryServer, nil
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
		--env REGISTRY_HTTP_ADDR=0.0.0.0:5000 \
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
			return "", fmt.Errorf("failed to run command on the jumpbox: %w", err)
		}
	}

	return localRegistryServer, nil
}

// distributeRegistryCert installs the local registry's self-signed certificate on the given
// nodes. It is idempotent, so it is safe — and required — to re-run for an additional data center.
func (b *GCPBootstrapper) distributeRegistryCert(registryNode *node.Node, nodes []*node.Node) error {
	for _, node := range nodes {
		b.stlog.Logf("Configuring node '%s' to trust local registry certificate", node.GetName())

		err := registryNode.RunSSHCommand("root", "scp -o StrictHostKeyChecking=no /root/registry.crt root@"+node.GetInternalIP()+":/usr/local/share/ca-certificates/registry.crt")
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
