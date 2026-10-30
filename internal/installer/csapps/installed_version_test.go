// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package csapps_test

import (
	"context"

	argov1alpha1 "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/codesphere-cloud/oms/internal/installer/csapps"
)

var _ = Describe("InstalledVersion", func() {
	clientWith := func(objects ...client.Object) client.Client {
		scheme := runtime.NewScheme()
		Expect(argov1alpha1.AddToScheme(scheme)).To(Succeed())

		return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
	}

	deployed := func(name string, revisions ...string) *argov1alpha1.Application {
		app := &argov1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "argocd"}}
		for i, revision := range revisions {
			app.Status.History = append(app.Status.History, argov1alpha1.RevisionHistory{ID: int64(i), Revision: revision})
		}

		return app
	}

	It("last deployed chart 1.113.0 with an image tag -> 1.113.0, the version migrations compare to", func() {
		app := deployed("pr-1-codesphere", "1.112.0-old-tag-1234567890", "1.113.0-nate-test-pre-7a1cd4d4-ca735854c8")

		version, err := csapps.InstalledVersion(context.Background(), clientWith(app), fullConfig("pr-1"))

		Expect(err).NotTo(HaveOccurred())
		Expect(version).To(Equal("1.113.0"))
	})

	It("Application of another release -> nothing installed for this one", func() {
		app := deployed("codesphere-codesphere", "1.113.0-tag")

		version, err := csapps.InstalledVersion(context.Background(), clientWith(app), fullConfig("pr-1"))

		Expect(err).NotTo(HaveOccurred())
		Expect(version).To(BeEmpty())
	})

	It("Application not synced yet -> nothing installed", func() {
		version, err := csapps.InstalledVersion(context.Background(), clientWith(deployed("pr-1-codesphere")), fullConfig("pr-1"))

		Expect(err).NotTo(HaveOccurred())
		Expect(version).To(BeEmpty())
	})

	It("deployed revision is not a semantic version -> error, not a guess", func() {
		app := deployed("pr-1-codesphere", "main")

		_, err := csapps.InstalledVersion(context.Background(), clientWith(app), fullConfig("pr-1"))

		Expect(err).To(MatchError(ContainSubstring(`chart version "main"`)))
	})
})
