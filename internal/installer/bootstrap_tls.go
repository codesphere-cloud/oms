// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/util"
	"go.yaml.in/yaml/v4"
)

// BootstrapTLSVault persists bootstrap certificates without exposing private keys in config.
//
//mockery:generate: true
type BootstrapTLSVault interface {
	Load() (*files.InstallVault, error)
	Save(*files.InstallVault) error
}

// CertbotRunner is the external process boundary for jumpbox DNS-01 issuance.
//
//mockery:generate: true
type CertbotRunner interface{ Run(args []string) error }

// PodmanCertbotRunner uses the jumpbox container runtime and its GCP identity.
type PodmanCertbotRunner struct{}

// Run executes Certbot without logging its potentially sensitive EAB arguments.
func (PodmanCertbotRunner) Run(args []string) error {
	if err := util.RunCommand("podman", args, ""); err != nil {
		return fmt.Errorf("running Certbot container: %w", err)
	}

	return nil
}

// BootstrapTLSOptions selects the data center and persistent jumpbox ACME cache.
type BootstrapTLSOptions struct {
	ConfigPath string
	DNSProject string
	CacheDir   string
	Domains    []string
}

// EnsureBootstrapTLS obtains a DNS-01 certificate on the connected jumpbox and
// configures both public entrypoints to use it. Helm creates the TLS Secrets;
// cert-manager remains available for internal, self-signed service certificates.
// The raw config is updated to preserve unrelated vault template references.
func EnsureBootstrapTLS(opts BootstrapTLSOptions, fw util.FileIO, store BootstrapTLSVault, runner CertbotRunner) error {
	data, err := fw.ReadFile(opts.ConfigPath)
	if err != nil {
		return fmt.Errorf("reading bootstrap TLS config: %w", err)
	}

	var config files.RootConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("decoding bootstrap TLS config: %w", err)
	}

	domains := []string{config.Codesphere.Domain, "*." + config.Codesphere.Domain, config.Codesphere.WorkspaceHostingBaseDomain, "*." + config.Codesphere.WorkspaceHostingBaseDomain}

	domains = append(domains, opts.Domains...)
	if config.Codesphere.Domain == "" || config.Codesphere.WorkspaceHostingBaseDomain == "" || opts.DNSProject == "" || !filepath.IsAbs(opts.CacheDir) {
		return fmt.Errorf("bootstrap TLS requires platform and workspace domains, a DNS project and an absolute cache directory")
	}

	secrets, err := store.Load()
	if err != nil {
		return fmt.Errorf("loading bootstrap TLS vault: %w", err)
	}

	acme := config.Codesphere.CertIssuer
	if acme == nil || acme.Acme == nil || acme.Acme.Server == "" || acme.Acme.Email == "" {
		return fmt.Errorf("bootstrap TLS requires configured ACME server and email")
	}

	name := fmt.Sprintf("oms-%x", sha256.Sum256([]byte(strings.Join(domains, ",")+acme.Acme.Server+opts.DNSProject)))[:20]
	if err := fw.MkdirAll(opts.CacheDir, 0700); err != nil {
		return fmt.Errorf("creating ACME cache: %w", err)
	}

	if err := fw.Chmod(opts.CacheDir, 0700); err != nil {
		return fmt.Errorf("protecting ACME cache: %w", err)
	}

	args := []string{"run", "--rm", "--network", "host", "-v", opts.CacheDir + ":/etc/letsencrypt:Z", "docker.io/certbot/dns-google:v5.8.0", "certonly", "--non-interactive", "--agree-tos", "--dns-google", "--dns-google-project", opts.DNSProject, "--email", acme.Acme.Email, "--server", acme.Acme.Server, "--cert-name", name, "--keep-until-expiring"}
	if acme.Acme.EABKeyID != "" {
		key := secrets.GetSecret(files.SecretAcmeEabMacKey)
		if key == nil || key.Fields == nil || key.Fields.Password == "" {
			return fmt.Errorf("bootstrap TLS ACME EAB MAC key is missing")
		}

		args = append(args, "--eab-kid", acme.Acme.EABKeyID, "--eab-hmac-key", key.Fields.Password)
	}

	for _, domain := range domains {
		args = append(args, "-d", domain)
	}

	if err := runner.Run(args); err != nil {
		return fmt.Errorf("issuing jumpbox DNS-01 certificate: %w", err)
	}

	cert, err := fw.ReadFile(filepath.Join(opts.CacheDir, "live", name, "fullchain.pem"))
	if err != nil {
		return fmt.Errorf("reading issued certificate: %w", err)
	}

	key, err := fw.ReadFile(filepath.Join(opts.CacheDir, "live", name, "privkey.pem"))
	if err != nil {
		return fmt.Errorf("reading issued certificate key: %w", err)
	}

	if err := validateBootstrapCertificate(cert, key, domains); err != nil {
		return err
	}

	return writeBootstrapTLS(opts.ConfigPath, data, cert, key, secrets, fw, store)
}

func validateBootstrapCertificate(cert, key []byte, domains []string) error {
	pair, err := tls.X509KeyPair(cert, key)
	if err != nil {
		return fmt.Errorf("invalid bootstrap TLS certificate/key pair: %w", err)
	}

	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return fmt.Errorf("parsing bootstrap TLS certificate: %w", err)
	}

	for _, domain := range domains {
		hostname := strings.Replace(domain, "*.", "oms-bootstrap-validation.", 1)
		if err := leaf.VerifyHostname(hostname); err != nil {
			return fmt.Errorf("bootstrap certificate does not cover %s: %w", domain, err)
		}
	}

	return nil
}

func writeBootstrapTLS(path string, data, cert, key []byte, secrets *files.InstallVault, fw util.FileIO, store BootstrapTLSVault) error {
	const encodedKeyName = "bootstrapWorkspaceTLSKeyBase64"

	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("decoding raw bootstrap config: %w", err)
	}

	manualCertificate := map[string]any{"nginx": map[string]any{"ingress": map[string]any{"certificate": map[string]any{
		"provideCertificateManually": true, "cert": base64.StdEncoding.EncodeToString(cert),
		"key": `{{ secret "` + encodedKeyName + `" }}`,
	}}}}
	updates := map[string]any{
		"codesphere": map[string]any{
			// The native chart reads the vault key directly; OMS resolves it into
			// the temporary config for the legacy installer.
			"customCertificate": map[string]any{"cert": string(cert), "key": ""},
			// Retain ACME account settings for idempotent jumpbox reissuance, but never use
			// an external issuer inside the isolated cluster.
			"certIssuer": map[string]any{"type": "self-signed", "acme": map[string]any{"enabled": false}},
		},
		"cluster": map[string]any{
			"certificates": map[string]any{"override": map[string]any{"issuers": map[string]any{
				"acme": map[string]any{"enabled": false}, "letsEncryptHttp": map[string]any{"enabled": false},
			}}},
			"publicGateway": map[string]any{"override": manualCertificate},
		},
		"pcApps": map[string]any{"applications": map[string]any{"public-gateway": map[string]any{"valuesObject": manualCertificate}}},
	}
	merged := util.DeepMergeMaps(raw, updates)

	updated, err := yaml.Marshal(merged)
	if err != nil {
		return fmt.Errorf("encoding bootstrap TLS config: %w", err)
	}

	secrets.SetSecret(files.SecretEntry{Name: files.SecretCustomCertificateKey, File: &files.SecretFile{Name: "bootstrap-tls.key", Content: string(key)}})
	secrets.SetSecret(files.SecretEntry{Name: encodedKeyName, Fields: &files.SecretFields{Password: base64.StdEncoding.EncodeToString(key)}})

	if err := store.Save(secrets); err != nil {
		return fmt.Errorf("saving bootstrap TLS vault: %w", err)
	}

	if err := fw.Chmod(path, 0600); err != nil {
		return fmt.Errorf("protecting bootstrap TLS config: %w", err)
	}

	if err := fw.WriteFile(path, updated, 0600); err != nil {
		return fmt.Errorf("writing bootstrap TLS config: %w", err)
	}

	return nil
}
