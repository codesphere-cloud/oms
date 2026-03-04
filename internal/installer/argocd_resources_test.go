// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	sigyaml "sigs.k8s.io/yaml"
)

var _ = Describe("Embedded ArgoCD YAML manifests", func() {
	Describe("appProjectsYAML", func() {
		It("is not empty", func() {
			Expect(appProjectsYAML).ToNot(BeEmpty())
		})

		It("decodes into three AppProject objects", func() {
			objects, err := decodeMultiDocYAML(appProjectsYAML)
			Expect(err).ToNot(HaveOccurred())
			Expect(objects).To(HaveLen(3))
		})

		It("contains the prod project with correct metadata", func() {
			objects, err := decodeMultiDocYAML(appProjectsYAML)
			Expect(err).ToNot(HaveOccurred())

			prod := objects[0]
			Expect(prod.GetName()).To(Equal("prod"))
			Expect(prod.GetNamespace()).To(Equal("argocd"))
			Expect(prod.GetKind()).To(Equal("AppProject"))
			Expect(prod.GetAPIVersion()).To(Equal("argoproj.io/v1alpha1"))
			Expect(prod.GetFinalizers()).To(ContainElement("resources-finalizer.argocd.argoproj.io"))
		})

		It("contains the dev project with correct metadata", func() {
			objects, err := decodeMultiDocYAML(appProjectsYAML)
			Expect(err).ToNot(HaveOccurred())

			dev := objects[1]
			Expect(dev.GetName()).To(Equal("dev"))
			Expect(dev.GetNamespace()).To(Equal("argocd"))
			Expect(dev.GetFinalizers()).To(ContainElement("resources-finalizer.argocd.argoproj.io"))
		})

		It("contains the default project with restricted spec", func() {
			objects, err := decodeMultiDocYAML(appProjectsYAML)
			Expect(err).ToNot(HaveOccurred())

			def := objects[2]
			Expect(def.GetName()).To(Equal("default"))
			Expect(def.GetNamespace()).To(Equal("argocd"))

			spec, ok := def.Object["spec"].(map[string]interface{})
			Expect(ok).To(BeTrue())

			sourceRepos, ok := spec["sourceRepos"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(sourceRepos).To(BeEmpty())

			destinations, ok := spec["destinations"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(destinations).To(BeEmpty())
		})

		It("maps all projects to a valid GVR", func() {
			objects, err := decodeMultiDocYAML(appProjectsYAML)
			Expect(err).ToNot(HaveOccurred())

			for _, obj := range objects {
				gvr, err := gvrForUnstructured(obj)
				Expect(err).ToNot(HaveOccurred())
				Expect(gvr.Resource).To(Equal("appprojects"))
			}
		})
	})

	Describe("localClusterTpl", func() {
		It("is not empty", func() {
			Expect(localClusterTpl).ToNot(BeEmpty())
		})

		It("renders and parses into a valid Secret", func() {
			rendered, err := renderTemplate(localClusterTpl, map[string]string{
				"DC_NUMBER": "3",
			})
			Expect(err).ToNot(HaveOccurred())

			secret := &corev1.Secret{}
			err = sigyaml.Unmarshal(rendered, secret)
			Expect(err).ToNot(HaveOccurred())

			Expect(secret.Name).To(Equal("argocd-cluster-dc-3"))
			Expect(secret.Namespace).To(Equal("argocd"))
			Expect(secret.Labels).To(HaveKeyWithValue("argocd.argoproj.io/secret-type", "cluster"))
			Expect(secret.StringData).To(HaveKeyWithValue("name", "dc-3"))
			Expect(secret.StringData).To(HaveKeyWithValue("server", "https://kubernetes.default.svc"))
			Expect(secret.StringData).To(HaveKey("config"))
		})

		It("substitutes the DC_NUMBER in all required fields", func() {
			rendered, err := renderTemplate(localClusterTpl, map[string]string{
				"DC_NUMBER": "7",
			})
			Expect(err).ToNot(HaveOccurred())

			secret := &corev1.Secret{}
			err = sigyaml.Unmarshal(rendered, secret)
			Expect(err).ToNot(HaveOccurred())

			Expect(secret.Name).To(Equal("argocd-cluster-dc-7"))
			Expect(secret.StringData["name"]).To(Equal("dc-7"))
		})
	})

	Describe("helmRegistryTpl", func() {
		It("is not empty", func() {
			Expect(helmRegistryTpl).ToNot(BeEmpty())
		})

		It("renders and parses into a valid OCI repository Secret", func() {
			rendered, err := renderTemplate(helmRegistryTpl, map[string]string{
				"SECRET_CODESPHERE_OCI_READ": "ghp_testtoken123",
			})
			Expect(err).ToNot(HaveOccurred())

			secret := &corev1.Secret{}
			err = sigyaml.Unmarshal(rendered, secret)
			Expect(err).ToNot(HaveOccurred())

			Expect(secret.Name).To(Equal("argocd-codesphere-oci-read"))
			Expect(secret.Namespace).To(Equal("argocd"))
			Expect(secret.Labels).To(HaveKeyWithValue("argocd.argoproj.io/secret-type", "repository"))
			Expect(secret.StringData).To(HaveKeyWithValue("type", "helm"))
			Expect(secret.StringData).To(HaveKeyWithValue("url", "ghcr.io/codesphere-cloud/charts"))
			Expect(secret.StringData).To(HaveKeyWithValue("username", "github"))
			Expect(secret.StringData).To(HaveKeyWithValue("password", "ghp_testtoken123"))
			Expect(secret.StringData).To(HaveKeyWithValue("enableOCI", "true"))
		})
	})

	Describe("gitRepoTpl", func() {
		It("is not empty", func() {
			Expect(gitRepoTpl).ToNot(BeEmpty())
		})

		It("renders and parses into a valid repo-creds Secret", func() {
			rendered, err := renderTemplate(gitRepoTpl, map[string]string{
				"SECRET_CODESPHERE_REPOS_READ": "ghp_repotoken456",
			})
			Expect(err).ToNot(HaveOccurred())

			secret := &corev1.Secret{}
			err = sigyaml.Unmarshal(rendered, secret)
			Expect(err).ToNot(HaveOccurred())

			Expect(secret.Name).To(Equal("argocd-codesphere-repos-read"))
			Expect(secret.Namespace).To(Equal("argocd"))
			Expect(secret.Labels).To(HaveKeyWithValue("argocd.argoproj.io/secret-type", "repo-creds"))
			Expect(secret.StringData).To(HaveKeyWithValue("type", "git"))
			Expect(secret.StringData).To(HaveKeyWithValue("url", "https://github.com/codesphere-cloud"))
			Expect(secret.StringData).To(HaveKeyWithValue("username", "github"))
			Expect(secret.StringData).To(HaveKeyWithValue("password", "ghp_repotoken456"))
		})
	})
})
