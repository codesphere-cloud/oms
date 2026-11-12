// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package argocd_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/argocd"
	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/vault"
	"github.com/codesphere-cloud/oms/internal/installer/vault/sops"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// prePullBOM is a BOM with the images of the Codesphere values and a pc-applications chart that has
// the image-pre-pull application.
func prePullBOM() *bom.Config {
	const prefix = "ghcr.io/codesphere-cloud/codesphere-monorepo/"

	images := map[string]string{
		"nginx":                 "ghcr.io/codesphere-cloud/docker/nginx:1.31.5",
		"frontendGatewayNginx":  "ghcr.io/codesphere-cloud/docker/nginx-gateway:1.31.5",
		"opensearch":            "ghcr.io/codesphere-cloud/docker/opensearch:3.8.0",
		"opensearchInit":        "ghcr.io/codesphere-cloud/docker/busybox:1.37.0",
		"kubectl":               "ghcr.io/codesphere-cloud/docker/kubectl:1.34.0",
		"observabilityOtel":     "ghcr.io/codesphere-cloud/docker/otel:0.160.0",
		"observabilityOtelInit": "ghcr.io/codesphere-cloud/docker/curl:8.20.0",
	}
	for _, name := range []string{
		"auth-service", "deployment-service", "error-page-server", "ide-frontend", "ide-service",
		"marketplace", "payment-service", "public-api-service", "team-service", "workspace-proxy",
		"workspace-service", "workspace-agent-24.04", "workspace-server", "vpn",
	} {
		images[name] = prefix + name + ":v1.2.3"
	}

	return &bom.Config{Components: map[string]bom.ComponentConfig{
		"codesphere": {ContainerImages: images},
		"pc-applications": {Files: map[string]bom.FileRef{
			"chart": {OciRef: "ghcr.io/codesphere-cloud/charts/pc-applications:0.442.0"},
		}},
	}}
}

func prePullInstaller(pcApps files.ChartValues) *argocd.AppInstaller {
	config := files.NewRootConfig()
	config.Datacenter = files.DatacenterConfig{ID: 1, Name: "main", City: "Karlsruhe", CountryCode: "DE"}
	config.Postgres = files.PostgresConfig{Mode: "external", ServerAddress: "postgres.example.com"}
	config.Kubernetes = files.KubernetesConfig{PodCIDR: "10.1.0.0/16", ServiceCIDR: "10.2.0.0/16"}
	config.Codesphere = files.CodesphereConfig{
		Domain:                     "codesphere.example.com",
		WorkspaceHostingBaseDomain: "ws.example.com",
		PublicIP:                   "203.0.113.10",
		CustomDomains:              files.CustomDomainsConfig{CNameBaseDomain: "custom.example.com"},
		Plans: files.PlansConfig{
			HostingPlans:   map[int]files.HostingPlan{8: {CPUTenth: 10, MemoryMb: 2048, StorageMb: 20480, TempStorageMb: 1500}},
			WorkspacePlans: map[int]files.WorkspacePlan{},
		},
	}
	config.PcApps = pcApps

	vault := &files.InstallVault{Secrets: []files.SecretEntry{{
		Name:   files.SecretRegistryPassword,
		Fields: &files.SecretFields{Password: "secret"},
	}}}

	return argocd.NewAppInstaller(argocd.AppInstallerConfig{Config: config, Vault: vault})
}

func sopsAndAgeAvailable() bool {
	return exec.Command("sops", "--version").Run() == nil &&
		exec.Command("age-keygen", "--version").Run() == nil
}

var _ = Describe("AppInstaller", func() {
	It("passes the context to the configured ArgoCD installer instance", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		argoCDInstall := argocd.NewMockInstallerAPI(GinkgoT())
		argoCDInstall.EXPECT().Install(ctx).Return(nil).Once()
		install := argocd.NewAppInstaller(argocd.AppInstallerConfig{
			Installer: argoCDInstall,
		})

		Expect(install.InstallArgoCD(ctx)).To(Succeed())
	})
})

var _ = Describe("getPcAppsValues", func() {
	It("enables the image-pre-pull app when the BOM has it", func() {
		values, err := prePullInstaller(nil).GetPcAppsValues(prePullBOM())

		Expect(err).NotTo(HaveOccurred())
		Expect(values["applications"]).To(HaveKeyWithValue("image-pre-pull", HaveKeyWithValue("enabled", true)))
	})

	It("lets the pcApps block disable the image-pre-pull app", func() {
		pcApps := files.ChartValues{
			"applications": files.ChartValues{"image-pre-pull": files.ChartValues{"enabled": false}},
		}

		values, err := prePullInstaller(pcApps).GetPcAppsValues(prePullBOM())

		Expect(err).NotTo(HaveOccurred())
		Expect(values["applications"]).To(HaveKeyWithValue("image-pre-pull", Equal(files.ChartValues{"enabled": false})))
	})
})

var _ = Describe("VaultAndRESTConfig", func() {
	It("falls back to config secrets.baseDir when the vault path is not set", func() {
		if !sopsAndAgeAvailable() {
			Skip("sops and age-keygen not available")
		}

		tmpDir := GinkgoT().TempDir()
		secretsDir := filepath.Join(tmpDir, "secrets")
		Expect(os.MkdirAll(secretsDir, 0700)).To(Succeed())

		installVault := &files.InstallVault{
			Secrets: []files.SecretEntry{
				{
					Name: files.SecretRegistryPassword,
					Fields: &files.SecretFields{
						Password: "registry-password",
					},
				},
				{
					Name: files.SecretKubeConfig,
					File: &files.SecretFile{
						Name: "kubeconfig",
						Content: `apiVersion: v1
kind: Config
clusters:
- name: test
  cluster:
    server: https://127.0.0.1:6443
contexts:
- name: test
  context:
    cluster: test
    user: test
current-context: test
users:
- name: test
  user:
    token: test-token
`,
					},
				},
			},
		}
		vaultYAML, err := installVault.Marshal()
		Expect(err).ToNot(HaveOccurred())

		plaintextVaultPath := filepath.Join(secretsDir, "prod.vault.plain.yaml")
		Expect(os.WriteFile(plaintextVaultPath, vaultYAML, 0600)).To(Succeed())

		ageKeyPath := filepath.Join(tmpDir, "age_key.txt")
		Expect(exec.Command("age-keygen", "-o", ageKeyPath).Run()).To(Succeed())
		recipient, err := exec.Command("age-keygen", "-y", ageKeyPath).Output()
		Expect(err).ToNot(HaveOccurred())

		vaultPath := filepath.Join(secretsDir, "prod.vault.yaml")
		Expect(sops.EncryptFile(plaintextVaultPath, vaultPath, strings.TrimSpace(string(recipient)))).To(Succeed())

		config := files.RootConfig{
			Secrets: files.SecretsConfig{
				BaseDir: secretsDir,
			},
		}

		loadedVault, restConfig, err := installer.VaultAndRESTConfig("", ageKeyPath, string(vault.TypeSOPS), config)
		Expect(err).ToNot(HaveOccurred())
		Expect(loadedVault).ToNot(BeNil())
		Expect(restConfig).ToNot(BeNil())
	})
})
