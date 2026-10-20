// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package vault

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/secrets"
	"github.com/golang-jwt/jwt/v5"
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

var _ = Describe("secretDataChecksum", func() {
	It("does not depend on the order the keys were added in", func() {
		first := map[string][]byte{}
		first["a"] = []byte("1")
		first["b"] = []byte("2")
		second := map[string][]byte{}
		second["b"] = []byte("2")
		second["a"] = []byte("1")

		Expect(secretDataChecksum(first)).To(Equal(secretDataChecksum(second)))
	})

	It("changes when a value changes", func() {
		Expect(secretDataChecksum(map[string][]byte{"a": []byte("1")})).ToNot(Equal(secretDataChecksum(map[string][]byte{"a": []byte("2")})))
	})

	It("tells apart data that only differs in where a key ends and its value starts", func() {
		Expect(secretDataChecksum(map[string][]byte{"ab": []byte("c")})).ToNot(Equal(secretDataChecksum(map[string][]byte{"a": []byte("bc")})))
	})
})

// checksumOf recomputes the documented checksum format independently of the code under test.
func checksumOf(data map[string][]byte) string {
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	hash := sha256.New()
	for _, key := range keys {
		hash.Write([]byte(key))
		hash.Write([]byte{0})
		hash.Write(data[key])
		hash.Write([]byte{0})
	}

	return hex.EncodeToString(hash.Sum(nil))
}

func setupExtraVaultSecret() (ctrlclient.Client, *files.InstallVault) {
	scheme := runtime.NewScheme()
	Expect(corev1.AddToScheme(scheme)).To(Succeed())
	kubeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	installVault := &files.InstallVault{Secrets: []files.SecretEntry{
		{Name: "oidcClientSecret", Fields: &files.SecretFields{Password: "oidc-secret"}},
	}}
	Expect(secrets.EnsureAuthKeys(installVault)).To(Succeed())

	return kubeClient, installVault
}

func getCodesphereSecret(kubeClient ctrlclient.Client, name string) *corev1.Secret {
	secret := &corev1.Secret{}
	Expect(kubeClient.Get(context.Background(), ctrlclient.ObjectKey{Namespace: "codesphere", Name: name}, secret)).To(Succeed())

	return secret
}

var _ = Describe("CreateSecretFromVault", func() {
	const secretName = "codesphere-pr-42-vault"

	It("writes the vault into the given secret", func() {
		kubeClient, installVault := setupExtraVaultSecret()

		Expect(NewVaultSecretCreator(kubeClient).CreateSecretFromVault(context.Background(), installVault, VaultSecretNamespace, secretName)).To(Succeed())

		Expect(getCodesphereSecret(kubeClient, secretName).Data).To(HaveKeyWithValue("oidcClientSecret.password", []byte("oidc-secret")))
	})

	It("signs service account tokens the token public key verifies", func() {
		kubeClient, installVault := setupExtraVaultSecret()

		Expect(NewVaultSecretCreator(kubeClient).CreateSecretFromVault(context.Background(), installVault, VaultSecretNamespace, secretName)).To(Succeed())

		publicKey, err := jwt.ParseRSAPublicKeyFromPEM([]byte(installVault.GetSecret(files.SecretTokenPublicKey).File.Content))
		Expect(err).ToNot(HaveOccurred())

		rawToken := getCodesphereSecret(kubeClient, secretName).Data["authServiceUserToken.password"]
		claims := jwt.MapClaims{}
		_, err = jwt.ParseWithClaims(string(rawToken), claims, func(*jwt.Token) (any, error) { return publicKey, nil },
			jwt.WithValidMethods([]string{jwt.SigningMethodRS512.Alg()}))
		Expect(err).ToNot(HaveOccurred())
		Expect(claims).To(HaveKeyWithValue("serviceId", "auth-service"))
	})

	It("leaves other vault secrets untouched", func() {
		kubeClient, installVault := setupExtraVaultSecret()
		for _, name := range []string{"cs-vault", "codesphere-pr-7-vault"} {
			Expect(kubeClient.Create(context.Background(), &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "codesphere"},
				Data:       map[string][]byte{"marker": []byte(name)},
			})).To(Succeed())
		}

		Expect(NewVaultSecretCreator(kubeClient).CreateSecretFromVault(context.Background(), installVault, VaultSecretNamespace, secretName)).To(Succeed())

		Expect(getCodesphereSecret(kubeClient, "cs-vault").Data).To(Equal(map[string][]byte{"marker": []byte("cs-vault")}))
		Expect(getCodesphereSecret(kubeClient, "codesphere-pr-7-vault").Data).To(Equal(map[string][]byte{"marker": []byte("codesphere-pr-7-vault")}))
	})

	It("fails without writing a secret when the vault has no token private key", func() {
		kubeClient, installVault := setupExtraVaultSecret()
		installVault.RemoveSecret(files.SecretTokenPrivateKey)

		err := NewVaultSecretCreator(kubeClient).CreateSecretFromVault(context.Background(), installVault, VaultSecretNamespace, secretName)
		Expect(err).To(MatchError(ContainSubstring("tokenPrivateKey not found")))

		secretList := &corev1.SecretList{}
		Expect(kubeClient.List(context.Background(), secretList)).To(Succeed())
		Expect(secretList.Items).To(BeEmpty())
	})
})

var _ = Describe("SecretChecksum", func() {
	newKubeClient := func(objects ...ctrlclient.Object) ctrlclient.Client {
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
	}

	It("returns the checksum of the data stored in the cluster", func() {
		data := map[string][]byte{"tokenPublicKey": []byte("written-by-the-dependencies-phase")}
		kubeClient := newKubeClient(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "cs-vault", Namespace: "codesphere"},
			Data:       data,
		})

		checksum, err := NewVaultSecretCreator(kubeClient).SecretChecksum(context.Background(), VaultSecretNamespace, "cs-vault")
		Expect(err).ToNot(HaveOccurred())
		Expect(checksum).To(Equal(checksumOf(data)))
	})

	It("matches the data CreateSecretFromVault wrote, including the signed tokens", func() {
		kubeClient, installVault := setupExtraVaultSecret()
		Expect(NewVaultSecretCreator(kubeClient).CreateSecretFromVault(context.Background(), installVault, VaultSecretNamespace, "codesphere-pr-42-vault")).To(Succeed())

		checksum, err := NewVaultSecretCreator(kubeClient).SecretChecksum(context.Background(), VaultSecretNamespace, "codesphere-pr-42-vault")
		Expect(err).ToNot(HaveOccurred())

		written := getCodesphereSecret(kubeClient, "codesphere-pr-42-vault").Data
		Expect(written).To(HaveKey("authServiceUserToken.password"))
		Expect(checksum).To(Equal(checksumOf(written)))
	})

	It("fails when the secret does not exist", func() {
		_, err := NewVaultSecretCreator(newKubeClient()).SecretChecksum(context.Background(), VaultSecretNamespace, "cs-vault")
		Expect(err).To(MatchError(ContainSubstring(`vault secret "cs-vault" not found`)))
	})
})

var _ = Describe("ValidateSecretName", func() {
	It("accepts a valid Secret name", func() {
		Expect(ValidateSecretName("codesphere-pr-42-vault")).To(Succeed())
	})

	It("rejects a name Kubernetes would refuse for a Secret", func() {
		Expect(ValidateSecretName("Codesphere_PR/42")).To(MatchError(ContainSubstring(`invalid secret name "Codesphere_PR/42"`)))
	})
})

var _ = Describe("AddExternalSecretsChecksum", func() {
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

		Expect(AddExternalSecretsChecksum(path, "abc123")).To(Succeed())

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

		Expect(AddExternalSecretsChecksum(path, "abc123")).To(Succeed())

		Expect(readConfig(path)).To(Equal(map[string]any{
			"postgres": map[string]any{"mode": "install"},
			"codesphere": map[string]any{
				"override": map[string]any{
					"global": map[string]any{"externalSecretsChecksum": "abc123"},
				},
			},
		}))
	})

	It("keeps the config readable only by its owner", func() {
		path := writeConfig("codesphere: {}\n")

		Expect(AddExternalSecretsChecksum(path, "abc123")).To(Succeed())

		info, err := os.Stat(path)
		Expect(err).ToNot(HaveOccurred())
		Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o600)))
	})
})
