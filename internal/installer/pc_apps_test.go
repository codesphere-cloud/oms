// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	argov1alpha1 "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/codesphere-cloud/oms/internal/installer"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("PCApps.Install", func() {
	const (
		version   = "1.2.3"
		namespace = "argocd"

		// Values matching the K8s secret template from argocd_resources.go
		secretURL      = "ghcr.io/codesphere-cloud/charts"
		secretUsername = "github"
		secretPassword = "super-secret-token"
	)

	var (
		fakeClient client.Client
		pcApps     *installer.PCApps
		scheme     *runtime.Scheme
	)

	newSecret := func() *corev1.Secret {
		return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "argocd-codesphere-oci-read",
				Namespace: "argocd",
			},
			Data: map[string][]byte{
				"url":      []byte(secretURL),
				"username": []byte(secretUsername),
				"password": []byte(secretPassword),
			},
		}
	}

	// getApp reads back the Application the installer applied.
	getApp := func() *argov1alpha1.Application {
		GinkgoHelper()
		app := &argov1alpha1.Application{}
		err := fakeClient.Get(context.Background(), client.ObjectKey{Name: "pc-applications", Namespace: "argocd"}, app)
		Expect(err).ToNot(HaveOccurred())
		return app
	}

	// helmValues decodes spec.source.helm.valuesObject of the applied Application.
	helmValues := func(app *argov1alpha1.Application) map[string]interface{} {
		GinkgoHelper()
		Expect(app.Spec.Source).ToNot(BeNil())
		Expect(app.Spec.Source.Helm).ToNot(BeNil())
		Expect(app.Spec.Source.Helm.ValuesObject).ToNot(BeNil())
		vals := map[string]interface{}{}
		Expect(json.Unmarshal(app.Spec.Source.Helm.ValuesObject.Raw, &vals)).To(Succeed())
		return vals
	}

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
		Expect(argov1alpha1.AddToScheme(scheme)).To(Succeed())
	})

	Context("successful apply (secret exists)", func() {
		BeforeEach(func() {
			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(newSecret()).
				Build()
			pcApps = installer.NewPCAppsForTesting(fakeClient, version, namespace, nil, nil, false)
		})

		It("creates an app-of-apps Application referencing the chart from the secret's registry", func() {
			Expect(pcApps.Install(context.Background())).To(Succeed())

			app := getApp()
			Expect(app.Name).To(Equal("pc-applications"))
			Expect(app.Namespace).To(Equal("argocd"))
			Expect(app.Spec.Project).To(Equal("default"))

			Expect(app.Spec.Source).ToNot(BeNil())
			Expect(app.Spec.Source.RepoURL).To(Equal(secretURL))
			Expect(app.Spec.Source.Chart).To(Equal("pc-applications"))
			Expect(app.Spec.Source.TargetRevision).To(Equal(version))
			Expect(app.Spec.Source.Helm.ReleaseName).To(Equal("pc-applications"))

			Expect(app.Spec.Destination.Server).To(Equal("https://kubernetes.default.svc"))
			Expect(app.Spec.Destination.Namespace).To(Equal(namespace))

			Expect(app.Spec.SyncPolicy).ToNot(BeNil())
			Expect(app.Spec.SyncPolicy.Automated).ToNot(BeNil())
			Expect(app.Spec.SyncPolicy.Automated.GetPrune()).To(BeTrue())
			Expect([]string(app.Spec.SyncPolicy.SyncOptions)).To(ConsistOf("CreateNamespace=true", "ServerSideApply=true"))
		})

		It("deploys into a custom destination namespace", func() {
			pcApps = installer.NewPCAppsForTesting(fakeClient, version, "platform", nil, nil, false)
			Expect(pcApps.Install(context.Background())).To(Succeed())

			app := getApp()
			// The Application itself always lives in the argocd namespace.
			Expect(app.Namespace).To(Equal("argocd"))
			Expect(app.Spec.Destination.Namespace).To(Equal("platform"))
		})

		It("updates an existing Application to a new chart version", func() {
			Expect(pcApps.Install(context.Background())).To(Succeed())

			upgraded := installer.NewPCAppsForTesting(fakeClient, "2.0.0", namespace, nil, nil, false)
			Expect(upgraded.Install(context.Background())).To(Succeed())

			Expect(getApp().Spec.Source.TargetRevision).To(Equal("2.0.0"))
		})
	})

	Context("client scheme without the ArgoCD types", func() {
		It("fails to construct the installer with a clear error", func() {
			bareScheme := runtime.NewScheme()
			Expect(clientgoscheme.AddToScheme(bareScheme)).To(Succeed())
			bareClient := fake.NewClientBuilder().WithScheme(bareScheme).Build()

			_, err := installer.NewPCApps(bareClient, version, namespace, nil, nil, false)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("does not recognize"))
			Expect(err.Error()).To(ContainSubstring("Application"))
		})
	})

	Context("no K8s secret", func() {
		BeforeEach(func() {
			// No objects in the fake client
			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				Build()
			pcApps = installer.NewPCAppsForTesting(fakeClient, version, namespace, nil, nil, false)
		})

		It("returns a clear error", func() {
			err := pcApps.Install(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("argocd-codesphere-oci-read"))
			Expect(err.Error()).To(ContainSubstring("oms beta install argocd"))
		})
	})

	Context("K8s secret exists but missing the registry url", func() {
		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "argocd-codesphere-oci-read",
					Namespace: "argocd",
				},
				Data: map[string][]byte{
					"username": []byte(secretUsername),
					"password": []byte(secretPassword),
					// url is missing
				},
			}
			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(secret).
				Build()
			pcApps = installer.NewPCAppsForTesting(fakeClient, version, namespace, nil, nil, false)
		})

		It("returns an error about the missing field", func() {
			err := pcApps.Install(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing required field"))
		})
	})

	Context("values files", func() {
		var tmpDir string

		BeforeEach(func() {
			var err error
			tmpDir, err = os.MkdirTemp("", "pc-apps-test-*")
			Expect(err).ToNot(HaveOccurred())

			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(newSecret()).
				Build()
		})

		AfterEach(func() {
			Expect(os.RemoveAll(tmpDir)).To(Succeed())
		})

		It("merges multiple values files into the Application's valuesObject", func() {
			base := filepath.Join(tmpDir, "base.yaml")
			Expect(os.WriteFile(base, []byte("foo: bar\nnested:\n  a: 1\n  b: 2\n"), 0644)).To(Succeed())

			overlay := filepath.Join(tmpDir, "overlay.yaml")
			Expect(os.WriteFile(overlay, []byte("foo: overridden\nnested:\n  b: 99\n  c: 3\n"), 0644)).To(Succeed())

			pcApps = installer.NewPCAppsForTesting(fakeClient, version, namespace, []string{base, overlay}, nil, false)
			Expect(pcApps.Install(context.Background())).To(Succeed())

			vals := helmValues(getApp())
			Expect(vals).To(HaveKeyWithValue("foo", "overridden"))
			nested, ok := vals["nested"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(nested["a"]).To(BeNumerically("==", 1))
			Expect(nested["b"]).To(BeNumerically("==", 99))
			Expect(nested["c"]).To(BeNumerically("==", 3))
		})

		It("returns an error for non-existent values file", func() {
			pcApps = installer.NewPCAppsForTesting(fakeClient, version, namespace, []string{"/nonexistent/values.yaml"}, nil, false)

			err := pcApps.Install(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("loading values files"))
		})

		It("returns an error for invalid YAML in values file", func() {
			badFile := filepath.Join(tmpDir, "bad.yaml")
			Expect(os.WriteFile(badFile, []byte("{{invalid yaml"), 0644)).To(Succeed())

			pcApps = installer.NewPCAppsForTesting(fakeClient, version, namespace, []string{badFile}, nil, false)

			err := pcApps.Install(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("loading values files"))
		})

		It("merges inline config overrides before values files", func() {
			override := map[string]interface{}{
				"foo": "from-config",
				"nested": map[string]interface{}{
					"configOnly": true,
					"shared":     "from-config",
				},
			}
			fileValues := filepath.Join(tmpDir, "values.yaml")
			Expect(os.WriteFile(fileValues, []byte("foo: from-file\nnested:\n  shared: from-file\n  fileOnly: true\n"), 0644)).To(Succeed())

			pcApps = installer.NewPCAppsForTesting(fakeClient, version, namespace, []string{fileValues}, override, false)
			Expect(pcApps.Install(context.Background())).To(Succeed())

			vals := helmValues(getApp())
			Expect(vals).To(HaveKeyWithValue("foo", "from-file"))
			nested, ok := vals["nested"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(nested["configOnly"]).To(BeTrue())
			Expect(nested["shared"]).To(Equal("from-file"))
			Expect(nested["fileOnly"]).To(BeTrue())
		})

		It("omits valuesObject when no values are configured", func() {
			pcApps = installer.NewPCAppsForTesting(fakeClient, version, namespace, nil, nil, false)
			Expect(pcApps.Install(context.Background())).To(Succeed())

			app := getApp()
			Expect(app.Spec.Source.Helm).ToNot(BeNil())
			Expect(app.Spec.Source.Helm.ValuesObject).To(BeNil())
		})
	})

	Context("ForceConflicts", func() {
		BeforeEach(func() {
			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(newSecret()).
				Build()
		})

		It("applies the Application with forced field ownership", func() {
			pcApps = installer.NewPCAppsForTesting(fakeClient, version, namespace, nil, nil, true)
			Expect(pcApps.Install(context.Background())).To(Succeed())
			Expect(getApp().Name).To(Equal("pc-applications"))
		})
	})
})
