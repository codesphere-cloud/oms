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

	Context("creating a new Helm OCI repository secret", func() {
		It("creates the secret with all expected fields", func() {
			repoSecret := &installer.ArgoCDRepoSecret{
				Config: installer.ArgoCDRepoSecretConfig{
					Name:       "ghcr-codesphere-helm-repo",
					URL:        "ghcr.io/codesphere-cloud/charts",
					RepoName:   "ghcr-codesphere",
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

			secret, err := fakeClient.CoreV1().Secrets("argocd").Get(ctx, "ghcr-codesphere-helm-repo", metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(secret.Labels).To(HaveKeyWithValue("argocd.argoproj.io/secret-type", "repository"))
			Expect(secret.StringData["type"]).To(Equal("helm"))
			Expect(secret.StringData["url"]).To(Equal("ghcr.io/codesphere-cloud/charts"))
			Expect(secret.StringData["name"]).To(Equal("ghcr-codesphere"))
			Expect(secret.StringData["username"]).To(Equal("CodesphereBot"))
			Expect(secret.StringData["password"]).To(Equal("super-secret-token"))
			Expect(secret.StringData["enableOCI"]).To(Equal("true"))
		})
	})

	Context("creating a git repo-creds secret", func() {
		It("creates the secret with git type and repo-creds label", func() {
			repoSecret := &installer.ArgoCDRepoSecret{
				Config: installer.ArgoCDRepoSecretConfig{
					Name:       "my-git-repo",
					URL:        "https://github.com/my-org",
					RepoName:   "my-org",
					Type:       "git",
					Username:   "bot-user",
					Password:   "git-token",
					EnableOCI:  false,
					SecretType: "repo-creds",
				},
				Clientset: fakeClient,
			}

			err := repoSecret.Apply(ctx)
			Expect(err).ToNot(HaveOccurred())

			secret, err := fakeClient.CoreV1().Secrets("argocd").Get(ctx, "my-git-repo", metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(secret.Labels).To(HaveKeyWithValue("argocd.argoproj.io/secret-type", "repo-creds"))
			Expect(secret.StringData["type"]).To(Equal("git"))
			Expect(secret.StringData["url"]).To(Equal("https://github.com/my-org"))
			Expect(secret.StringData["name"]).To(Equal("my-org"))
			Expect(secret.StringData["username"]).To(Equal("bot-user"))
			Expect(secret.StringData["password"]).To(Equal("git-token"))
			Expect(secret.StringData["enableOCI"]).To(Equal("false"))
		})
	})

	Context("updating an existing secret", func() {
		It("updates the secret when it already exists", func() {
			// Create initial secret
			existing := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ghcr-codesphere-helm-repo",
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
					Name:       "ghcr-codesphere-helm-repo",
					URL:        "ghcr.io/codesphere-cloud/charts",
					RepoName:   "ghcr-codesphere",
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

			secret, err := fakeClient.CoreV1().Secrets("argocd").Get(ctx, "ghcr-codesphere-helm-repo", metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(secret.StringData["password"]).To(Equal("new-password"))
		})
	})
})
