// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package argocd_test

import (
	"context"

	"github.com/codesphere-cloud/oms/internal/installer/argocd"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

var _ = Describe("ArgoCDResources", func() {
	registrySecret := func(username string) map[string]string {
		clientset := fake.NewSimpleClientset()

		resources, err := argocd.NewArgoCDResources(clientset, "", username, "registry-password", "10.10.0.7:5000/codesphere-cloud/charts", "")
		Expect(err).NotTo(HaveOccurred())
		Expect(resources.ApplyAll(context.Background())).To(Succeed())

		secret, err := clientset.CoreV1().Secrets("argocd").Get(context.Background(), "argocd-codesphere-oci-read", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		return secret.StringData
	}

	It("gives ArgoCD the credentials of the registry the charts were mirrored into", func() {
		secret := registrySecret("custom-registry")

		Expect(secret["username"]).To(Equal("custom-registry"))
		Expect(secret["password"]).To(Equal("registry-password"))
		Expect(secret["url"]).To(Equal("10.10.0.7:5000/codesphere-cloud/charts"))
	})

	It("falls back to the GHCR user when no username is configured", func() {
		Expect(registrySecret("")["username"]).To(Equal("github"))
	})
})
