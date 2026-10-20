// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package vault

import (
	"context"
	"os"
	"path/filepath"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/secrets"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.yaml.in/yaml/v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("VaultSecretCreator", func() {
	var (
		ctx        context.Context
		kubeClient ctrlclient.Client
		creator    *VaultSecretCreator
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		kubeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
		creator = NewVaultSecretCreator(kubeClient)
	})

	It("creates the target namespace before syncing the vault secret", func() {
		installVault := &files.InstallVault{Secrets: []files.SecretEntry{
			{Name: "registry", Fields: &files.SecretFields{Password: "first"}},
		}}
		Expect(secrets.EnsureAuthKeys(installVault)).To(Succeed())

		Expect(creator.CreateSecretFromVault(ctx, installVault, VaultSecretNamespace, VaultSecretName)).To(Succeed())

		namespace := &corev1.Namespace{}
		Expect(kubeClient.Get(ctx, ctrlclient.ObjectKey{Name: VaultSecretNamespace}, namespace)).To(Succeed())

		secret := &corev1.Secret{}
		Expect(kubeClient.Get(ctx, ctrlclient.ObjectKey{Namespace: VaultSecretNamespace, Name: VaultSecretName}, secret)).To(Succeed())
		Expect(secret.Data).To(HaveKeyWithValue("registry.password", []byte("first")))
	})

	It("keeps namespace creation idempotent when updating the vault secret", func() {
		namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: VaultSecretNamespace}}
		Expect(kubeClient.Create(ctx, namespace)).To(Succeed())

		installVault := &files.InstallVault{Secrets: []files.SecretEntry{
			{Name: "registry", Fields: &files.SecretFields{Password: "first"}},
		}}
		Expect(secrets.EnsureAuthKeys(installVault)).To(Succeed())
		Expect(creator.CreateSecretFromVault(ctx, installVault, VaultSecretNamespace, VaultSecretName)).To(Succeed())

		installVault.SetSecret(files.SecretEntry{Name: "registry", Fields: &files.SecretFields{Password: "updated"}})
		Expect(creator.CreateSecretFromVault(ctx, installVault, VaultSecretNamespace, VaultSecretName)).To(Succeed())

		secret := &corev1.Secret{}
		Expect(kubeClient.Get(ctx, ctrlclient.ObjectKey{Namespace: VaultSecretNamespace, Name: VaultSecretName}, secret)).To(Succeed())
		Expect(secret.Data).To(HaveKeyWithValue("registry.password", []byte("updated")))
	})

	It("writes the signed service account tokens into the secret", func() {
		installVault := &files.InstallVault{}
		Expect(secrets.EnsureAuthKeys(installVault)).To(Succeed())

		Expect(creator.CreateSecretFromVault(ctx, installVault, VaultSecretNamespace, VaultSecretName)).To(Succeed())

		secret := &corev1.Secret{}
		Expect(kubeClient.Get(ctx, ctrlclient.ObjectKey{Namespace: VaultSecretNamespace, Name: VaultSecretName}, secret)).To(Succeed())
		Expect(secret.Data["authServiceUserToken.password"]).ToNot(BeEmpty())
	})

	It("fails without writing a secret when the vault has no token private key", func() {
		installVault := &files.InstallVault{Secrets: []files.SecretEntry{
			{Name: "registry", Fields: &files.SecretFields{Password: "first"}},
		}}

		err := creator.CreateSecretFromVault(ctx, installVault, VaultSecretNamespace, VaultSecretName)
		Expect(err).To(MatchError(ContainSubstring("tokenPrivateKey not found")))

		secretList := &corev1.SecretList{}
		Expect(kubeClient.List(ctx, secretList)).To(Succeed())
		Expect(secretList.Items).To(BeEmpty())
	})
})

var _ = Describe("vaultToSecretData", func() {
	It("stores file entry content under the entry name", func() {
		vault := &files.InstallVault{
			Secrets: []files.SecretEntry{
				{Name: "tlsCert", File: &files.SecretFile{Name: "tls.crt", Content: "cert-content"}},
			},
		}
		data, err := vaultToSecretData(vault)
		Expect(err).ToNot(HaveOccurred())
		Expect(data).To(HaveKeyWithValue("tlsCert", []byte("cert-content")))
	})

	It("stores field entry with password only under entryName/password", func() {
		vault := &files.InstallVault{
			Secrets: []files.SecretEntry{
				{Name: "dbAdmin", Fields: &files.SecretFields{Password: "s3cr3t"}},
			},
		}
		data, err := vaultToSecretData(vault)
		Expect(err).ToNot(HaveOccurred())
		Expect(data).To(HaveKeyWithValue("dbAdmin.password", []byte("s3cr3t")))
		Expect(data).NotTo(HaveKey("dbAdmin.username"))
	})

	It("stores field entry with username and password under entryName/username and entryName/password", func() {
		vault := &files.InstallVault{
			Secrets: []files.SecretEntry{
				{Name: "registry", Fields: &files.SecretFields{Username: "robot", Password: "token123"}},
			},
		}
		data, err := vaultToSecretData(vault)
		Expect(err).ToNot(HaveOccurred())
		Expect(data).To(HaveKeyWithValue("registry.username", []byte("robot")))
		Expect(data).To(HaveKeyWithValue("registry.password", []byte("token123")))
	})

	It("handles a mix of file and field entries", func() {
		vault := &files.InstallVault{
			Secrets: []files.SecretEntry{
				{Name: "sshKey", File: &files.SecretFile{Name: "id_rsa", Content: "-----BEGIN RSA PRIVATE KEY-----"}},
				{Name: "git", Fields: &files.SecretFields{Username: "deploy", Password: "gh_token"}},
			},
		}
		data, err := vaultToSecretData(vault)
		Expect(err).ToNot(HaveOccurred())
		Expect(data).To(HaveKey("sshKey"))
		Expect(data).To(HaveKey("git.username"))
		Expect(data).To(HaveKey("git.password"))
		Expect(data).To(HaveLen(3))
	})

	It("returns an error for an empty vault", func() {
		vault := &files.InstallVault{}
		_, err := vaultToSecretData(vault)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no secrets found"))
	})

	It("skips entries that have neither file nor fields", func() {
		vault := &files.InstallVault{
			Secrets: []files.SecretEntry{
				{Name: "empty"},
				{Name: "real", Fields: &files.SecretFields{Password: "pw"}},
			},
		}
		data, err := vaultToSecretData(vault)
		Expect(err).ToNot(HaveOccurred())
		Expect(data).NotTo(HaveKey("empty"))
		Expect(data).To(HaveKey("real.password"))
	})
})

var _ = Describe("SecretChecksum", func() {
	checksumOf := func(objects ...ctrlclient.Object) (string, error) {
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		kubeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()

		return NewVaultSecretCreator(kubeClient).SecretChecksum(context.Background(), VaultSecretNamespace, VaultSecretName)
	}

	checksumOfData := func(data map[string][]byte) string {
		checksum, err := checksumOf(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: VaultSecretName, Namespace: VaultSecretNamespace},
			Data:       data,
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(checksum).ToNot(BeEmpty())

		return checksum
	}

	It("changes when a value changes", func() {
		Expect(checksumOfData(map[string][]byte{"a": []byte("1")})).ToNot(Equal(checksumOfData(map[string][]byte{"a": []byte("2")})))
	})

	It("tells apart data that only differs in where a key ends and its value starts", func() {
		Expect(checksumOfData(map[string][]byte{"ab": []byte("c")})).ToNot(Equal(checksumOfData(map[string][]byte{"a": []byte("bc")})))
	})

	It("fails when the secret does not exist", func() {
		_, err := checksumOf()
		Expect(err).To(MatchError(ContainSubstring(`vault secret "cs-vault" not found`)))
	})
})

var _ = Describe("ValidateSecretName", func() {
	It("accepts a valid Secret name", func() {
		Expect(ValidateSecretName("codesphere-pr-42-vault")).To(Succeed())
	})
})

var _ = Describe("WriteVaultChecksumToConfig", func() {
	writeConfig := func(content string) string {
		path := filepath.Join(GinkgoT().TempDir(), "config.yaml")
		Expect(os.WriteFile(path, []byte(content), 0o600)).To(Succeed())

		return path
	}

	readConfig := func(path string) map[string]any {
		data, err := os.ReadFile(path)
		Expect(err).ToNot(HaveOccurred())

		config := map[string]any{}
		Expect(yaml.Unmarshal(data, &config)).To(Succeed())

		return config
	}

	It("sets the checksum next to the existing global overrides", func() {
		path := writeConfig("codesphere:\n  domain: example.com\n  override:\n    global:\n      hostName: pr-42\n      externalSecrets: true\n")

		Expect(WriteVaultChecksumToConfig(path, "abc123")).To(Succeed())

		Expect(readConfig(path)).To(Equal(map[string]any{
			"codesphere": map[string]any{
				"domain": "example.com",
				"override": map[string]any{
					"global": map[string]any{
						"hostName":                "pr-42",
						"externalSecrets":         true,
						"externalSecretsChecksum": "abc123",
					},
				},
			},
		}))
	})

	It("creates the override path when the config has none", func() {
		path := writeConfig("postgres:\n  mode: install\n")

		Expect(WriteVaultChecksumToConfig(path, "abc123")).To(Succeed())

		Expect(readConfig(path)).To(Equal(map[string]any{
			"postgres": map[string]any{"mode": "install"},
			"codesphere": map[string]any{
				"override": map[string]any{
					"global": map[string]any{"externalSecretsChecksum": "abc123"},
				},
			},
		}))
	})
})
