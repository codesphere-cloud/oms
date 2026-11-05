// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package csapps_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	argo "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/installer/csapps"
	csappsvalues "github.com/codesphere-cloud/oms/internal/installer/csapps/values"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

const (
	chartsRegistry    = "registry.example.com/codesphere-cloud/charts"
	codesphereVersion = "1.40.0-v1.2.3"
	vaultSecretName   = "pr-1-vault"
	csAppsVersion     = "0.2.0"
)

func fullConfig(releaseName string) *files.RootConfig {
	data, err := os.ReadFile(filepath.Join("values", "testdata", "full_config.yaml"))
	Expect(err).NotTo(HaveOccurred())

	cfg := files.NewRootConfig()
	Expect(cfg.Unmarshal(data)).To(Succeed())

	// Under ArgoCD the key comes from the vault.
	cfg.Codesphere.CustomCertificate.Key = ""

	cfg.Codesphere.ReleaseName = releaseName

	return &cfg
}

func bomWithCharts() *bom.Config {
	b, err := bom.Parse(filepath.Join("values", "testdata", "full_config_bom.json"))
	Expect(err).NotTo(HaveOccurred())

	codesphere := b.Components["codesphere"]
	codesphere.Files = map[string]bom.FileRef{
		"chart": {OciRef: "ghcr.io/codesphere-cloud/charts/codesphere:" + codesphereVersion},
	}
	b.Components["codesphere"] = codesphere
	b.Components["cs-applications"] = bom.ComponentConfig{Files: map[string]bom.FileRef{
		"chart": {OciRef: "ghcr.io/codesphere-cloud/charts/cs-applications:" + csAppsVersion},
	}}

	return b
}

func at(values map[string]any, keys ...string) any {
	GinkgoHelper()

	var current any = values
	for _, key := range keys {
		m, ok := current.(map[string]any)
		Expect(ok).To(BeTrue(), "%v is not a map", keys)

		current = m[key]
	}

	return current
}

// asJSON returns value as it reads back from the Application's JSON values.
func asJSON(value any) any {
	GinkgoHelper()

	raw, err := json.Marshal(value)
	Expect(err).NotTo(HaveOccurred())

	var decoded any
	Expect(json.Unmarshal(raw, &decoded)).To(Succeed())

	return decoded
}

var _ = Describe("Install", func() {
	var kubeClient client.Client

	BeforeEach(func() {
		scheme := runtime.NewScheme()
		Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
		Expect(argo.AddToScheme(scheme)).To(Succeed())

		registrySecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "argocd-codesphere-oci-read", Namespace: "argocd"},
			Data:       map[string][]byte{"url": []byte(chartsRegistry)},
		}
		kubeClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(registrySecret).Build()
	})

	// Nothing syncs the Applications in a fake cluster, so Install stops at the wait.
	installUntilWaitWith := func(cfg *files.RootConfig) error {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		return csapps.Install(ctx, kubeClient, cfg, bomWithCharts(), vaultSecretName)
	}

	installUntilWait := func() error {
		return installUntilWaitWith(fullConfig("pr-1"))
	}

	parentValues := func() map[string]any {
		GinkgoHelper()

		app := &argo.Application{}
		Expect(kubeClient.Get(context.Background(), client.ObjectKey{Name: "pr-1-cs-applications", Namespace: "argocd"}, app)).
			To(Succeed())

		values := map[string]any{}
		Expect(json.Unmarshal(app.Spec.Source.Helm.ValuesObject.Raw, &values)).To(Succeed())

		return values
	}

	Context("values of the cs-applications Application", func() {
		appliedValues := func(cfg *files.RootConfig) map[string]any {
			GinkgoHelper()

			Expect(installUntilWaitWith(cfg)).To(HaveOccurred())

			return parentValues()
		}

		It("full config -> the Codesphere chart version, and the built global values shared at the top level", func() {
			cfg := fullConfig("pr-1")

			values := appliedValues(cfg)

			Expect(values).NotTo(HaveKey("releaseName"))
			Expect(at(values, "namespace")).To(Equal("codesphere"))
			Expect(at(values, "applications", "codesphere", "enabled")).To(BeTrue())
			Expect(at(values, "applications", "codesphere", "version")).To(Equal(codesphereVersion))

			expected, err := csappsvalues.Build(cfg, bomWithCharts())
			Expect(err).NotTo(HaveOccurred())

			syncTrigger := at(values, "global", "externalSecretsSyncTrigger")
			Expect(syncTrigger).NotTo(BeEmpty())

			expectedGlobal := expected["global"].(map[string]any)
			expectedGlobal["externalSecrets"] = true
			expectedGlobal["externalSecretsSyncTrigger"] = syncTrigger
			expectedGlobal["externalSecretsVaultSecret"] = vaultSecretName
			expectedGlobal["argoCd"] = true

			delete(expected, "global")

			Expect(at(values, "global")).To(Equal(asJSON(expectedGlobal)))
			Expect(at(values, "applications", "codesphere", "valuesObject")).To(Equal(asJSON(expected)))
		})

		It("override with a key outside global -> only in the Codesphere values, not shared", func() {
			cfg := fullConfig("pr-1")
			cfg.Codesphere.Override = map[string]any{"ideService": map[string]any{"replicas": 3}}

			values := appliedValues(cfg)

			Expect(at(values, "applications", "codesphere", "valuesObject")).
				To(Equal(map[string]any{"ideService": map[string]any{"replicas": float64(3)}}))
			Expect(at(values, "global")).NotTo(HaveKey("ideService"))
		})

		It("override disables external secrets -> still enabled, the values hold no secrets", func() {
			cfg := fullConfig("pr-1")
			cfg.Codesphere.Override = map[string]any{"global": map[string]any{"externalSecrets": false}}

			Expect(at(appliedValues(cfg), "global", "externalSecrets")).To(BeTrue())
		})

		It("csApps block -> merged over the built values", func() {
			cfg := fullConfig("pr-1")
			cfg.CsApps = map[string]any{"appProject": "previews"}
			cfg.CsApps["applications"] = map[string]any{
				"codesphere": map[string]any{"ignoreDifferences": []any{map[string]any{"kind": "HTTPRoute"}}},
			}

			values := appliedValues(cfg)

			Expect(at(values, "appProject")).To(Equal("previews"))
			Expect(at(values, "applications", "codesphere", "ignoreDifferences")).To(HaveLen(1))
			Expect(at(values, "applications", "codesphere", "version")).To(Equal(codesphereVersion))
		})
	})

	It("BOM without a Codesphere chart -> error before applying anything", func() {
		b := bomWithCharts()
		codesphere := b.Components["codesphere"]
		codesphere.Files = nil
		b.Components["codesphere"] = codesphere

		Expect(csapps.Install(context.Background(), kubeClient, fullConfig("pr-1"), b, vaultSecretName)).
			To(MatchError(ContainSubstring("codesphere chart not found in BOM")))

		apps := &argo.ApplicationList{}
		Expect(kubeClient.List(context.Background(), apps)).To(Succeed())
		Expect(apps.Items).To(BeEmpty())
	})

	It("release pr-1 -> applies pr-1-cs-applications and waits for it to sync", func() {
		Expect(installUntilWait()).To(MatchError(ContainSubstring(`"pr-1-cs-applications"`)))

		app := &argo.Application{}
		Expect(kubeClient.Get(context.Background(), client.ObjectKey{Name: "pr-1-cs-applications", Namespace: "argocd"}, app)).
			To(Succeed())
		Expect(app.Spec.Source.RepoURL).To(Equal(chartsRegistry))
		Expect(app.Spec.Source.Chart).To(Equal("cs-applications"))
		Expect(app.Spec.Source.TargetRevision).To(Equal(csAppsVersion))
		Expect(app.Spec.Source.Helm.ReleaseName).To(Equal("pr-1"))
		Expect(app.Finalizers).To(ConsistOf(argo.ResourcesFinalizerName))

		values := parentValues()
		Expect(at(values, "chartsRegistry")).To(Equal(chartsRegistry))
		Expect(at(values, "applications", "codesphere", "version")).To(Equal(codesphereVersion))
		Expect(at(values, "global", "externalSecretsSyncTrigger")).
			NotTo(BeEmpty())
	})

	It("invalid release name -> error before applying anything", func() {
		cfg := fullConfig("PR_1")

		Expect(csapps.Install(context.Background(), kubeClient, cfg, bomWithCharts(), vaultSecretName)).
			To(MatchError(ContainSubstring(`release name "PR_1"`)))

		apps := &argo.ApplicationList{}
		Expect(kubeClient.List(context.Background(), apps)).To(Succeed())
		Expect(apps.Items).To(BeEmpty())
	})

	It("second install -> a new sync trigger, so External Secrets reads the vault again", func() {
		syncTrigger := func() any {
			return at(parentValues(), "global", "externalSecretsSyncTrigger")
		}

		Expect(installUntilWait()).To(HaveOccurred())

		first := syncTrigger()

		Expect(installUntilWait()).To(HaveOccurred())

		Expect(syncTrigger()).NotTo(Equal(first))
	})

	Context("ArgoCD syncs the Applications", func() {
		const parentAppName = "pr-1-cs-applications"

		subAppNames := []string{"pr-1-codesphere", "pr-1-mounter"}

		var synced map[string]bool

		BeforeEach(func() {
			synced = map[string]bool{parentAppName: true}

			scheme := runtime.NewScheme()
			Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
			Expect(argo.AddToScheme(scheme)).To(Succeed())

			objects := []client.Object{&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "argocd-codesphere-oci-read", Namespace: "argocd"},
				Data:       map[string][]byte{"url": []byte(chartsRegistry)},
			}}

			var resources []argo.ResourceStatus

			for _, name := range subAppNames {
				objects = append(objects, &argo.Application{
					ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "argocd"},
					Spec:       argo.ApplicationSpec{Source: &argo.ApplicationSource{Chart: name}},
				})
				resources = append(resources, argo.ResourceStatus{
					Group: "argoproj.io", Kind: "Application", Namespace: "argocd", Name: name,
				})
			}
			// The parent also deploys resources that are not Applications.
			resources = append(resources, argo.ResourceStatus{Kind: "ConfigMap", Namespace: "argocd", Name: "pr-1-config"})

			kubeClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).WithInterceptorFuncs(interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					if err := c.Get(ctx, key, obj, opts...); err != nil {
						return fmt.Errorf("failed to get %s: %w", key, err)
					}

					app, ok := obj.(*argo.Application)
					if !ok || !synced[key.Name] {
						return nil
					}

					app.Status.Sync.ComparedTo.Source = *app.Spec.Source.DeepCopy()
					app.Status.Sync.Status = argo.SyncStatusCodeSynced

					app.Status.Health.Status = "Healthy"
					if key.Name == parentAppName {
						app.Status.Resources = resources
					}

					return nil
				},
			}).Build()
		})

		installWithTimeout := func() error {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			return csapps.Install(ctx, kubeClient, fullConfig("pr-1"), bomWithCharts(), vaultSecretName)
		}

		It("parent and every sub application synced and healthy -> done", func() {
			for _, name := range subAppNames {
				synced[name] = true
			}

			Expect(installWithTimeout()).To(Succeed())
		})

		It("one sub application never syncs -> error naming only that one", func() {
			synced["pr-1-codesphere"] = true

			err := installWithTimeout()

			Expect(err).To(MatchError(ContainSubstring(`"pr-1-mounter"`)))
			Expect(err).NotTo(MatchError(ContainSubstring(`"pr-1-codesphere"`)))
		})

		It("no sub application syncs -> error naming each one", func() {
			err := installWithTimeout()

			Expect(err).To(MatchError(ContainSubstring(`"pr-1-codesphere"`)))
			Expect(err).To(MatchError(ContainSubstring(`"pr-1-mounter"`)))
		})
	})

	It("no registry credentials -> error before applying anything", func() {
		Expect(kubeClient.Delete(context.Background(), &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "argocd-codesphere-oci-read", Namespace: "argocd"},
		})).To(Succeed())

		Expect(installUntilWait()).To(MatchError(ContainSubstring("failed to resolve repo url")))

		apps := &argo.ApplicationList{}
		Expect(kubeClient.List(context.Background(), apps)).To(Succeed())
		Expect(apps.Items).To(BeEmpty())
	})
})
