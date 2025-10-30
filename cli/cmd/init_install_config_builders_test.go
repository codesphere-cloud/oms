// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"strings"
	"testing"
)

func TestBuildGen0Config(t *testing.T) {
	cmd := &InitInstallConfigCmd{
		Opts: &InitInstallConfigOpts{
			DatacenterID:                      1,
			DatacenterName:                    "test-dc",
			DatacenterCity:                    "Berlin",
			DatacenterCountryCode:             "DE",
			SecretsBaseDir:                    "/root/secrets",
			CephSubnet:                        "10.53.101.0/24",
			CephHosts:                         []CephHostConfig{{Hostname: "ceph-1", IPAddress: "10.53.101.2", IsMaster: true}},
			PostgresMode:                      "install",
			PostgresPrimaryIP:                 "10.50.0.2",
			PostgresPrimaryHost:               "pg-primary",
			PostgresReplicaIP:                 "10.50.0.3",
			PostgresReplicaName:               "replica1",
			K8sManaged:                        true,
			K8sAPIServer:                      "10.50.0.2",
			K8sControlPlane:                   []string{"10.50.0.2"},
			K8sWorkers:                        []string{"10.50.0.3"},
			ClusterGatewayType:                "LoadBalancer",
			ClusterPublicGatewayType:          "LoadBalancer",
			CodesphereDomain:                  "codesphere.example.com",
			CodesphereWorkspaceBaseDomain:     "ws.example.com",
			CodespherePublicIP:                "1.2.3.4",
			CodesphereCustomDomainBaseDomain:  "custom.example.com",
			CodesphereDNSServers:              []string{"8.8.8.8"},
			CodesphereWorkspaceImageBomRef:    "workspace-agent-24.04",
			CodesphereHostingPlanCPU:          10,
			CodesphereHostingPlanMemory:       2048,
			CodesphereHostingPlanStorage:      20480,
			CodesphereHostingPlanTempStorage:  1024,
			CodesphereWorkspacePlanName:       "Standard",
			CodesphereWorkspacePlanMaxReplica: 3,
		},
	}

	secrets := &GeneratedSecrets{
		CephSSHPublicKey:      "ssh-rsa TEST",
		IngressCACert:         "-----BEGIN CERTIFICATE-----\nTEST\n-----END CERTIFICATE-----",
		PostgresCACert:        "-----BEGIN CERTIFICATE-----\nPG-CA\n-----END CERTIFICATE-----",
		PostgresPrimaryCert:   "-----BEGIN CERTIFICATE-----\nPG-PRIMARY\n-----END CERTIFICATE-----",
		PostgresReplicaCert:   "-----BEGIN CERTIFICATE-----\nPG-REPLICA\n-----END CERTIFICATE-----",
		PostgresUserPasswords: map[string]string{"auth": "password123"},
	}

	config := cmd.buildGen0Config(secrets)

	if config.DataCenter.ID != 1 {
		t.Errorf("DataCenter.ID = %d, want 1", config.DataCenter.ID)
	}
	if config.DataCenter.Name != "test-dc" {
		t.Errorf("DataCenter.Name = %s, want test-dc", config.DataCenter.Name)
	}

	if len(config.Ceph.Hosts) != 1 {
		t.Errorf("len(Ceph.Hosts) = %d, want 1", len(config.Ceph.Hosts))
	}
	if config.Ceph.Hosts[0].Hostname != "ceph-1" {
		t.Errorf("Ceph.Hosts[0].Hostname = %s, want ceph-1", config.Ceph.Hosts[0].Hostname)
	}
	if config.Ceph.CephAdmSSHKey.PublicKey != "ssh-rsa TEST" {
		t.Error("Ceph SSH public key not set correctly")
	}

	if config.Postgres.CACertPem == "" {
		t.Error("Postgres.CACertPem should not be empty")
	}
	if config.Postgres.Primary == nil {
		t.Fatal("Postgres.Primary should not be nil")
	}
	if config.Postgres.Primary.IP != "10.50.0.2" {
		t.Errorf("Postgres.Primary.IP = %s, want 10.50.0.2", config.Postgres.Primary.IP)
	}
	if config.Postgres.Replica == nil {
		t.Fatal("Postgres.Replica should not be nil")
	}

	if !config.Kubernetes.ManagedByCodesphere {
		t.Error("Kubernetes.ManagedByCodesphere should be true")
	}
	if len(config.Kubernetes.ControlPlanes) != 1 {
		t.Errorf("len(Kubernetes.ControlPlanes) = %d, want 1", len(config.Kubernetes.ControlPlanes))
	}

	if config.Codesphere.Domain != "codesphere.example.com" {
		t.Errorf("Codesphere.Domain = %s, want codesphere.example.com", config.Codesphere.Domain)
	}
	if len(config.Codesphere.Plans.HostingPlans) != 1 {
		t.Error("Should have one hosting plan")
	}
	if len(config.Codesphere.Plans.WorkspacePlans) != 1 {
		t.Error("Should have one workspace plan")
	}
}

func TestBuildGen0ConfigExternalPostgres(t *testing.T) {
	cmd := &InitInstallConfigCmd{
		Opts: &InitInstallConfigOpts{
			DatacenterID:                      1,
			DatacenterName:                    "test-dc",
			DatacenterCity:                    "Berlin",
			DatacenterCountryCode:             "DE",
			SecretsBaseDir:                    "/root/secrets",
			CephSubnet:                        "10.53.101.0/24",
			CephHosts:                         []CephHostConfig{{Hostname: "ceph-1", IPAddress: "10.53.101.2", IsMaster: true}},
			PostgresMode:                      "external",
			PostgresExternal:                  "postgres.example.com:5432",
			K8sManaged:                        false,
			K8sPodCIDR:                        "100.96.0.0/11",
			K8sServiceCIDR:                    "100.64.0.0/13",
			ClusterGatewayType:                "LoadBalancer",
			ClusterPublicGatewayType:          "LoadBalancer",
			CodesphereDomain:                  "codesphere.example.com",
			CodesphereWorkspaceBaseDomain:     "ws.example.com",
			CodesphereCustomDomainBaseDomain:  "custom.example.com",
			CodesphereDNSServers:              []string{"8.8.8.8"},
			CodesphereWorkspaceImageBomRef:    "workspace-agent-24.04",
			CodesphereHostingPlanCPU:          10,
			CodesphereHostingPlanMemory:       2048,
			CodesphereHostingPlanStorage:      20480,
			CodesphereHostingPlanTempStorage:  1024,
			CodesphereWorkspacePlanName:       "Standard",
			CodesphereWorkspacePlanMaxReplica: 3,
		},
	}

	secrets := &GeneratedSecrets{
		CephSSHPublicKey: "ssh-rsa TEST",
		IngressCACert:    "-----BEGIN CERTIFICATE-----\nTEST\n-----END CERTIFICATE-----",
	}

	config := cmd.buildGen0Config(secrets)

	if config.Postgres.ServerAddress != "postgres.example.com:5432" {
		t.Errorf("Postgres.ServerAddress = %s, want postgres.example.com:5432", config.Postgres.ServerAddress)
	}
	if config.Postgres.Primary != nil {
		t.Error("Postgres.Primary should be nil for external mode")
	}

	if config.Kubernetes.ManagedByCodesphere {
		t.Error("Kubernetes.ManagedByCodesphere should be false")
	}
	if config.Kubernetes.PodCIDR != "100.96.0.0/11" {
		t.Errorf("Kubernetes.PodCIDR = %s, want 100.96.0.0/11", config.Kubernetes.PodCIDR)
	}
}

func TestBuildGen0Vault(t *testing.T) {
	cmd := &InitInstallConfigCmd{
		Opts: &InitInstallConfigOpts{
			PostgresMode:      "install",
			PostgresReplicaIP: "10.50.0.3",
			RegistryServer:    "ghcr.io",
			K8sManaged:        true,
		},
	}

	secrets := &GeneratedSecrets{
		CephSSHPrivateKey:       "-----BEGIN RSA PRIVATE KEY-----\nCEPH\n-----END RSA PRIVATE KEY-----",
		IngressCAKey:            "-----BEGIN RSA PRIVATE KEY-----\nCA\n-----END RSA PRIVATE KEY-----",
		DomainAuthPrivateKey:    "-----BEGIN EC PRIVATE KEY-----\nDOMAIN\n-----END EC PRIVATE KEY-----",
		DomainAuthPublicKey:     "-----BEGIN PUBLIC KEY-----\nDOMAIN-PUB\n-----END PUBLIC KEY-----",
		PostgresAdminPassword:   "admin123",
		PostgresReplicaPassword: "replica123",
		PostgresPrimaryKey:      "-----BEGIN RSA PRIVATE KEY-----\nPG-PRIMARY\n-----END RSA PRIVATE KEY-----",
		PostgresReplicaKey:      "-----BEGIN RSA PRIVATE KEY-----\nPG-REPLICA\n-----END RSA PRIVATE KEY-----",
		PostgresUserPasswords: map[string]string{
			"auth":       "auth-pass",
			"deployment": "deployment-pass",
			"ide":        "ide-pass",
		},
	}

	vault := cmd.buildGen0Vault(secrets)

	expectedSecrets := map[string]bool{
		"cephSshPrivateKey":           false,
		"selfSignedCaKeyPem":          false,
		"domainAuthPrivateKey":        false,
		"domainAuthPublicKey":         false,
		"postgresPassword":            false,
		"postgresReplicaPassword":     false,
		"postgresPrimaryServerKeyPem": false,
		"postgresReplicaServerKeyPem": false,
		"registryUsername":            false,
		"registryPassword":            false,
		"managedServiceSecrets":       false,
	}

	for _, secret := range vault.Secrets {
		if _, exists := expectedSecrets[secret.Name]; exists {
			expectedSecrets[secret.Name] = true
		}
	}

	for name, found := range expectedSecrets {
		if !found && strings.HasPrefix(name, "postgres") {
			t.Errorf("Expected postgres secret %s not found in vault", name)
		}
	}

	foundCephSSH := false
	foundIngressCA := false
	for _, secret := range vault.Secrets {
		if secret.Name == "cephSshPrivateKey" {
			foundCephSSH = true
			if secret.File == nil {
				t.Error("cephSshPrivateKey should have a file")
			} else if secret.File.Name != "id_rsa" {
				t.Errorf("cephSshPrivateKey file name = %s, want id_rsa", secret.File.Name)
			}
		}
		if secret.Name == "selfSignedCaKeyPem" {
			foundIngressCA = true
			if secret.File == nil {
				t.Error("selfSignedCaKeyPem should have a file")
			}
		}
	}

	if !foundCephSSH {
		t.Error("cephSshPrivateKey not found in vault")
	}
	if !foundIngressCA {
		t.Error("selfSignedCaKeyPem not found in vault")
	}
}

func TestBuildGen0VaultExternalK8s(t *testing.T) {
	cmd := &InitInstallConfigCmd{
		Opts: &InitInstallConfigOpts{
			PostgresMode: "external",
			K8sManaged:   false,
		},
	}

	secrets := &GeneratedSecrets{
		CephSSHPrivateKey:    "-----BEGIN RSA PRIVATE KEY-----\nCEPH\n-----END RSA PRIVATE KEY-----",
		IngressCAKey:         "-----BEGIN RSA PRIVATE KEY-----\nCA\n-----END RSA PRIVATE KEY-----",
		DomainAuthPrivateKey: "-----BEGIN EC PRIVATE KEY-----\nDOMAIN\n-----END EC PRIVATE KEY-----",
		DomainAuthPublicKey:  "-----BEGIN PUBLIC KEY-----\nDOMAIN-PUB\n-----END PUBLIC KEY-----",
	}

	vault := cmd.buildGen0Vault(secrets)

	foundKubeConfig := false
	for _, secret := range vault.Secrets {
		if secret.Name == "kubeConfig" {
			foundKubeConfig = true
			if secret.File == nil {
				t.Error("kubeConfig should have a file")
			}
		}
	}

	if !foundKubeConfig {
		t.Error("kubeConfig not found in vault for external Kubernetes")
	}
}

func TestAddConfigComments(t *testing.T) {
	cmd := &InitInstallConfigCmd{}
	yamlData := []byte("test: value\n")

	result := cmd.addConfigComments(yamlData)
	resultStr := string(result)

	if !strings.Contains(resultStr, "Codesphere Gen0 Installer Configuration") {
		t.Error("Config comments should contain header text")
	}
	if !strings.Contains(resultStr, "test: value") {
		t.Error("Config comments should preserve original YAML")
	}
}

func TestAddVaultComments(t *testing.T) {
	cmd := &InitInstallConfigCmd{}
	yamlData := []byte("secrets:\n  - name: test\n")

	result := cmd.addVaultComments(yamlData)
	resultStr := string(result)

	if !strings.Contains(resultStr, "Codesphere Gen0 Installer Secrets") {
		t.Error("Vault comments should contain header text")
	}
	if !strings.Contains(resultStr, "IMPORTANT") {
		t.Error("Vault comments should contain security warning")
	}
	if !strings.Contains(resultStr, "SOPS") {
		t.Error("Vault comments should mention SOPS")
	}
	if !strings.Contains(resultStr, "secrets:") {
		t.Error("Vault comments should preserve original YAML")
	}
}
