// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package vault

import (
	"context"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/secrets"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type tokenVaultStore struct {
	data  *files.InstallVault
	saves int
}

func (s *tokenVaultStore) Load() (*files.InstallVault, error)         { return s.data.Clone(), nil }
func (s *tokenVaultStore) LoadOrCreate() (*files.InstallVault, error) { return s.Load() }
func (s *tokenVaultStore) Save(data *files.InstallVault) error {
	s.data = data.Clone()
	s.saves++

	return nil
}

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
		Expect(creator.CreateSecretFromVault(ctx, installVault, VaultSecretNamespace, VaultSecretName)).To(Succeed())

		installVault.SetSecret(files.SecretEntry{Name: "registry", Fields: &files.SecretFields{Password: "updated"}})
		Expect(creator.CreateSecretFromVault(ctx, installVault, VaultSecretNamespace, VaultSecretName)).To(Succeed())

		secret := &corev1.Secret{}
		Expect(kubeClient.Get(ctx, ctrlclient.ObjectKey{Namespace: VaultSecretNamespace, Name: VaultSecretName}, secret)).To(Succeed())
		Expect(secret.Data).To(HaveKeyWithValue("registry.password", []byte("updated")))
	})

	It("renews service tokens on every sync", func() {
		store := &tokenVaultStore{data: &files.InstallVault{}}
		Expect(secrets.EnsureAuthKeys(store.data)).To(Succeed())
		Expect(creator.CreateSecretFromStore(ctx, store, VaultSecretNamespace, VaultSecretName)).To(Succeed())

		secret := &corev1.Secret{}
		key := ctrlclient.ObjectKey{Namespace: VaultSecretNamespace, Name: VaultSecretName}
		Expect(kubeClient.Get(ctx, key, secret)).To(Succeed())
		original := secret.Data["authServiceUserToken.password"]
		Expect(original).NotTo(BeEmpty())

		Expect(creator.CreateSecretFromStore(ctx, store, VaultSecretNamespace, VaultSecretName)).To(Succeed())
		Expect(kubeClient.Get(ctx, key, secret)).To(Succeed())
		Expect(secret.Data["authServiceUserToken.password"]).NotTo(Equal(original))
		Expect(store.saves).To(Equal(0))
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
