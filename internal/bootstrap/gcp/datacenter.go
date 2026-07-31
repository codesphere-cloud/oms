// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"fmt"

	"github.com/codesphere-cloud/oms/internal/bootstrap/datacenter"
	"github.com/codesphere-cloud/oms/internal/installer"
)

// BuildDataCenters derives the data center layout from the bootstrap environment: a single
// entry in single-DC mode, and two entries in multi-DC mode where the second one shares the
// first one's PostgreSQL server.
func BuildDataCenters(env *CodesphereEnvironment, newICG func() installer.InstallConfigManager) []*datacenter.DataCenter {
	if !env.MultiDC {
		// A single data center keeps honouring --datacenter-id. In multi-DC mode the IDs are
		// derived instead, because they drive the per-data-center domains; validateMultiDC
		// rejects the combination.
		id := env.DatacenterID
		if id == 0 {
			id = datacenter.PrimaryID
		}
		return []*datacenter.DataCenter{newDataCenter(env, id, "", newICG)}
	}

	return []*datacenter.DataCenter{
		newDataCenter(env, datacenter.PrimaryID, "", newICG),
		newDataCenter(env, datacenter.PrimaryID+1, "-dc2", newICG),
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
		if dc.ConfigManager != nil {
			continue
		}
		if i == 0 && b.icg != nil {
			dc.ConfigManager = b.icg
			continue
		}
		dc.ConfigManager = newICG()
	}
}

// adoptLegacyEnvFields moves state that a caller supplied through the deprecated top-level
// environment fields into the primary data center. Infra files written before multi-DC support
// carry the primary data center's nodes and IPs there.
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

// mirrorPrimaryDataCenter projects the primary data center's state back onto the top-level
// environment fields it lived in before multi-DC support. The steps that still read those fields
// keep working while they are migrated one by one, and the infra file keeps the shape an earlier
// OMS wrote. The projection is one-way and never read back into a DataCenter.
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
func newDataCenter(env *CodesphereEnvironment, id int, suffix string, newICG func() installer.InstallConfigManager) *datacenter.DataCenter {
	name := env.DatacenterName
	if name == "" {
		name = "dev"
	}
	if suffix != "" {
		// The k0s cluster is named codesphere-<datacenter name>, so the names must differ.
		name += suffix
	}

	dc := &datacenter.DataCenter{
		ID:                         id,
		Name:                       name,
		Suffix:                     suffix,
		InstallConfigPath:          datacenter.SuffixedPath(env.InstallConfigPath, suffix),
		SecretsFilePath:            datacenter.SuffixedPath(env.SecretsFilePath, suffix),
		RemoteConfigPath:           datacenter.SuffixedPath(remoteInstallConfigPath, suffix),
		SecretsDir:                 env.SecretsDir + suffix,
		WorkspaceHostingBaseDomain: workspaceHostingBaseDomain(env, id),
		SshBaseDomain:              sshBaseDomain(env, id),
		ExternalPostgres:           suffix != "",
	}
	if newICG != nil {
		dc.ConfigManager = newICG()
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
