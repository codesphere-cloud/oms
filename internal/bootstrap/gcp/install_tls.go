// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/codesphere-cloud/oms/internal/bootstrap/datacenter"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/node"
	"github.com/codesphere-cloud/oms/internal/util"
	"go.yaml.in/yaml/v3"
)

// prepareInstallTLS keeps configuration and vault handling on the bootstrap host.
// Only Certbot needs the connected jumpbox's runtime and Google identity.
func (b *GCPBootstrapper) prepareInstallTLS(dc *datacenter.DataCenter) error {
	data, err := b.fw.ReadFile(dc.InstallConfigPath)
	if err != nil {
		return fmt.Errorf("reading certificate configuration: %w", err)
	}

	var config files.RootConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("decoding certificate configuration: %w", err)
	}

	domains := []string{config.Codesphere.Domain, "*." + config.Codesphere.Domain, config.Codesphere.WorkspaceHostingBaseDomain, "*." + config.Codesphere.WorkspaceHostingBaseDomain}
	if config.Codesphere.Domain == "" || config.Codesphere.WorkspaceHostingBaseDomain == "" {
		return fmt.Errorf("bootstrap TLS requires platform and workspace domains")
	}

	if b.Env.MultiDC {
		domain := dc.PlatformDomain(b.Env.BaseDomain)
		domains = append(domains, domain, "*."+domain)
	}

	secrets := dc.ConfigManager.GetVault()

	certificate, key, err := b.installCertificate(config, secrets, domains)
	if err != nil {
		return err
	}

	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("decoding raw certificate configuration: %w", err)
	}

	updated, err := installer.ConfigureCustomTLS(raw, certificate, key, secrets, domains)
	if err != nil {
		return fmt.Errorf("configuring custom certificates: %w", err)
	}

	rendered, err := yaml.Marshal(updated)
	if err != nil {
		return fmt.Errorf("encoding certificate configuration: %w", err)
	}

	if err := b.fw.Chmod(dc.InstallConfigPath, 0600); err != nil {
		return fmt.Errorf("protecting certificate configuration: %w", err)
	}

	if err := b.fw.WriteFile(dc.InstallConfigPath, rendered, 0600); err != nil {
		return fmt.Errorf("writing certificate configuration: %w", err)
	}

	if err := dc.ConfigManager.WriteVault(dc.SecretsFilePath, true); err != nil {
		return fmt.Errorf("saving certificate vault: %w", err)
	}

	if err := b.copyDataCenterConfig(dc); err != nil {
		return err
	}

	return b.encryptVault(dc)
}

// installCertificate preserves supplied material, including vault-backed keys.
func (b *GCPBootstrapper) installCertificate(config files.RootConfig, secrets *files.InstallVault, domains []string) ([]byte, []byte, error) {
	custom := config.Codesphere.CustomCertificate
	if custom == nil || custom.Cert == "" {
		return b.issueInstallCertificate(config, secrets, domains)
	}

	key := custom.Key
	if key == "" {
		if entry := secrets.GetSecret(files.SecretCustomCertificateKey); entry != nil && entry.File != nil {
			key = entry.File.Content
		}
	}

	return []byte(custom.Cert), []byte(key), nil
}

// issueInstallCertificate is GCP-specific: Certbot uses the connected jumpbox's
// Google identity and retains its ACME account and certificates there.
func (b *GCPBootstrapper) issueInstallCertificate(config files.RootConfig, secrets *files.InstallVault, domains []string) ([]byte, []byte, error) {
	dnsProject := b.Env.DNSProjectID
	if dnsProject == "" {
		dnsProject = b.Env.ProjectID
	}

	acme := config.Codesphere.CertIssuer
	if dnsProject == "" || acme == nil || acme.Acme == nil || acme.Acme.Server == "" || acme.Acme.Email == "" {
		return nil, nil, fmt.Errorf("bootstrap TLS requires a DNS project and configured ACME server and email")
	}

	const cacheDir = "/root/oms-acme"

	name := fmt.Sprintf("oms-%x", sha256.Sum256([]byte(strings.Join(domains, ",")+acme.Acme.Server+dnsProject)))[:20]

	args := []string{
		"run", "--rm",
		"--network", "host",
		"-v", cacheDir + ":/etc/letsencrypt:Z",
		"docker.io/certbot/dns-google:v5.8.0",
		"certonly", "--non-interactive", "--agree-tos",
		"--dns-google", "--dns-google-project", dnsProject,
		"--email", acme.Acme.Email,
		"--server", acme.Acme.Server,
		"--cert-name", name,
		"--keep-until-expiring",
	}
	if acme.Acme.EABKeyID != "" {
		key := secrets.GetSecret(files.SecretAcmeEabMacKey)
		if key == nil || key.Fields == nil || key.Fields.Password == "" {
			return nil, nil, fmt.Errorf("bootstrap TLS ACME EAB MAC key is missing")
		}

		args = append(args, "--eab-kid", acme.Acme.EABKeyID, "--eab-hmac-key", key.Fields.Password)
	}

	for _, domain := range domains {
		args = append(args, "-d", domain)
	}

	runner := jumpboxCertbotRunner{jumpbox: b.Env.Jumpbox, fw: b.fw}

	return runner.Issue(args, cacheDir, name)
}

type jumpboxCertbotRunner struct {
	jumpbox *node.Node
	fw      util.FileIO
}

func (r jumpboxCertbotRunner) Issue(args []string, cacheDir, name string) ([]byte, []byte, error) {
	command := "mkdir -p " + shellQuote(cacheDir) + " && chmod 700 " + shellQuote(cacheDir) + " && podman " + shellQuoteAll(args)
	if err := r.jumpbox.RunSSHCommand("root", command); err != nil {
		return nil, nil, fmt.Errorf("running Certbot on jumpbox: %w", err)
	}

	certificate, err := r.download(filepath.Join(cacheDir, "live", name, "fullchain.pem"))
	if err != nil {
		return nil, nil, err
	}

	key, err := r.download(filepath.Join(cacheDir, "live", name, "privkey.pem"))

	return certificate, key, err
}

func (r jumpboxCertbotRunner) download(remotePath string) ([]byte, error) {
	path, err := r.fw.CreateTemp("", "oms-bootstrap-tls-*")
	if err != nil {
		return nil, fmt.Errorf("creating certificate download file: %w", err)
	}
	defer func() { _ = r.fw.Remove(path) }()

	if err := r.jumpbox.NodeClient.DownloadFile(r.jumpbox, remotePath, path); err != nil {
		return nil, fmt.Errorf("downloading issued certificate material: %w", err)
	}

	data, err := r.fw.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading issued certificate material: %w", err)
	}

	return data, nil
}
