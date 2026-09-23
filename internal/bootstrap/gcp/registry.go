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
// points the install config's registry server at it
func (b *GCPBootstrapper) EnsureArtifactRegistry() error {
	repoName := "codesphere-registry"

	repo, err := b.GCPClient.GetArtifactRegistry(b.Env.ProjectID, b.Env.Region, repoName)
	if err == nil && repo != nil {
		b.Env.InstallConfig.EnsureRegistry().Server = repo.GetRegistryUri()
		return nil
	}

	repo, err = b.GCPClient.CreateArtifactRegistry(b.Env.ProjectID, b.Env.Region, repoName)
	if err != nil || repo == nil {
		return fmt.Errorf("failed to create artifact registry: %w, repo: %v", err, repo)
	}

	return nil
}

// EnsureLocalContainerRegistry installs a docker registry on the jumpbox to speed up image loading time
func (b *GCPBootstrapper) EnsureLocalContainerRegistry() error {
	registryNode := b.Env.Jumpbox
	if registryNode == nil {
		return fmt.Errorf("jumpbox not found in bootstrap environment")
	}

	if registryNode.GetInternalIP() == "" {
		return fmt.Errorf("jumpbox has no internal IP")
	}

	registry := b.Env.InstallConfig.EnsureRegistry()
	localRegistryServer := registryNode.GetInternalIP() + ":5000"

	// Figure out if registry is already running
	b.stlog.Logf("Checking if local container registry is already running on the jumpbox")

	checkCommand := `test "$(podman ps --filter 'name=registry' --format '{{.Names}}' | wc -l)" -eq "1"`
	err := registryNode.RunSSHCommand("root", checkCommand)
	registryUsername := ""
	registryPassword := ""

	if s := b.icg.GetVault().GetSecret(files.SecretRegistryUsername); s != nil && s.Fields != nil {
		registryUsername = s.Fields.Password
	}

	if s := b.icg.GetVault().GetSecret(files.SecretRegistryPassword); s != nil && s.Fields != nil {
		registryPassword = s.Fields.Password
	}

	if err == nil && registry.Server == localRegistryServer &&
		registryUsername != "" && registryPassword != "" {
		b.stlog.Logf("Local container registry already running on the jumpbox")

		err := b.ensureJumpboxRegistryAccess(registryNode, localRegistryServer, registryUsername, registryPassword)
		if err != nil {
			return err
		}

		return b.distributeRegistryCertificate(registryNode)
	}

	registry.Server = localRegistryServer
	registryUsername = "custom-registry"
	registryPassword = shortuuid.New()

	b.icg.GetVault().SetSecret(files.SecretEntry{Name: files.SecretRegistryUsername, Fields: &files.SecretFields{Password: registryUsername}})
	b.icg.GetVault().SetSecret(files.SecretEntry{Name: files.SecretRegistryPassword, Fields: &files.SecretFields{Password: registryPassword}})

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
		`mkdir -p /etc/docker/certs.d/` + registry.Server,
		`cp /root/registry.crt /etc/docker/certs.d/` + registry.Server + `/ca.crt`,
	}
	for _, cmd := range commands {
		b.stlog.Logf("Running command on the jumpbox: %s", util.Truncate(cmd, 12))

		err := registryNode.RunSSHCommand("root", cmd)
		if err != nil {
			return fmt.Errorf("failed to run command on the jumpbox: %w", err)
		}
	}

	if err := b.ensureJumpboxRegistryAccess(registryNode, registry.Server, registryUsername, registryPassword); err != nil {
		return err
	}

	return b.distributeRegistryCertificate(registryNode)
}

// distributeRegistryCertificate installs the registry's certificate on every node that pulls
// images from it. The postgres node needs it just like the cluster nodes do, and the work is
// repeated whenever the registry is ensured so that a rerun repairs an environment whose nodes
// never received the certificate.
func (b *GCPBootstrapper) distributeRegistryCertificate(registryNode *node.Node) error {
	allNodes := append([]*node.Node{b.Env.PostgreSQLNode}, b.Env.ControlPlaneNodes...)
	allNodes = append(allNodes, b.Env.CephNodes...)

	for _, node := range allNodes {
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

// ensureJumpboxRegistryAccess lets the jumpbox itself talk to the registries a local container
// registry setup involves. Go tools such as OMS and the crane library it copies images with read
// the system trust store and the Docker config instead of the podman locations the registry
// installation writes to, so the registry CA and the credentials are installed where they can
// find them. The upstream login is what allows 'oms copy package' to mirror the Codesphere
// images into the local registry.
func (b *GCPBootstrapper) ensureJumpboxRegistryAccess(registryNode *node.Node, server, username, password string) error {
	commands := []string{
		"cp /root/registry.crt /usr/local/share/ca-certificates/registry.crt",
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
// piped in through the shell built-in printf so it never shows up in the jumpbox process list.
func registryLoginCommand(server, username, password string) string {
	return fmt.Sprintf("printf '%%s' '%s' | podman login --authfile %s --username '%s' --password-stdin %s",
		password, jumpboxRegistryAuthFile, username, server)
}

// EnsureGitHubAccessConfigured points the install config at ghcr.io and stores the GitHub
// credentials in the vault. The cluster pulls images from GHCR directly
func (b *GCPBootstrapper) EnsureGitHubAccessConfigured() error {
	if b.Env.GitHubPAT == "" {
		return fmt.Errorf("GitHub PAT is not set")
	}

	registry := b.Env.InstallConfig.EnsureRegistry()
	registry.Server = "ghcr.io"
	registry.ReplaceImagesInBom = false
	registry.LoadContainerImages = false

	b.icg.GetVault().SetSecret(files.SecretEntry{Name: files.SecretRegistryUsername, Fields: &files.SecretFields{Password: b.Env.RegistryUser}})
	b.icg.GetVault().SetSecret(files.SecretEntry{Name: files.SecretRegistryPassword, Fields: &files.SecretFields{Password: b.Env.GitHubPAT}})

	return nil
}
