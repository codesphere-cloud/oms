// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	"context"

	"github.com/golang-jwt/jwt/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/secrets"
	"github.com/codesphere-cloud/oms/internal/installer/vault"
)

func parseRootConfig(yamlConfig string) files.RootConfig {
	cfg := files.RootConfig{}
	Expect(cfg.Unmarshal([]byte(yamlConfig))).To(Succeed())

	return cfg
}

var _ = Describe("ReleaseName", func() {
	It("defaults to codesphere when the config sets no release name", func() {
		Expect(installer.ReleaseName(files.CodesphereConfig{})).To(Equal("codesphere"))
	})

	It("uses the release name from the config", func() {
		cfg := parseRootConfig("codesphere:\n  releaseName: codesphere-pr-42\n")
		Expect(installer.ReleaseName(cfg.Codesphere)).To(Equal("codesphere-pr-42"))
	})
})

var _ = Describe("UsesExternalSecrets", func() {
	It("is true when the override enables global.externalSecrets", func() {
		cfg := parseRootConfig("codesphere:\n  override:\n    global:\n      externalSecrets: true\n")
		Expect(installer.UsesExternalSecrets(cfg.Codesphere)).To(BeTrue())
	})

	It("is false when the override disables global.externalSecrets", func() {
		cfg := parseRootConfig("codesphere:\n  override:\n    global:\n      externalSecrets: false\n")
		Expect(installer.UsesExternalSecrets(cfg.Codesphere)).To(BeFalse())
	})

	It("is false when the override sets other global values only", func() {
		cfg := parseRootConfig("codesphere:\n  override:\n    global:\n      env: prod\n")
		Expect(installer.UsesExternalSecrets(cfg.Codesphere)).To(BeFalse())
	})

	It("is false when the config has no override", func() {
		Expect(installer.UsesExternalSecrets(files.CodesphereConfig{})).To(BeFalse())
	})

	It("is false when externalSecrets is the string true rather than a boolean", func() {
		cfg := parseRootConfig("codesphere:\n  override:\n    global:\n      externalSecrets: \"true\"\n")
		Expect(installer.UsesExternalSecrets(cfg.Codesphere)).To(BeFalse())
	})
})

var _ = Describe("EnsureReleaseVaultSecret", func() {
	It("is a no-op when the chart does not use external secrets", func() {
		// No vault path or secrets baseDir: loading the vault would fail,
		// so a nil result proves the opt-in check short-circuits first.
		err := installer.EnsureReleaseVaultSecret(context.Background(), "", "", string(vault.TypeSOPS), files.RootConfig{})
		Expect(err).ToNot(HaveOccurred())
	})

	It("propagates vault loading errors when the chart uses external secrets", func() {
		cfg := parseRootConfig("codesphere:\n  override:\n    global:\n      externalSecrets: true\n")
		err := installer.EnsureReleaseVaultSecret(context.Background(), "", "", string(vault.TypeSOPS), cfg)
		Expect(err).To(MatchError(ContainSubstring("vault path is not set")))
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

	It("writes the vault into <release>-vault labelled with the release", func() {
		Expect(installer.WriteReleaseVaultSecret(ctx, kubeClient, installVault, "codesphere-pr-42")).To(Succeed())

		secret := getSecret("codesphere-pr-42-vault")
		Expect(secret.Labels).To(HaveKeyWithValue("codesphere.com/release", "codesphere-pr-42"))
		Expect(secret.Data).To(HaveKeyWithValue("oidcClientSecret.password", []byte("oidc-secret")))
	})

	It("signs service account tokens the token public key verifies", func() {
		Expect(installer.WriteReleaseVaultSecret(ctx, kubeClient, installVault, "codesphere")).To(Succeed())

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

		Expect(installer.WriteReleaseVaultSecret(ctx, kubeClient, installVault, "codesphere-pr-42")).To(Succeed())

		Expect(getSecret("cs-vault").Data).To(Equal(map[string][]byte{"marker": []byte("cs-vault")}))
		Expect(getSecret("codesphere-vault").Data).To(Equal(map[string][]byte{"marker": []byte("codesphere-vault")}))
	})

	It("fails without writing a secret when the vault has no token private key", func() {
		installVault.RemoveSecret(files.SecretTokenPrivateKey)

		err := installer.WriteReleaseVaultSecret(ctx, kubeClient, installVault, "codesphere")
		Expect(err).To(MatchError(ContainSubstring("tokenPrivateKey not found")))

		secretList := &corev1.SecretList{}
		Expect(kubeClient.List(ctx, secretList)).To(Succeed())
		Expect(secretList.Items).To(BeEmpty())
	})
})
