// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	"context"

	"github.com/codesphere-cloud/oms/internal/installer"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// NOTE: Assertions use secret.StringData because the fake client does not perform
// the server-side StringData -> Data (base64) conversion that real Kubernetes does.
var _ = Describe("ArgoCDRepoSecret.Apply", func() {

	var (
		fakeClient *fake.Clientset
		ctx        context.Context
	)

	BeforeEach(func() {
		fakeClient = fake.NewSimpleClientset()
		ctx = context.Background()

		// Ensure argocd namespace exists
		_, err := fakeClient.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "argocd"},
		}, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())
	})

	Context("creating the codesphere-helm-repo secret", func() {
		It("creates the secret with all expected fields", func() {
			repoSecret := &installer.ArgoCDRepoSecret{
				Config: installer.ArgoCDRepoSecretConfig{
					Name:       "codesphere-helm-repo",
					URL:        "ghcr.io/codesphere-cloud/charts",
					RepoName:   "codesphere-helm-repo",
					Type:       "helm",
					Username:   "CodesphereBot",
					Password:   "super-secret-token",
					EnableOCI:  true,
					SecretType: "repository",
				},
				Clientset: fakeClient,
			}

			err := repoSecret.Apply(ctx)
			Expect(err).ToNot(HaveOccurred())

			secret, err := fakeClient.CoreV1().Secrets("argocd").Get(ctx, "codesphere-helm-repo", metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(secret.Labels).To(HaveKeyWithValue("argocd.argoproj.io/secret-type", "repository"))
			Expect(secret.StringData["type"]).To(Equal("helm"))
			Expect(secret.StringData["url"]).To(Equal("ghcr.io/codesphere-cloud/charts"))
			Expect(secret.StringData["name"]).To(Equal("codesphere-helm-repo"))
			Expect(secret.StringData["username"]).To(Equal("CodesphereBot"))
			Expect(secret.StringData["password"]).To(Equal("super-secret-token"))
			Expect(secret.StringData["enableOCI"]).To(Equal("true"))
		})
	})

	Context("using a mirrored registry URL", func() {
		It("creates the secret with the custom URL", func() {
			repoSecret := &installer.ArgoCDRepoSecret{
				Config: installer.ArgoCDRepoSecretConfig{
					Name:       "codesphere-helm-repo",
					URL:        "my-mirror.example.com/charts",
					RepoName:   "codesphere-helm-repo",
					Type:       "helm",
					Username:   "CodesphereBot",
					Password:   "mirror-token",
					EnableOCI:  true,
					SecretType: "repository",
				},
				Clientset: fakeClient,
			}

			err := repoSecret.Apply(ctx)
			Expect(err).ToNot(HaveOccurred())

			secret, err := fakeClient.CoreV1().Secrets("argocd").Get(ctx, "codesphere-helm-repo", metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(secret.StringData["url"]).To(Equal("my-mirror.example.com/charts"))
			Expect(secret.StringData["username"]).To(Equal("CodesphereBot"))
		})
	})

	Context("updating an existing secret", func() {
		It("updates the secret when it already exists", func() {
			// Create initial secret
			existing := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "codesphere-helm-repo",
					Namespace: "argocd",
					Labels: map[string]string{
						"argocd.argoproj.io/secret-type": "repository",
					},
				},
				StringData: map[string]string{
					"password": "old-password",
				},
			}
			_, err := fakeClient.CoreV1().Secrets("argocd").Create(ctx, existing, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			repoSecret := &installer.ArgoCDRepoSecret{
				Config: installer.ArgoCDRepoSecretConfig{
					Name:       "codesphere-helm-repo",
					URL:        "ghcr.io/codesphere-cloud/charts",
					RepoName:   "codesphere-helm-repo",
					Type:       "helm",
					Username:   "CodesphereBot",
					Password:   "new-password",
					EnableOCI:  true,
					SecretType: "repository",
				},
				Clientset: fakeClient,
			}

			err = repoSecret.Apply(ctx)
			Expect(err).ToNot(HaveOccurred())

			secret, err := fakeClient.CoreV1().Secrets("argocd").Get(ctx, "codesphere-helm-repo", metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(secret.StringData["password"]).To(Equal("new-password"))
		})
	})
})
