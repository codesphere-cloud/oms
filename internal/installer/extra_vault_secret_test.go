// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"

	"github.com/golang-jwt/jwt/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.yaml.in/yaml/v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/secrets"
)

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

func parseRootConfig(yamlConfig string) files.RootConfig {
	cfg := files.RootConfig{}
	Expect(cfg.Unmarshal([]byte(yamlConfig))).To(Succeed())

	return cfg
}

var _ = Describe("ValidateExtraVaultSecretName", func() {
	It("accepts an empty name, which turns the extra secret off", func() {
		Expect(installer.ValidateExtraVaultSecretName("")).To(Succeed())
	})

	It("accepts a valid Secret name", func() {
		Expect(installer.ValidateExtraVaultSecretName("codesphere-pr-42-vault")).To(Succeed())
	})

	It("rejects a name Kubernetes would refuse for a Secret", func() {
		Expect(installer.ValidateExtraVaultSecretName("Codesphere_PR/42")).To(MatchError(ContainSubstring("invalid extra vault secret name")))
	})
})

var _ = Describe("ChartVaultSecretName", func() {
	It("is cs-vault when no extra vault secret is set", func() {
		Expect(installer.ChartVaultSecretName("")).To(Equal("cs-vault"))
	})

	It("is the extra vault secret when one is set", func() {
		Expect(installer.ChartVaultSecretName("codesphere-pr-42-vault")).To(Equal("codesphere-pr-42-vault"))
	})
})

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

var _ = Describe("WriteExtraVaultSecret", func() {
	const secretName = "codesphere-pr-42-vault"

	It("writes the vault into the given secret", func() {
		kubeClient, installVault := setupExtraVaultSecret()

		Expect(installer.WriteExtraVaultSecret(context.Background(), kubeClient, installVault, secretName)).To(Succeed())

		Expect(getCodesphereSecret(kubeClient, secretName).Data).To(HaveKeyWithValue("oidcClientSecret.password", []byte("oidc-secret")))
	})

	It("signs service account tokens the token public key verifies", func() {
		kubeClient, installVault := setupExtraVaultSecret()

		Expect(installer.WriteExtraVaultSecret(context.Background(), kubeClient, installVault, secretName)).To(Succeed())

		publicKey, err := jwt.ParseRSAPublicKeyFromPEM([]byte(installVault.GetSecret(files.SecretTokenPublicKey).File.Content))
		Expect(err).ToNot(HaveOccurred())

		rawToken := getCodesphereSecret(kubeClient, secretName).Data["authServiceUserToken.password"]
		claims := jwt.MapClaims{}
		_, err = jwt.ParseWithClaims(string(rawToken), claims, func(*jwt.Token) (any, error) { return publicKey, nil },
			jwt.WithValidMethods([]string{jwt.SigningMethodRS512.Alg()}))
		Expect(err).ToNot(HaveOccurred())
		Expect(claims).To(HaveKeyWithValue("serviceId", "auth-service"))
	})

	It("leaves cs-vault and other vault secrets untouched", func() {
		kubeClient, installVault := setupExtraVaultSecret()
		for _, name := range []string{"cs-vault", "codesphere-pr-7-vault"} {
			Expect(kubeClient.Create(context.Background(), &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "codesphere"},
				Data:       map[string][]byte{"marker": []byte(name)},
			})).To(Succeed())
		}

		Expect(installer.WriteExtraVaultSecret(context.Background(), kubeClient, installVault, secretName)).To(Succeed())

		Expect(getCodesphereSecret(kubeClient, "cs-vault").Data).To(Equal(map[string][]byte{"marker": []byte("cs-vault")}))
		Expect(getCodesphereSecret(kubeClient, "codesphere-pr-7-vault").Data).To(Equal(map[string][]byte{"marker": []byte("codesphere-pr-7-vault")}))
	})

	It("fails without writing a secret when the vault has no token private key", func() {
		kubeClient, installVault := setupExtraVaultSecret()
		installVault.RemoveSecret(files.SecretTokenPrivateKey)

		err := installer.WriteExtraVaultSecret(context.Background(), kubeClient, installVault, secretName)
		Expect(err).To(MatchError(ContainSubstring("tokenPrivateKey not found")))

		secretList := &corev1.SecretList{}
		Expect(kubeClient.List(context.Background(), secretList)).To(Succeed())
		Expect(secretList.Items).To(BeEmpty())
	})
})

var _ = Describe("VaultSecretChecksum", func() {
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

		checksum, found, err := installer.VaultSecretChecksum(context.Background(), kubeClient, "cs-vault")
		Expect(err).ToNot(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(checksum).To(Equal(checksumOf(data)))
	})

	It("matches the data WriteExtraVaultSecret wrote, including the signed tokens", func() {
		kubeClient, installVault := setupExtraVaultSecret()
		Expect(installer.WriteExtraVaultSecret(context.Background(), kubeClient, installVault, "codesphere-pr-42-vault")).To(Succeed())

		checksum, _, err := installer.VaultSecretChecksum(context.Background(), kubeClient, "codesphere-pr-42-vault")
		Expect(err).ToNot(HaveOccurred())

		written := getCodesphereSecret(kubeClient, "codesphere-pr-42-vault").Data
		Expect(written).To(HaveKey("authServiceUserToken.password"))
		Expect(checksum).To(Equal(checksumOf(written)))
	})

	It("reports not found when the secret does not exist", func() {
		checksum, found, err := installer.VaultSecretChecksum(context.Background(), newKubeClient(), "cs-vault")
		Expect(err).ToNot(HaveOccurred())
		Expect(found).To(BeFalse())
		Expect(checksum).To(BeEmpty())
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

		Expect(installer.AddExternalSecretsChecksum(path, "abc123")).To(Succeed())

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

		Expect(installer.AddExternalSecretsChecksum(path, "abc123")).To(Succeed())

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

		Expect(installer.AddExternalSecretsChecksum(path, "abc123")).To(Succeed())

		info, err := os.Stat(path)
		Expect(err).ToNot(HaveOccurred())
		Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o600)))
	})
})
