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

var _ = Describe("UsesExternalSecrets", func() {
	It("is true when the override enables global.externalSecrets", func() {
		cfg := parseRootConfig("codesphere:\n  override:\n    global:\n      externalSecrets: true\n")
		Expect(installer.UsesExternalSecrets(cfg.Codesphere)).To(BeTrue())
	})

	It("is false when the config has no override", func() {
		Expect(installer.UsesExternalSecrets(files.CodesphereConfig{})).To(BeFalse())
	})

	It("is false when externalSecrets is the string true rather than a boolean", func() {
		cfg := parseRootConfig("codesphere:\n  override:\n    global:\n      externalSecrets: \"true\"\n")
		Expect(installer.UsesExternalSecrets(cfg.Codesphere)).To(BeFalse())
	})
})

var _ = Describe("WriteReleaseVaultSecret", func() {
	var (
		ctx          context.Context
		kubeClient   ctrlclient.Client
		installVault *files.InstallVault
	)

	getSecret := func(name string) *corev1.Secret {
		secret := &corev1.Secret{}
		Expect(kubeClient.Get(ctx, ctrlclient.ObjectKey{Namespace: "codesphere", Name: name}, secret)).To(Succeed())

		return secret
	}

	BeforeEach(func() {
		ctx = context.Background()
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		kubeClient = fake.NewClientBuilder().WithScheme(scheme).Build()

		installVault = &files.InstallVault{Secrets: []files.SecretEntry{
			{Name: "oidcClientSecret", Fields: &files.SecretFields{Password: "oidc-secret"}},
		}}
		Expect(secrets.EnsureAuthKeys(installVault)).To(Succeed())
	})

	It("writes the vault into <release>-vault for the configured release", func() {
		cfg := parseRootConfig("codesphere:\n  releaseName: codesphere-pr-42\n").Codesphere
		_, err := installer.WriteReleaseVaultSecret(ctx, kubeClient, installVault, cfg)
		Expect(err).ToNot(HaveOccurred())

		Expect(getSecret("codesphere-pr-42-vault").Data).To(HaveKeyWithValue("oidcClientSecret.password", []byte("oidc-secret")))
	})

	It("writes codesphere-vault when the config sets no release name", func() {
		_, err := installer.WriteReleaseVaultSecret(ctx, kubeClient, installVault, files.CodesphereConfig{})
		Expect(err).ToNot(HaveOccurred())

		Expect(getSecret("codesphere-vault").Data).To(HaveKey("oidcClientSecret.password"))
	})

	It("signs service account tokens the token public key verifies", func() {
		_, err := installer.WriteReleaseVaultSecret(ctx, kubeClient, installVault, files.CodesphereConfig{})
		Expect(err).ToNot(HaveOccurred())

		publicKey, err := jwt.ParseRSAPublicKeyFromPEM([]byte(installVault.GetSecret(files.SecretTokenPublicKey).File.Content))
		Expect(err).ToNot(HaveOccurred())

		rawToken := getSecret("codesphere-vault").Data["authServiceUserToken.password"]
		claims := jwt.MapClaims{}
		_, err = jwt.ParseWithClaims(string(rawToken), claims, func(*jwt.Token) (any, error) { return publicKey, nil },
			jwt.WithValidMethods([]string{jwt.SigningMethodRS512.Alg()}))
		Expect(err).ToNot(HaveOccurred())
		Expect(claims).To(HaveKeyWithValue("serviceId", "auth-service"))
	})

	It("leaves the secrets of other releases and cs-vault untouched", func() {
		for _, name := range []string{"cs-vault", "codesphere-vault"} {
			Expect(kubeClient.Create(ctx, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "codesphere"},
				Data:       map[string][]byte{"marker": []byte(name)},
			})).To(Succeed())
		}

		cfg := parseRootConfig("codesphere:\n  releaseName: codesphere-pr-42\n").Codesphere
		_, err := installer.WriteReleaseVaultSecret(ctx, kubeClient, installVault, cfg)
		Expect(err).ToNot(HaveOccurred())

		Expect(getSecret("cs-vault").Data).To(Equal(map[string][]byte{"marker": []byte("cs-vault")}))
		Expect(getSecret("codesphere-vault").Data).To(Equal(map[string][]byte{"marker": []byte("codesphere-vault")}))
	})

	It("returns the checksum of the data it wrote, including the signed tokens", func() {
		checksum, err := installer.WriteReleaseVaultSecret(ctx, kubeClient, installVault, files.CodesphereConfig{})
		Expect(err).ToNot(HaveOccurred())

		written := getSecret("codesphere-vault").Data
		Expect(written).To(HaveKey("authServiceUserToken.password"))
		Expect(checksum).To(Equal(checksumOf(written)))
	})

	It("fails without writing a secret when the vault has no token private key", func() {
		installVault.RemoveSecret(files.SecretTokenPrivateKey)

		_, err := installer.WriteReleaseVaultSecret(ctx, kubeClient, installVault, files.CodesphereConfig{})
		Expect(err).To(MatchError(ContainSubstring("tokenPrivateKey not found")))

		secretList := &corev1.SecretList{}
		Expect(kubeClient.List(ctx, secretList)).To(Succeed())
		Expect(secretList.Items).To(BeEmpty())
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
