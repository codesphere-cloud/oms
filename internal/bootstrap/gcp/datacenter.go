// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/node"
)

// primaryDatacenterID is the ID of the first data center. It stays 1 in both modes so
// single-DC bootstraps keep their existing dataCenter.id.
const primaryDatacenterID = 1

// DataCenter holds the state of one Codesphere data center inside a bootstrapped GCP project.
// Project-level state (the GCP project, VPC, jumpbox, shared postgres node and container
// registry) lives on CodesphereEnvironment; everything that must differ between data centers
// lives here.
type DataCenter struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	// Suffix is appended to data-center-scoped GCP resource names. It is empty for the primary
	// data center, so single-DC bootstraps keep the resource names they have always used.
	Suffix string `json:"suffix"`

	ControlPlaneNodes []*node.Node `json:"control_plane_nodes"`
	CephNodes         []*node.Node `json:"ceph_nodes"`

	GatewayIP       string `json:"gateway_ip"`
	PublicGatewayIP string `json:"public_gateway_ip"`
	SshProxyIP      string `json:"ssh_proxy_ip"`

	// Local paths of the generated config and vault.
	InstallConfigPath string `json:"-"`
	SecretsFilePath   string `json:"-"`
	// Paths on the shared jumpbox.
	RemoteConfigPath string `json:"remote_config_path"`
	SecretsDir       string `json:"secrets_dir"`

	WorkspaceHostingBaseDomain string `json:"workspace_hosting_base_domain"`
	SshBaseDomain              string `json:"ssh_base_domain"`

	// ExternalPostgres marks a data center that uses the primary data center's PostgreSQL
	// server instead of installing its own.
	ExternalPostgres bool `json:"external_postgres"`

	InstallConfig      *files.RootConfig              `json:"-"`
	ExistingConfigUsed bool                           `json:"-"`
	icg                installer.InstallConfigManager `json:"-"`
}

// IsPrimary reports whether this is the first data center of the installation. The primary data
// center owns the shared PostgreSQL server and the platform gateway that codesphere.domain
// resolves to.
func (dc *DataCenter) IsPrimary() bool {
	return dc.Suffix == ""
}

// ConfigManager returns the install config manager owning this data center's config and vault.
func (dc *DataCenter) ConfigManager() installer.InstallConfigManager {
	return dc.icg
}

// SetConfigManager assigns the install config manager for this data center. Exported for tests;
// Bootstrap assigns it via BuildDataCenters.
func (dc *DataCenter) SetConfigManager(icg installer.InstallConfigManager) {
	dc.icg = icg
}

// RemoteVaultPath returns the path of this data center's vault on the jumpbox.
func (dc *DataCenter) RemoteVaultPath() string {
	return filepath.Join(dc.SecretsDir, "prod.vault.yaml")
}

// RemoteAgeKeyPath returns the path of this data center's age identity on the jumpbox.
func (dc *DataCenter) RemoteAgeKeyPath() string {
	return filepath.Join(dc.SecretsDir, "age_key.txt")
}

// K0sConfigScriptPath returns the local filename of this data center's k0s configuration script.
func (dc *DataCenter) K0sConfigScriptPath() string {
	return fmt.Sprintf("configure-k0s%s.sh", dc.Suffix)
}

// StepName qualifies a bootstrap step name with the data center it applies to. Single-DC
// bootstraps keep their unqualified step names.
func (dc *DataCenter) StepName(name string) string {
	if dc.Suffix == "" {
		return name
	}
	return fmt.Sprintf("%s (dc %d)", name, dc.ID)
}

// BuildDataCenters derives the data center layout from the bootstrap environment: a single
// entry in single-DC mode, and two entries in multi-DC mode where the second one shares the
// first one's PostgreSQL server.
func BuildDataCenters(env *CodesphereEnvironment, newICG func() installer.InstallConfigManager) []*DataCenter {
	if !env.MultiDC {
		// A single data center keeps honouring --datacenter-id. In multi-DC mode the IDs are
		// derived instead, because they drive the per-data-center domains; validateMultiDC
		// rejects the combination.
		id := env.DatacenterID
		if id == 0 {
			id = primaryDatacenterID
		}
		return []*DataCenter{newDataCenter(env, id, "", newICG)}
	}

	return []*DataCenter{
		newDataCenter(env, primaryDatacenterID, "", newICG),
		newDataCenter(env, primaryDatacenterID+1, "-dc2", newICG),
	}
}

// ensureDataCenters makes sure the environment has a usable data center layout. It derives the
// layout on first use and gives every data center an install config manager, so any entry point
// works whether or not Bootstrap ran first.
func (b *GCPBootstrapper) ensureDataCenters() {
	if len(b.Env.DataCenters) > 0 {
		b.ensureConfigManagers()
		return
	}

	b.Env.DataCenters = BuildDataCenters(b.Env, nil)
	b.adoptLegacyEnvFields()
	b.ensureConfigManagers()
}

// ensureConfigManagers gives every data center an install config manager. The primary one reuses
// the bootstrapper's, so a single-DC bootstrap behaves exactly as it did before multi-DC support.
// Data centers restored from an infra file arrive without a manager, since it is not serialised.
func (b *GCPBootstrapper) ensureConfigManagers() {
	newICG := b.NewConfigManager
	if newICG == nil {
		newICG = installer.NewInstallConfigManager
	}

	for i, dc := range b.Env.DataCenters {
		if dc.ConfigManager() != nil {
			continue
		}
		if i == 0 && b.icg != nil {
			dc.SetConfigManager(b.icg)
			continue
		}
		dc.SetConfigManager(newICG())
	}
}

// adoptLegacyEnvFields moves state that a caller supplied through the legacy top-level
// environment fields into the primary data center. Environments loaded from an infra file written
// before multi-DC support carry the primary data center's nodes and IPs there.
func (b *GCPBootstrapper) adoptLegacyEnvFields() {
	primary := b.Env.DataCenters[0]
	if len(primary.ControlPlaneNodes) == 0 {
		primary.ControlPlaneNodes = b.Env.ControlPlaneNodes
	}
	if len(primary.CephNodes) == 0 {
		primary.CephNodes = b.Env.CephNodes
	}
	if primary.GatewayIP == "" {
		primary.GatewayIP = b.Env.GatewayIP
	}
	if primary.PublicGatewayIP == "" {
		primary.PublicGatewayIP = b.Env.PublicGatewayIP
	}
	if primary.SshProxyIP == "" {
		primary.SshProxyIP = b.Env.SshProxyIP
	}
	if primary.InstallConfig == nil {
		primary.InstallConfig = b.Env.InstallConfig
	}
	// A caller that supplied a config through the environment also tells us whether it is an
	// existing one, which decides between generating and regenerating secrets.
	if b.Env.ExistingConfigUsed {
		primary.ExistingConfigUsed = true
	}
}

// mirrorPrimaryDataCenter projects the primary data center's state onto the legacy top-level
// environment fields. Those are what the infra file exposes to `cleanup` and `restart-vms`, and
// what infra files written before multi-DC support contain. The projection is one-way and never
// read back into a DataCenter.
func (b *GCPBootstrapper) mirrorPrimaryDataCenter() {
	if len(b.Env.DataCenters) == 0 {
		return
	}

	primary := b.primaryDC()
	b.Env.ControlPlaneNodes = primary.ControlPlaneNodes
	b.Env.CephNodes = primary.CephNodes
	b.Env.GatewayIP = primary.GatewayIP
	b.Env.PublicGatewayIP = primary.PublicGatewayIP
	b.Env.SshProxyIP = primary.SshProxyIP
	b.Env.InstallConfig = primary.InstallConfig
	b.Env.ExistingConfigUsed = primary.ExistingConfigUsed
}

// newDataCenter builds one data center, deriving its resource names, file paths and domains
// from the environment and the data-center suffix.
func newDataCenter(env *CodesphereEnvironment, id int, suffix string, newICG func() installer.InstallConfigManager) *DataCenter {
	name := env.DatacenterName
	if name == "" {
		name = "dev"
	}
	if suffix != "" {
		// The k0s cluster is named codesphere-<datacenter name>, so the names must differ.
		name += suffix
	}

	dc := &DataCenter{
		ID:                         id,
		Name:                       name,
		Suffix:                     suffix,
		InstallConfigPath:          dcSuffixedPath(env.InstallConfigPath, suffix),
		SecretsFilePath:            dcSuffixedPath(env.SecretsFilePath, suffix),
		RemoteConfigPath:           dcSuffixedPath(remoteInstallConfigPath, suffix),
		SecretsDir:                 env.SecretsDir + suffix,
		WorkspaceHostingBaseDomain: workspaceHostingBaseDomain(env, id),
		SshBaseDomain:              sshBaseDomain(env, id),
		ExternalPostgres:           suffix != "",
	}
	if newICG != nil {
		dc.icg = newICG()
	}

	return dc
}

// workspaceHostingBaseDomain returns the domain workspaces of the given data center are served
// from. Single-DC installations keep ws.<base-domain>; multi-DC installations prefix it with the
// data center ID so each data center's public gateway gets its own name.
func workspaceHostingBaseDomain(env *CodesphereEnvironment, id int) string {
	if !env.MultiDC {
		return "ws." + env.BaseDomain
	}
	return fmt.Sprintf("%d.ws.%s", id, env.BaseDomain)
}

// sshBaseDomain returns the domain the workspace SSH proxy of the given data center is served
// from, following the same scheme as workspaceHostingBaseDomain.
func sshBaseDomain(env *CodesphereEnvironment, id int) string {
	if !env.MultiDC {
		return "ssh.cs." + env.BaseDomain
	}
	return fmt.Sprintf("%d.ssh.cs.%s", id, env.BaseDomain)
}

// dcSuffixedPath inserts the data-center suffix before the file extension, turning
// config.yaml into config-dc2.yaml and prod.vault.yaml into prod-dc2.vault.yaml.
func dcSuffixedPath(path, suffix string) string {
	if suffix == "" {
		return path
	}

	dir, file := filepath.Split(path)
	base, ext := file, ""
	if idx := strings.Index(file, "."); idx > 0 {
		base, ext = file[:idx], file[idx:]
	}

	return filepath.Join(dir, base+suffix+ext)
}
