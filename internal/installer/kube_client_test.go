// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	argov1alpha1 "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"

	"github.com/codesphere-cloud/oms/internal/installer"
)

var _ = Describe("NewKubeClient", func() {
	It("creates a client that knows core and ArgoCD types", func() {
		kubeClient, err := installer.NewKubeClient(&rest.Config{Host: "https://127.0.0.1:6443"})
		Expect(err).ToNot(HaveOccurred())

		Expect(kubeClient.Scheme().Recognizes(corev1.SchemeGroupVersion.WithKind("Secret"))).To(BeTrue())
		Expect(kubeClient.Scheme().Recognizes(argov1alpha1.SchemeGroupVersion.WithKind("Application"))).To(BeTrue())
	})
})
