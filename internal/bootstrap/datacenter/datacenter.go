// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package datacenter models one Codesphere data center of a bootstrapped installation: the nodes
// it runs on, the addresses it is reached through, and the paths of the config and vault that
// describe it. A multi-data-center installation has one of these per data center.
//
// The model deliberately carries no reference to the infrastructure a data center runs on, so the
// bootstrap flows can share it and derive it from their own environment.
package datacenter

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/node"
)

// PrimaryID is the ID of the first data center. It stays 1 in both single- and multi-DC mode, so
// single-DC installations keep their existing dataCenter.id.
const PrimaryID = 1

// DataCenter holds the state of one Codesphere data center. Everything that a bootstrap shares
// between its data centers (the cloud project, the network, the jumpbox, the PostgreSQL server
// and the container registry) stays with the bootstrap; everything that must differ between them
// lives here.
type DataCenter struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	// Suffix is appended to data-center-scoped resource names. It is empty for the primary
	// data center, so single-DC bootstraps keep the resource names they have always used.
	Suffix string `json:"suffix"`

	ControlPlaneNodes []*node.Node `json:"control_plane_nodes"`
	CephNodes         []*node.Node `json:"ceph_nodes"`

	GatewayIP       string `json:"gateway_ip"`
	PublicGatewayIP string `json:"public_gateway_ip"`
	SSHProxyIP      string `json:"ssh_proxy_ip"`

	// Local paths of the generated config and vault.
	InstallConfigPath string `json:"-"`
	SecretsFilePath   string `json:"-"`
	// Paths on the shared jumpbox.
	RemoteConfigPath string `json:"remote_config_path"`
	SecretsDir       string `json:"secrets_dir"`

	WorkspaceHostingBaseDomain string `json:"workspace_hosting_base_domain"`
	SSHBaseDomain              string `json:"ssh_base_domain"`

	// ExternalPostgres marks a data center that uses the primary data center's PostgreSQL
	// server instead of installing its own.
	ExternalPostgres bool `json:"external_postgres"`

	InstallConfig      *files.RootConfig `json:"-"`
	ExistingConfigUsed bool              `json:"-"`
	// ConfigManager owns this data center's config and vault. It is not serialised, so a data
	// center restored from an infra file is given a fresh one.
	ConfigManager installer.InstallConfigManager `json:"-"`
}

// IsPrimary reports whether this is the first data center of the installation. The primary data
// center owns the shared PostgreSQL server and the platform gateway that codesphere.domain
// resolves to.
func (dc *DataCenter) IsPrimary() bool {
	return dc.Suffix == ""
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

// SuffixedPath inserts a data-center suffix before the file extension, turning config.yaml into
// config-dc2.yaml and prod.vault.yaml into prod-dc2.vault.yaml. The primary data center's empty
// suffix leaves the path untouched.
func SuffixedPath(path, suffix string) string {
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
