// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package csapps_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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
	csAppsVersion     = "0.2.0"
	vaultSecretName   = "pr-1-vault"
	parentAppName     = "pr-1-cs-applications"
)

var subAppNames = []string{"pr-1-codesphere", "pr-1-mounter"}

func fullConfig(releaseName string) *files.RootConfig {
	data, err := os.ReadFile(filepath.Join("values", "testdata", "full_config.yaml"))
	Expect(err).NotTo(HaveOccurred())

	cfg := files.NewRootConfig()
	Expect(cfg.Unmarshal(data)).To(Succeed())

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

// newCluster returns a fake cluster with the registry credentials and the parent's sub
// applications. ArgoCD reports the Applications named in synced as synced and healthy.
func newCluster(synced ...string) client.Client {
	scheme := runtime.NewScheme()
	Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
	Expect(argo.AddToScheme(scheme)).To(Succeed())

	objects := []client.Object{&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "argocd-codesphere-oci-read", Namespace: "argocd"},
		Data:       map[string][]byte{"url": []byte(chartsRegistry)},
	}}

	resources := []argo.ResourceStatus{{Kind: "ConfigMap", Namespace: "argocd", Name: "pr-1-config"}}
	for _, name := range subAppNames {
		objects = append(objects, &argo.Application{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "argocd"},
			Spec:       argo.ApplicationSpec{Source: &argo.ApplicationSource{Chart: name}},
		})
		resources = append(resources, argo.ResourceStatus{Group: "argoproj.io", Kind: "Application", Namespace: "argocd", Name: name})
	}

	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).WithInterceptorFuncs(interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if err := c.Get(ctx, key, obj, opts...); err != nil {
				return fmt.Errorf("failed to get %s: %w", key, err)
			}

			app, ok := obj.(*argo.Application)
			if !ok || !slices.Contains(synced, key.Name) {
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
}

// install runs Install with a short deadline: an Application that never syncs fails its wait.
func install(kubeClient client.Client, cfg *files.RootConfig, csBom *bom.Config) error {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	if err := csapps.Install(ctx, kubeClient, cfg, csBom, vaultSecretName); err != nil {
		return fmt.Errorf("install: %w", err)
	}

	return nil
}

func parentApp(kubeClient client.Client) *argo.Application {
	GinkgoHelper()

	app := &argo.Application{}
	Expect(kubeClient.Get(context.Background(), client.ObjectKey{Name: parentAppName, Namespace: "argocd"}, app)).
		To(Succeed())

	return app
}

func parentValues(kubeClient client.Client) map[string]any {
	GinkgoHelper()

	values := map[string]any{}
	Expect(json.Unmarshal(parentApp(kubeClient).Spec.Source.Helm.ValuesObject.Raw, &values)).To(Succeed())

	return values
}

func newSyncedCluster() client.Client {
	return newCluster(append([]string{parentAppName}, subAppNames...)...)
}

// appliedValues installs cfg and returns the parent's values.
func appliedValues(cfg *files.RootConfig) map[string]any {
	GinkgoHelper()

	kubeClient := newSyncedCluster()
	Expect(install(kubeClient, cfg, bomWithCharts())).To(Succeed())

	return parentValues(kubeClient)
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
	It("release pr-1 -> applies pr-1-cs-applications with the cs-applications chart from the BOM", func() {
		kubeClient := newSyncedCluster()

		Expect(install(kubeClient, fullConfig("pr-1"), bomWithCharts())).To(Succeed())

		app := parentApp(kubeClient)
		Expect(app.Spec.Source.Chart).To(Equal("cs-applications"))
		Expect(app.Spec.Source.TargetRevision).To(Equal(csAppsVersion))
		Expect(app.Spec.Source.Helm.ReleaseName).To(Equal("pr-1"))
		Expect(app.Finalizers).To(ConsistOf(argo.ResourcesFinalizerName))
	})

	It("full config -> the Codesphere chart version, and the built global values at the top level", func() {
		cfg := fullConfig("pr-1")

		values := appliedValues(cfg)

		expected, err := csappsvalues.Build(cfg, bomWithCharts())
		Expect(err).NotTo(HaveOccurred())

		expectedGlobal := expected["global"].(map[string]any)
		expectedGlobal["externalSecrets"] = true
		expectedGlobal["externalSecretsSyncTrigger"] = at(values, "global", "externalSecretsSyncTrigger")
		expectedGlobal["externalSecretsVaultSecret"] = vaultSecretName
		expectedGlobal["argoCd"] = true

		delete(expected, "global")

		Expect(at(values, "applications", "codesphere", "version")).To(Equal(codesphereVersion))
		Expect(at(values, "global")).To(Equal(asJSON(expectedGlobal)))
		Expect(at(values, "applications", "codesphere", "valuesObject")).To(Equal(asJSON(expected)))
	})

	It("override with a key outside global -> only in the Codesphere values, not in global", func() {
		cfg := fullConfig("pr-1")
		cfg.Codesphere.Override = map[string]any{"ideService": map[string]any{"replicas": 3}}

		values := appliedValues(cfg)

		Expect(at(values, "applications", "codesphere", "valuesObject")).
			To(Equal(map[string]any{"ideService": map[string]any{"replicas": float64(3)}}))
		Expect(at(values, "global")).NotTo(HaveKey("ideService"))
	})

	It("csApps disables external secrets -> still enabled, the values hold no secrets", func() {
		cfg := fullConfig("pr-1")
		cfg.CsApps = map[string]any{"global": map[string]any{"externalSecrets": false}}

		Expect(at(appliedValues(cfg), "global", "externalSecrets")).To(BeTrue())
	})

	It("second install -> a new sync trigger, so External Secrets reads the vault again", func() {
		kubeClient := newSyncedCluster()

		Expect(install(kubeClient, fullConfig("pr-1"), bomWithCharts())).To(Succeed())
		first := at(parentValues(kubeClient), "global", "externalSecretsSyncTrigger")

		Expect(install(kubeClient, fullConfig("pr-1"), bomWithCharts())).To(Succeed())

		Expect(at(parentValues(kubeClient), "global", "externalSecretsSyncTrigger")).NotTo(Equal(first))
	})

	It("BOM without a Codesphere chart -> error before applying anything", func() {
		kubeClient := newCluster()
		b := bomWithCharts()
		codesphere := b.Components["codesphere"]
		codesphere.Files = nil
		b.Components["codesphere"] = codesphere

		Expect(install(kubeClient, fullConfig("pr-1"), b)).To(MatchError(ContainSubstring("codesphere chart not found in BOM")))

		Expect(kubeClient.Get(context.Background(), client.ObjectKey{Name: parentAppName, Namespace: "argocd"}, &argo.Application{})).
			NotTo(Succeed())
	})
})
