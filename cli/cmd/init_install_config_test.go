// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/codesphere-cloud/oms/internal/util"
)

func TestIsValidIP(t *testing.T) {
	tests := []struct {
		name  string
		ip    string
		valid bool
	}{
		{"valid IPv4", "192.168.1.1", true},
		{"valid IPv6", "2001:db8::1", true},
		{"invalid IP", "not-an-ip", false},
		{"empty string", "", false},
		{"partial IP", "192.168", false},
		{"localhost", "127.0.0.1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidIP(tt.ip)
			if result != tt.valid {
				t.Errorf("isValidIP(%q) = %v, want %v", tt.ip, result, tt.valid)
			}
		})
	}
}

func TestApplyProfile(t *testing.T) {
	tests := []struct {
		name            string
		profile         string
		wantErr         bool
		checkDatacenter string
	}{
		{"dev profile", "dev", false, "dev"},
		{"development profile", "development", false, "dev"},
		{"prod profile", "prod", false, "production"},
		{"production profile", "production", false, "production"},
		{"minimal profile", "minimal", false, "minimal"},
		{"invalid profile", "invalid", true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &InitInstallConfigCmd{
				Opts: &InitInstallConfigOpts{
					Profile: tt.profile,
				},
			}

			err := cmd.applyProfile()
			if (err != nil) != tt.wantErr {
				t.Errorf("applyProfile() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && cmd.Opts.DatacenterName != tt.checkDatacenter {
				t.Errorf("DatacenterName = %s, want %s", cmd.Opts.DatacenterName, tt.checkDatacenter)
			}
		})
	}
}

func TestApplyDevProfile(t *testing.T) {
	cmd := &InitInstallConfigCmd{
		Opts: &InitInstallConfigOpts{
			Profile: "dev",
		},
	}

	err := cmd.applyProfile()
	if err != nil {
		t.Fatalf("applyProfile failed: %v", err)
	}

	if cmd.Opts.DatacenterID != 1 {
		t.Errorf("DatacenterID = %d, want 1", cmd.Opts.DatacenterID)
	}
	if cmd.Opts.DatacenterName != "dev" {
		t.Errorf("DatacenterName = %s, want dev", cmd.Opts.DatacenterName)
	}
	if cmd.Opts.PostgresMode != "install" {
		t.Errorf("PostgresMode = %s, want install", cmd.Opts.PostgresMode)
	}
	if cmd.Opts.K8sManaged != true {
		t.Error("K8sManaged should be true for dev profile")
	}
}

func TestValidateConfig(t *testing.T) {
	configFile, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp config file: %v", err)
	}
	defer func() { _ = os.Remove(configFile.Name()) }()

	vaultFile, err := os.CreateTemp("", "vault-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp vault file: %v", err)
	}
	defer func() { _ = os.Remove(vaultFile.Name()) }()

	validConfig := `dataCenter:
  id: 1
  name: test
  city: Berlin
  countryCode: DE
secrets:
  baseDir: /root/secrets
postgres:
  serverAddress: postgres.example.com:5432
ceph:
  cephAdmSshKey:
    publicKey: ssh-rsa TEST
  nodesSubnet: 10.53.101.0/24
  hosts:
    - hostname: ceph-1
      ipAddress: 10.53.101.2
      isMaster: true
  osds: []
kubernetes:
  managedByCodesphere: false
  podCidr: 100.96.0.0/11
  serviceCidr: 100.64.0.0/13
cluster:
  certificates:
    ca:
      algorithm: RSA
      keySizeBits: 2048
      certPem: "-----BEGIN CERTIFICATE-----\nTEST\n-----END CERTIFICATE-----"
  gateway:
    serviceType: LoadBalancer
  publicGateway:
    serviceType: LoadBalancer
codesphere:
  domain: codesphere.example.com
  workspaceHostingBaseDomain: ws.example.com
  publicIp: 1.2.3.4
  customDomains:
    cNameBaseDomain: custom.example.com
  dnsServers:
    - 8.8.8.8
  experiments: []
  deployConfig:
    images:
      ubuntu-24.04:
        name: Ubuntu 24.04
        supportedUntil: "2028-05-31"
        flavors:
          default:
            image:
              bomRef: workspace-agent-24.04
            pool:
              1: 1
  plans:
    hostingPlans:
      1:
        cpuTenth: 10
        gpuParts: 0
        memoryMb: 2048
        storageMb: 20480
        tempStorageMb: 1024
    workspacePlans:
      1:
        name: Standard
        hostingPlanId: 1
        maxReplicas: 3
        onDemand: true
`

	validVault := `secrets:
  - name: cephSshPrivateKey
    file:
      name: id_rsa
      content: "-----BEGIN RSA PRIVATE KEY-----\nTEST\n-----END RSA PRIVATE KEY-----"
  - name: selfSignedCaKeyPem
    file:
      name: key.pem
      content: "-----BEGIN RSA PRIVATE KEY-----\nCA\n-----END RSA PRIVATE KEY-----"
  - name: domainAuthPrivateKey
    file:
      name: key.pem
      content: "-----BEGIN EC PRIVATE KEY-----\nDOMAIN\n-----END EC PRIVATE KEY-----"
  - name: domainAuthPublicKey
    file:
      name: key.pem
      content: "-----BEGIN PUBLIC KEY-----\nDOMAIN-PUB\n-----END PUBLIC KEY-----"
`

	if _, err := configFile.WriteString(validConfig); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}
	if err := configFile.Close(); err != nil {
		t.Fatalf("Failed to close config file: %v", err)
	}

	if _, err := vaultFile.WriteString(validVault); err != nil {
		t.Fatalf("Failed to write vault: %v", err)
	}
	if err := vaultFile.Close(); err != nil {
		t.Fatalf("Failed to close vault file: %v", err)
	}

	cmd := &InitInstallConfigCmd{
		Opts: &InitInstallConfigOpts{
			ConfigFile:   configFile.Name(),
			VaultFile:    vaultFile.Name(),
			ValidateOnly: true,
		},
		FileWriter: util.NewFilesystemWriter(),
	}

	err = cmd.validateConfig()
	if err != nil {
		t.Errorf("validateConfig() failed for valid config: %v", err)
	}
}

func TestValidateConfigInvalidDatacenter(t *testing.T) {
	configFile, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp config file: %v", err)
	}
	defer func() { _ = os.Remove(configFile.Name()) }()

	invalidConfig := `dataCenter:
  id: 0
  name: ""
secrets:
  baseDir: /root/secrets
postgres:
  serverAddress: postgres.example.com:5432
ceph:
  hosts: []
kubernetes:
  managedByCodesphere: true
cluster:
  certificates:
    ca:
      algorithm: RSA
      keySizeBits: 2048
      certPem: "-----BEGIN CERTIFICATE-----\nTEST\n-----END CERTIFICATE-----"
  gateway:
    serviceType: LoadBalancer
  publicGateway:
    serviceType: LoadBalancer
codesphere:
  domain: ""
  deployConfig:
    images: {}
  plans:
    hostingPlans: {}
    workspacePlans: {}
`

	if _, err := configFile.WriteString(invalidConfig); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}
	if err := configFile.Close(); err != nil {
		t.Fatalf("Failed to close config file: %v", err)
	}

	cmd := &InitInstallConfigCmd{
		Opts: &InitInstallConfigOpts{
			ConfigFile:   configFile.Name(),
			ValidateOnly: true,
		},
		FileWriter: util.NewFilesystemWriter(),
	}

	err = cmd.validateConfig()
	if err == nil {
		t.Error("validateConfig() should fail for invalid config")
	}
}

func TestValidateConfigInvalidIP(t *testing.T) {
	configFile, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp config file: %v", err)
	}
	defer func() { _ = os.Remove(configFile.Name()) }()

	configWithInvalidIP := `dataCenter:
  id: 1
  name: test
  city: Berlin
  countryCode: DE
secrets:
  baseDir: /root/secrets
postgres:
  serverAddress: postgres.example.com:5432
ceph:
  cephAdmSshKey:
    publicKey: ssh-rsa TEST
  nodesSubnet: 10.53.101.0/24
  hosts:
    - hostname: ceph-1
      ipAddress: invalid-ip-address
      isMaster: true
  osds: []
kubernetes:
  managedByCodesphere: true
  controlPlanes:
    - ipAddress: 10.0.0.1
cluster:
  certificates:
    ca:
      algorithm: RSA
      keySizeBits: 2048
      certPem: "-----BEGIN CERTIFICATE-----\nTEST\n-----END CERTIFICATE-----"
  gateway:
    serviceType: LoadBalancer
  publicGateway:
    serviceType: LoadBalancer
codesphere:
  domain: codesphere.example.com
  deployConfig:
    images: {}
  plans:
    hostingPlans: {}
    workspacePlans: {}
`

	if _, err := configFile.WriteString(configWithInvalidIP); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}
	if err := configFile.Close(); err != nil {
		t.Fatalf("Failed to close config file: %v", err)
	}

	cmd := &InitInstallConfigCmd{
		Opts: &InitInstallConfigOpts{
			ConfigFile:   configFile.Name(),
			ValidateOnly: true,
		},
		FileWriter: util.NewFilesystemWriter(),
	}

	err = cmd.validateConfig()
	if err == nil {
		t.Error("validateConfig() should fail for invalid IP address")
	}
	if err != nil && !strings.Contains(err.Error(), "invalid") {
		t.Logf("Got error: %v", err)
	}
}
