// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/codesphere-cloud/oms/internal/bootstrap/datacenter"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/node"
	"github.com/codesphere-cloud/oms/internal/util"
)

// prepareInstallTLS keeps configuration and vault handling on the bootstrap host.
// Only Certbot needs the connected jumpbox's runtime and Google identity.
func (b *GCPBootstrapper) prepareInstallTLS(dc *datacenter.DataCenter) error {
	dnsProject := b.Env.DNSProjectID
	if dnsProject == "" {
		dnsProject = b.Env.ProjectID
	}

	opts := installer.BootstrapTLSOptions{ConfigPath: dc.InstallConfigPath, DNSProject: dnsProject, CacheDir: "/root/oms-acme"}
	if b.Env.MultiDC {
		domain := dc.PlatformDomain(b.Env.BaseDomain)
		opts.Domains = []string{domain, "*." + domain}
	}

	store := bootstrapTLSVault{manager: dc.ConfigManager, path: dc.SecretsFilePath}

	runner := jumpboxCertbotRunner{jumpbox: b.Env.Jumpbox, fw: b.fw}
	if err := installer.EnsureBootstrapTLS(opts, b.fw, store, runner); err != nil {
		return fmt.Errorf("preparing local certificate configuration: %w", err)
	}

	if err := b.Env.Jumpbox.NodeClient.CopyFile(b.Env.Jumpbox, dc.InstallConfigPath, dc.RemoteConfigPath); err != nil {
		return fmt.Errorf("uploading certificate config: %w", err)
	}

	if err := b.Env.Jumpbox.NodeClient.CopyFile(b.Env.Jumpbox, dc.SecretsFilePath, dc.RemoteVaultPath()); err != nil {
		return fmt.Errorf("uploading certificate vault: %w", err)
	}

	return b.encryptVault(dc)
}

type bootstrapTLSVault struct {
	manager installer.InstallConfigManager
	path    string
}

func (v bootstrapTLSVault) Load() (*files.InstallVault, error) { return v.manager.GetVault(), nil }

func (v bootstrapTLSVault) Save(secrets *files.InstallVault) error {
	v.manager.SetVault(secrets)

	if err := v.manager.WriteVault(v.path, true); err != nil {
		return fmt.Errorf("saving local certificate vault: %w", err)
	}

	return nil
}

type jumpboxCertbotRunner struct {
	jumpbox *node.Node
	fw      util.FileIO
}

func (r jumpboxCertbotRunner) Issue(args []string, cacheDir, name string) ([]byte, []byte, error) {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = shellQuote(arg)
	}

	command := "mkdir -p " + shellQuote(cacheDir) + " && chmod 700 " + shellQuote(cacheDir) + " && podman " + strings.Join(quoted, " ")
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
