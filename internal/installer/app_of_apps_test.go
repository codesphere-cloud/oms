// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	argov1alpha1 "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/bom"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("AppOfApps.Install", func() {
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
		pcApps     *installer.AppOfApps
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

	bomWithChart := func(component, ociRef string) *bom.Config {
		return &bom.Config{Components: map[string]bom.ComponentConfig{
			component: {Files: map[string]bom.FileRef{"chart": {OciRef: ociRef}}},
		}}
	}

	newPcApps := func(chartVersion, destination string, valuesFiles []string, valuesOverride map[string]interface{}) *installer.AppOfApps {
		GinkgoHelper()

		b := bomWithChart("pc-applications", "ghcr.io/codesphere-cloud/charts/pc-applications:"+chartVersion)
		apps, err := installer.NewAppOfAppsFromBom(fakeClient, b, installer.AppOfAppsConfig{
			Component:      "pc-applications",
			Name:           "pc-applications",
			ReleaseName:    "pc-applications",
			Namespace:      destination,
			ValuesFiles:    valuesFiles,
			ValuesOverride: valuesOverride,
		})
		Expect(err).ToNot(HaveOccurred())

		return apps
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
			pcApps = newPcApps(version, namespace, nil, nil)
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
			pcApps = newPcApps(version, "platform", nil, nil)
			Expect(pcApps.Install(context.Background())).To(Succeed())

			app := getApp()
			// The Application itself always lives in the argocd namespace.
			Expect(app.Namespace).To(Equal("argocd"))
			Expect(app.Spec.Destination.Namespace).To(Equal("platform"))
		})

		It("updates an existing Application to a new chart version", func() {
			Expect(pcApps.Install(context.Background())).To(Succeed())

			upgraded := newPcApps("2.0.0", namespace, nil, nil)
			Expect(upgraded.Install(context.Background())).To(Succeed())

			Expect(getApp().Spec.Source.TargetRevision).To(Equal("2.0.0"))
		})
	})

	Context("client scheme without the ArgoCD types", func() {
		It("fails to construct the installer with a clear error", func() {
			bareScheme := runtime.NewScheme()
			Expect(clientgoscheme.AddToScheme(bareScheme)).To(Succeed())
			bareClient := fake.NewClientBuilder().WithScheme(bareScheme).Build()

			b := bomWithChart("pc-applications", "ghcr.io/codesphere-cloud/charts/pc-applications:"+version)
			_, err := installer.NewAppOfAppsFromBom(bareClient, b, installer.AppOfAppsConfig{
				Component:   "pc-applications",
				Name:        "pc-applications",
				ReleaseName: "pc-applications",
				Namespace:   namespace,
			})
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
			pcApps = newPcApps(version, namespace, nil, nil)
		})

		It("returns a clear error", func() {
			err := pcApps.Install(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("argocd-codesphere-oci-read"))
			Expect(err.Error()).To(ContainSubstring("oms install codesphere dependencies"))
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
			pcApps = newPcApps(version, namespace, nil, nil)
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

			pcApps = newPcApps(version, namespace, []string{base, overlay}, nil)
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
			pcApps = newPcApps(version, namespace, []string{"/nonexistent/values.yaml"}, nil)

			err := pcApps.Install(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("loading values files"))
		})

		It("returns an error for invalid YAML in values file", func() {
			badFile := filepath.Join(tmpDir, "bad.yaml")
			Expect(os.WriteFile(badFile, []byte("{{invalid yaml"), 0644)).To(Succeed())

			pcApps = newPcApps(version, namespace, []string{badFile}, nil)

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

			pcApps = newPcApps(version, namespace, []string{fileValues}, override)
			Expect(pcApps.Install(context.Background())).To(Succeed())

			vals := helmValues(getApp())
			Expect(vals).To(HaveKeyWithValue("foo", "from-file"))
			nested, ok := vals["nested"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(nested["configOnly"]).To(BeTrue())
			Expect(nested["shared"]).To(Equal("from-file"))
			Expect(nested["fileOnly"]).To(BeTrue())
		})

		It("defaults child chart sources to the configured OCI repository", func() {
			pcApps = newPcApps(version, namespace, nil, nil)
			Expect(pcApps.Install(context.Background())).To(Succeed())

			app := getApp()
			Expect(app.Spec.Source.Helm).ToNot(BeNil())
			Expect(helmValues(app)).To(HaveKeyWithValue("chartsRegistry", secretURL))
		})

		It("preserves an explicit child chart registry override", func() {
			pcApps = newPcApps(version, namespace, nil, map[string]interface{}{"chartsRegistry": "other.example.com/charts"})
			Expect(pcApps.Install(context.Background())).To(Succeed())
			Expect(helmValues(getApp())).To(HaveKeyWithValue("chartsRegistry", "other.example.com/charts"))
		})
	})

	Context("chart from a BOM component", func() {
		BeforeEach(func() {
			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(newSecret()).
				Build()
		})

		It("applies the component's chart and version under the given Application and release name", func() {
			b := bomWithChart("cs-applications", "ghcr.io/codesphere-cloud/charts/cs-applications:0.3.0")
			apps, err := installer.NewAppOfAppsFromBom(fakeClient, b, installer.AppOfAppsConfig{
				Component:   "cs-applications",
				Name:        "pr-1-cs-applications",
				ReleaseName: "pr-1",
				Namespace:   "codesphere",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(apps.Install(context.Background())).To(Succeed())

			app := &argov1alpha1.Application{}
			Expect(fakeClient.Get(context.Background(), client.ObjectKey{Name: "pr-1-cs-applications", Namespace: "argocd"}, app)).To(Succeed())
			Expect(app.Spec.Source.Chart).To(Equal("cs-applications"))
			Expect(app.Spec.Source.TargetRevision).To(Equal("0.3.0"))
			Expect(app.Spec.Source.Helm.ReleaseName).To(Equal("pr-1"))
			Expect(app.Spec.Destination.Namespace).To(Equal("codesphere"))
		})

		It("release name longer than Helm allows -> error", func() {
			b := bomWithChart("cs-applications", "ghcr.io/codesphere-cloud/charts/cs-applications:0.3.0")
			_, err := installer.NewAppOfAppsFromBom(fakeClient, b, installer.AppOfAppsConfig{
				Component:   "cs-applications",
				Name:        "cs-applications",
				ReleaseName: strings.Repeat("a", 54),
				Namespace:   "codesphere",
			})
			Expect(err).To(MatchError(ContainSubstring("release name")))
		})

		It("release name Helm rejects -> error", func() {
			b := bomWithChart("cs-applications", "ghcr.io/codesphere-cloud/charts/cs-applications:0.3.0")
			_, err := installer.NewAppOfAppsFromBom(fakeClient, b, installer.AppOfAppsConfig{
				Component:   "cs-applications",
				Name:        "cs-applications",
				ReleaseName: "PR_1",
				Namespace:   "codesphere",
			})
			Expect(err).To(MatchError(ContainSubstring(`release name "PR_1"`)))
		})

		It("empty release name -> error", func() {
			b := bomWithChart("cs-applications", "ghcr.io/codesphere-cloud/charts/cs-applications:0.3.0")
			_, err := installer.NewAppOfAppsFromBom(fakeClient, b, installer.AppOfAppsConfig{
				Component:   "cs-applications",
				Name:        "cs-applications",
				ReleaseName: "",
				Namespace:   "codesphere",
			})
			Expect(err).To(MatchError(ContainSubstring("release name")))
		})

		It("takes the chart name from the reference, not the component", func() {
			b := bomWithChart("apps", "ghcr.io/codesphere-cloud/charts/cs-applications:0.3.0")
			apps, err := installer.NewAppOfAppsFromBom(fakeClient, b, installer.AppOfAppsConfig{
				Component:   "apps",
				Name:        "apps",
				ReleaseName: "apps",
				Namespace:   "codesphere",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(apps.Install(context.Background())).To(Succeed())

			app := &argov1alpha1.Application{}
			Expect(fakeClient.Get(context.Background(), client.ObjectKey{Name: "apps", Namespace: "argocd"}, app)).To(Succeed())
			Expect(app.Spec.Source.Chart).To(Equal("cs-applications"))
		})

		It("fails when the BOM has no chart for the component", func() {
			b := bomWithChart("pc-applications", "ghcr.io/codesphere-cloud/charts/pc-applications:1.0.0")
			_, err := installer.NewAppOfAppsFromBom(fakeClient, b, installer.AppOfAppsConfig{
				Component:   "cs-applications",
				Name:        "cs-applications",
				ReleaseName: "codesphere",
				Namespace:   "codesphere",
			})
			Expect(err).To(MatchError(ContainSubstring("cs-applications chart not found in BOM")))
		})

		It("fails without a BOM", func() {
			_, err := installer.NewAppOfAppsFromBom(fakeClient, nil, installer.AppOfAppsConfig{
				Component:   "cs-applications",
				Name:        "cs-applications",
				ReleaseName: "codesphere",
				Namespace:   "codesphere",
			})
			Expect(err).To(MatchError(ContainSubstring("BOM is required")))
		})

		It("fails when the BOM chart has no version", func() {
			b := bomWithChart("cs-applications", "ghcr.io/codesphere-cloud/charts/cs-applications")
			_, err := installer.NewAppOfAppsFromBom(fakeClient, b, installer.AppOfAppsConfig{
				Component:   "cs-applications",
				Name:        "cs-applications",
				ReleaseName: "codesphere",
				Namespace:   "codesphere",
			})
			Expect(err).To(MatchError(ContainSubstring("cs-applications chart not found in BOM")))
		})

		It("fails without an Application name", func() {
			b := bomWithChart("cs-applications", "ghcr.io/codesphere-cloud/charts/cs-applications:0.3.0")
			_, err := installer.NewAppOfAppsFromBom(fakeClient, b, installer.AppOfAppsConfig{
				Component:   "cs-applications",
				Name:        "",
				ReleaseName: "codesphere",
				Namespace:   "codesphere",
			})
			Expect(err).To(MatchError(ContainSubstring("name is required")))
		})

		It("fails without a destination namespace", func() {
			b := bomWithChart("cs-applications", "ghcr.io/codesphere-cloud/charts/cs-applications:0.3.0")
			_, err := installer.NewAppOfAppsFromBom(fakeClient, b, installer.AppOfAppsConfig{
				Component:   "cs-applications",
				Name:        "cs-applications",
				ReleaseName: "codesphere",
				Namespace:   "",
			})
			Expect(err).To(MatchError(ContainSubstring("namespace is required")))
		})

		It("two Applications from the same chart -> each applied with its own name and release", func() {
			b := bomWithChart("cs-applications", "ghcr.io/codesphere-cloud/charts/cs-applications:0.3.0")
			for _, release := range []string{"pr-1", "pr-2"} {
				apps, err := installer.NewAppOfAppsFromBom(fakeClient, b, installer.AppOfAppsConfig{
					Component:   "cs-applications",
					Name:        release + "-cs-applications",
					ReleaseName: release,
					Namespace:   "codesphere",
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(apps.Install(context.Background())).To(Succeed())
			}

			for _, release := range []string{"pr-1", "pr-2"} {
				app := &argov1alpha1.Application{}
				Expect(fakeClient.Get(context.Background(), client.ObjectKey{Name: release + "-cs-applications", Namespace: "argocd"}, app)).To(Succeed())
				Expect(app.Spec.Source.Helm.ReleaseName).To(Equal(release))
			}
		})
	})
})
