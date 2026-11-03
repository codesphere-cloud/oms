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
		component   = "apps"
		chart       = "app-chart"
		version     = "1.2.3"
		appName     = "my-apps"
		releaseName = "my-release"
		destination = "platform"

		// Values matching the K8s secret template from argocd_resources.go
		secretURL      = "ghcr.io/codesphere-cloud/charts"
		secretUsername = "github"
		secretPassword = "super-secret-token"
	)

	var (
		fakeClient client.Client
		apps       *installer.AppOfApps
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

	getApp := func(name string) *argov1alpha1.Application {
		GinkgoHelper()

		app := &argov1alpha1.Application{}
		Expect(fakeClient.Get(context.Background(), client.ObjectKey{Name: name, Namespace: "argocd"}, app)).To(Succeed())

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

	bomWithChart := func(chartVersion string) *bom.Config {
		ociRef := "ghcr.io/codesphere-cloud/charts/" + chart
		if chartVersion != "" {
			ociRef += ":" + chartVersion
		}

		return &bom.Config{Components: map[string]bom.ComponentConfig{
			component: {Files: map[string]bom.FileRef{"chart": {OciRef: ociRef}}},
		}}
	}

	appConfig := func() installer.AppOfAppsConfig {
		return installer.AppOfAppsConfig{
			Component:   component,
			Name:        appName,
			ReleaseName: releaseName,
			Namespace:   destination,
		}
	}

	newAppOfApps := func(chartVersion string, cfg installer.AppOfAppsConfig) *installer.AppOfApps {
		GinkgoHelper()

		apps, err := installer.NewAppOfAppsFromBom(fakeClient, bomWithChart(chartVersion), cfg)
		Expect(err).ToNot(HaveOccurred())

		return apps
	}

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
		Expect(argov1alpha1.AddToScheme(scheme)).To(Succeed())
		fakeClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(newSecret()).
			Build()
	})

	Context("successful apply (secret exists)", func() {
		BeforeEach(func() {
			apps = newAppOfApps(version, appConfig())
		})

		It("creates the Application from the BOM chart, the secret's registry and the given names", func() {
			Expect(apps.Install(context.Background())).To(Succeed())

			app := getApp(appName)
			Expect(app.Spec.Project).To(Equal("default"))

			Expect(app.Spec.Source).ToNot(BeNil())
			Expect(app.Spec.Source.RepoURL).To(Equal(secretURL))
			Expect(app.Spec.Source.Chart).To(Equal(chart))
			Expect(app.Spec.Source.TargetRevision).To(Equal(version))
			Expect(app.Spec.Source.Helm.ReleaseName).To(Equal(releaseName))

			Expect(app.Spec.Destination.Server).To(Equal("https://kubernetes.default.svc"))

			Expect(app.Spec.SyncPolicy).ToNot(BeNil())
			Expect(app.Spec.SyncPolicy.Automated).ToNot(BeNil())
			Expect(app.Spec.SyncPolicy.Automated.GetPrune()).To(BeTrue())
			Expect([]string(app.Spec.SyncPolicy.SyncOptions)).To(ConsistOf("CreateNamespace=true", "ServerSideApply=true"))
		})

		It("deploys into the given destination namespace, the Application itself stays in argocd", func() {
			Expect(apps.Install(context.Background())).To(Succeed())

			app := getApp(appName)
			Expect(app.Namespace).To(Equal("argocd"))
			Expect(app.Spec.Destination.Namespace).To(Equal(destination))
		})

		It("updates an existing Application to a new chart version", func() {
			Expect(apps.Install(context.Background())).To(Succeed())

			upgraded := newAppOfApps("2.0.0", appConfig())
			Expect(upgraded.Install(context.Background())).To(Succeed())

			Expect(getApp(appName).Spec.Source.TargetRevision).To(Equal("2.0.0"))
		})

		It("two Applications from the same chart -> each applied with its own name and release", func() {
			for _, release := range []string{"pr-1", "pr-2"} {
				cfg := appConfig()
				cfg.Name = release + "-apps"
				cfg.ReleaseName = release
				Expect(newAppOfApps(version, cfg).Install(context.Background())).To(Succeed())
			}

			for _, release := range []string{"pr-1", "pr-2"} {
				Expect(getApp(release + "-apps").Spec.Source.Helm.ReleaseName).To(Equal(release))
			}
		})
	})

	Context("invalid input", func() {
		It("client scheme without the ArgoCD types -> error", func() {
			bareScheme := runtime.NewScheme()
			Expect(clientgoscheme.AddToScheme(bareScheme)).To(Succeed())
			bareClient := fake.NewClientBuilder().WithScheme(bareScheme).Build()

			_, err := installer.NewAppOfAppsFromBom(bareClient, bomWithChart(version), appConfig())
			Expect(err).To(MatchError(ContainSubstring("does not recognize")))
		})

		It("no Application name -> error", func() {
			cfg := appConfig()
			cfg.Name = ""
			_, err := installer.NewAppOfAppsFromBom(fakeClient, bomWithChart(version), cfg)
			Expect(err).To(MatchError(ContainSubstring("name is required")))
		})

		It("no destination namespace -> error", func() {
			cfg := appConfig()
			cfg.Namespace = ""
			_, err := installer.NewAppOfAppsFromBom(fakeClient, bomWithChart(version), cfg)
			Expect(err).To(MatchError(ContainSubstring("namespace is required")))
		})

		It("no release name -> error", func() {
			cfg := appConfig()
			cfg.ReleaseName = ""
			_, err := installer.NewAppOfAppsFromBom(fakeClient, bomWithChart(version), cfg)
			Expect(err).To(MatchError(ContainSubstring("release name")))
		})

		It("release name Helm rejects -> error", func() {
			cfg := appConfig()
			cfg.ReleaseName = "PR_1"
			_, err := installer.NewAppOfAppsFromBom(fakeClient, bomWithChart(version), cfg)
			Expect(err).To(MatchError(ContainSubstring(`release name "PR_1"`)))
		})

		It("release name longer than Helm allows -> error", func() {
			cfg := appConfig()
			cfg.ReleaseName = strings.Repeat("a", 54)
			_, err := installer.NewAppOfAppsFromBom(fakeClient, bomWithChart(version), cfg)
			Expect(err).To(MatchError(ContainSubstring("release name")))
		})

		It("no BOM -> error", func() {
			_, err := installer.NewAppOfAppsFromBom(fakeClient, nil, appConfig())
			Expect(err).To(MatchError(ContainSubstring("BOM is required")))
		})

		It("BOM without the component -> error", func() {
			cfg := appConfig()
			cfg.Component = "other"
			_, err := installer.NewAppOfAppsFromBom(fakeClient, bomWithChart(version), cfg)
			Expect(err).To(MatchError(ContainSubstring("other chart not found in BOM")))
		})

		It("BOM chart without a version -> error", func() {
			_, err := installer.NewAppOfAppsFromBom(fakeClient, bomWithChart(""), appConfig())
			Expect(err).To(MatchError(ContainSubstring("apps chart not found in BOM")))
		})
	})

	Context("no K8s secret", func() {
		BeforeEach(func() {
			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				Build()
			apps = newAppOfApps(version, appConfig())
		})

		It("returns a clear error", func() {
			err := apps.Install(context.Background())
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
				},
			}
			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(secret).
				Build()
			apps = newAppOfApps(version, appConfig())
		})

		It("returns an error about the missing field", func() {
			err := apps.Install(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing required field"))
		})
	})

	Context("values", func() {
		var tmpDir string

		BeforeEach(func() {
			var err error

			tmpDir, err = os.MkdirTemp("", "app-of-apps-test-*")
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			Expect(os.RemoveAll(tmpDir)).To(Succeed())
		})

		withValues := func(valuesFiles []string, valuesOverride map[string]interface{}) installer.AppOfAppsConfig {
			cfg := appConfig()
			cfg.ValuesFiles = valuesFiles
			cfg.ValuesOverride = valuesOverride

			return cfg
		}

		It("merges multiple values files into the Application's valuesObject", func() {
			base := filepath.Join(tmpDir, "base.yaml")
			Expect(os.WriteFile(base, []byte("foo: bar\nnested:\n  a: 1\n  b: 2\n"), 0644)).To(Succeed())

			overlay := filepath.Join(tmpDir, "overlay.yaml")
			Expect(os.WriteFile(overlay, []byte("foo: overridden\nnested:\n  b: 99\n  c: 3\n"), 0644)).To(Succeed())

			apps = newAppOfApps(version, withValues([]string{base, overlay}, nil))
			Expect(apps.Install(context.Background())).To(Succeed())

			vals := helmValues(getApp(appName))
			Expect(vals).To(HaveKeyWithValue("foo", "overridden"))
			nested, ok := vals["nested"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(nested["a"]).To(BeNumerically("==", 1))
			Expect(nested["b"]).To(BeNumerically("==", 99))
			Expect(nested["c"]).To(BeNumerically("==", 3))
		})

		It("returns an error for non-existent values file", func() {
			apps = newAppOfApps(version, withValues([]string{"/nonexistent/values.yaml"}, nil))

			err := apps.Install(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("loading values files"))
		})

		It("returns an error for invalid YAML in values file", func() {
			badFile := filepath.Join(tmpDir, "bad.yaml")
			Expect(os.WriteFile(badFile, []byte("{{invalid yaml"), 0644)).To(Succeed())

			apps = newAppOfApps(version, withValues([]string{badFile}, nil))

			err := apps.Install(context.Background())
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

			apps = newAppOfApps(version, withValues([]string{fileValues}, override))
			Expect(apps.Install(context.Background())).To(Succeed())

			vals := helmValues(getApp(appName))
			Expect(vals).To(HaveKeyWithValue("foo", "from-file"))
			nested, ok := vals["nested"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(nested["configOnly"]).To(BeTrue())
			Expect(nested["shared"]).To(Equal("from-file"))
			Expect(nested["fileOnly"]).To(BeTrue())
		})

		It("defaults child chart sources to the configured OCI repository", func() {
			apps = newAppOfApps(version, appConfig())
			Expect(apps.Install(context.Background())).To(Succeed())

			Expect(helmValues(getApp(appName))).To(HaveKeyWithValue("chartsRegistry", secretURL))
		})

		It("preserves an explicit child chart registry override", func() {
			apps = newAppOfApps(version, withValues(nil, map[string]interface{}{"chartsRegistry": "other.example.com/charts"}))
			Expect(apps.Install(context.Background())).To(Succeed())

			Expect(helmValues(getApp(appName))).To(HaveKeyWithValue("chartsRegistry", "other.example.com/charts"))
		})
	})
})
