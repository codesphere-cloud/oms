// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package argocd_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	argov1alpha1 "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/argocd"
	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/vault"
	"github.com/codesphere-cloud/oms/internal/installer/vault/sops"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func sopsAndAgeAvailable() bool {
	return exec.Command("sops", "--version").Run() == nil &&
		exec.Command("age-keygen", "--version").Run() == nil
}

type argoCDInstallerStub struct {
	called bool
}

func (s *argoCDInstallerStub) Install() error {
	s.called = true
	return nil
}

var _ = Describe("AppInstaller", func() {
	It("uses the configured ArgoCD installer instance", func() {
		argoCDInstall := &argoCDInstallerStub{}
		install := argocd.NewAppInstaller(argocd.AppInstallerConfig{
			Installer: argoCDInstall,
		})

		Expect(install.InstallArgoCD()).To(Succeed())
		Expect(argoCDInstall.called).To(BeTrue())
	})

	It("configures the pc-applications global image registry", func() {
		scheme := runtime.NewScheme()
		Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
		Expect(argov1alpha1.AddToScheme(scheme)).To(Succeed())
		kubeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "argocd-codesphere-oci-read", Namespace: "argocd"},
			Data:       map[string][]byte{"url": []byte("registry.example.com/mirror/codesphere-cloud/charts")},
		}).Build()
		install := argocd.NewAppInstaller(argocd.AppInstallerConfig{
			Config:     files.RootConfig{Registry: &files.RegistryConfig{Server: "registry.example.com/mirror"}},
			Vault:      &files.InstallVault{},
			KubeClient: kubeClient,
		})
		bomConfig := &bom.Config{Components: map[string]bom.ComponentConfig{
			"pc-applications": {Files: map[string]bom.FileRef{
				"chart": {OciRef: "oci://registry.example.com/mirror/codesphere-cloud/charts/pc-applications:1.2.3"},
			}},
		}}

		Expect(install.InstallPCApps(context.Background(), bomConfig)).To(Succeed())
		app := &argov1alpha1.Application{}
		Expect(kubeClient.Get(context.Background(), client.ObjectKey{Name: "pc-applications", Namespace: "argocd"}, app)).To(Succeed())
		values := map[string]any{}
		Expect(json.Unmarshal(app.Spec.Source.Helm.ValuesObject.Raw, &values)).To(Succeed())
		Expect(values).To(HaveKeyWithValue("global", map[string]any{"imageRegistry": "registry.example.com/mirror"}))
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
