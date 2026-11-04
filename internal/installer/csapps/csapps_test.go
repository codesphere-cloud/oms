// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package csapps_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	argov1alpha1 "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/installer/csapps"
	csappsvalues "github.com/codesphere-cloud/oms/internal/installer/csapps/values"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

const (
	chartsRegistry    = "registry.example.com/codesphere-cloud/charts"
	codesphereVersion = "1.40.0-v1.2.3"
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

var _ = Describe("ValidateConfig", func() {
	It("custom certificate key in the config -> error naming the vault entry", func() {
		cfg := &files.RootConfig{Codesphere: files.CodesphereConfig{
			CustomCertificate: &files.CustomCertificate{Cert: "cert-pem", Key: "key-pem"},
		}}

		Expect(csapps.ValidateConfig(cfg)).To(MatchError(ContainSubstring(`vault entry "customCertificateKey"`)))
	})

	It("custom certificate without key -> no error", func() {
		cfg := &files.RootConfig{Codesphere: files.CodesphereConfig{
			CustomCertificate: &files.CustomCertificate{Cert: "cert-pem"},
		}}

		Expect(csapps.ValidateConfig(cfg)).To(Succeed())
	})

	It("no custom certificate -> no error", func() {
		Expect(csapps.ValidateConfig(&files.RootConfig{})).To(Succeed())
	})

	It("release name pr-1 -> no error", func() {
		Expect(csapps.ValidateConfig(&files.RootConfig{Codesphere: files.CodesphereConfig{ReleaseName: "pr-1"}})).To(Succeed())
	})

	It("release name with upper case and underscore -> error naming the field", func() {
		cfg := &files.RootConfig{Codesphere: files.CodesphereConfig{ReleaseName: "PR_1"}}

		Expect(csapps.ValidateConfig(cfg)).To(MatchError(ContainSubstring(`codesphere.releaseName "PR_1"`)))
	})

	It("release name longer than Helm's 53 characters -> error", func() {
		cfg := &files.RootConfig{Codesphere: files.CodesphereConfig{ReleaseName: strings.Repeat("a", 54)}}

		Expect(csapps.ValidateConfig(cfg)).To(MatchError(ContainSubstring("longer than 53")))
	})
})

var _ = Describe("Values", func() {
	It("full config -> the Codesphere chart version and values, with secrets from the vault", func() {
		cfg := fullConfig("pr-1")
		b := bomWithCharts()

		values, err := csapps.Values(cfg, b, chartsRegistry, "trigger-1")
		Expect(err).NotTo(HaveOccurred())

		Expect(at(values, "chartsRegistry")).To(Equal(chartsRegistry))
		Expect(values).NotTo(HaveKey("releaseName"))
		Expect(at(values, "namespace")).To(Equal("codesphere"))
		Expect(at(values, "applications", "codesphere", "enabled")).To(BeTrue())
		Expect(at(values, "applications", "codesphere", "version")).To(Equal(codesphereVersion))

		expected, err := csappsvalues.Build(cfg, b)
		Expect(err).NotTo(HaveOccurred())

		expectedGlobal := expected["global"].(map[string]any)
		expectedGlobal["externalSecrets"] = true
		expectedGlobal["externalSecretsSyncTrigger"] = "trigger-1"
		expectedGlobal["argoCd"] = true

		Expect(at(values, "applications", "codesphere", "valuesObject")).To(Equal(expected))
	})

	It("override disables external secrets -> still enabled, the values hold no secrets", func() {
		cfg := fullConfig("pr-1")
		cfg.Codesphere.Override = map[string]any{"global": map[string]any{"externalSecrets": false}}

		values, err := csapps.Values(cfg, bomWithCharts(), chartsRegistry, "trigger-1")
		Expect(err).NotTo(HaveOccurred())

		Expect(at(values, "applications", "codesphere", "valuesObject", "global", "externalSecrets")).To(BeTrue())
	})

	It("csApps block -> merged over the built values", func() {
		cfg := fullConfig("pr-1")
		cfg.CsApps = map[string]any{"appProject": "previews"}
		cfg.CsApps["applications"] = map[string]any{
			"codesphere": map[string]any{"ignoreDifferences": []any{map[string]any{"kind": "HTTPRoute"}}},
		}

		values, err := csapps.Values(cfg, bomWithCharts(), chartsRegistry, "trigger-1")
		Expect(err).NotTo(HaveOccurred())

		Expect(at(values, "appProject")).To(Equal("previews"))
		Expect(at(values, "applications", "codesphere", "ignoreDifferences")).To(HaveLen(1))
		Expect(at(values, "applications", "codesphere", "version")).To(Equal(codesphereVersion))
	})

	It("BOM without a Codesphere chart -> error", func() {
		b := bomWithCharts()
		codesphere := b.Components["codesphere"]
		codesphere.Files = nil
		b.Components["codesphere"] = codesphere

		_, err := csapps.Values(fullConfig("pr-1"), b, chartsRegistry, "trigger-1")

		Expect(err).To(MatchError(ContainSubstring("codesphere chart not found in BOM")))
	})
})

var _ = Describe("Install", func() {
	var kubeClient client.Client

	BeforeEach(func() {
		scheme := runtime.NewScheme()
		Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
		Expect(argov1alpha1.AddToScheme(scheme)).To(Succeed())

		registrySecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "argocd-codesphere-oci-read", Namespace: "argocd"},
			Data:       map[string][]byte{"url": []byte(chartsRegistry)},
		}
		kubeClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(registrySecret).Build()
	})

	// Nothing syncs the Applications in a fake cluster, so Install stops at the wait.
	installUntilWait := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		return csapps.Install(ctx, kubeClient, fullConfig("pr-1"), bomWithCharts())
	}

	It("release pr-1 -> applies pr-1-cs-applications and waits for it to sync", func() {
		Expect(installUntilWait()).To(MatchError(ContainSubstring(`"pr-1-cs-applications"`)))

		app := &argov1alpha1.Application{}
		Expect(kubeClient.Get(context.Background(), client.ObjectKey{Name: "pr-1-cs-applications", Namespace: "argocd"}, app)).
			To(Succeed())
		Expect(app.Spec.Source.RepoURL).To(Equal(chartsRegistry))
		Expect(app.Spec.Source.Chart).To(Equal("cs-applications"))
		Expect(app.Spec.Source.TargetRevision).To(Equal(csAppsVersion))
		Expect(app.Spec.Source.Helm.ReleaseName).To(Equal("pr-1"))
		Expect(app.Finalizers).To(ConsistOf(argov1alpha1.ResourcesFinalizerName))

		values := map[string]any{}
		Expect(json.Unmarshal(app.Spec.Source.Helm.ValuesObject.Raw, &values)).To(Succeed())
		Expect(at(values, "chartsRegistry")).To(Equal(chartsRegistry))
		Expect(at(values, "applications", "codesphere", "version")).To(Equal(codesphereVersion))
		Expect(at(values, "applications", "codesphere", "valuesObject", "global", "externalSecretsSyncTrigger")).
			NotTo(BeEmpty())
	})

	It("default release -> no finalizer, deleting the Application leaves Codesphere running", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		Expect(csapps.Install(ctx, kubeClient, fullConfig(""), bomWithCharts())).To(HaveOccurred())

		app := &argov1alpha1.Application{}
		Expect(kubeClient.Get(context.Background(), client.ObjectKey{Name: "codesphere-cs-applications", Namespace: "argocd"}, app)).
			To(Succeed())
		Expect(app.Spec.Source.Helm.ReleaseName).To(Equal("codesphere"))
		Expect(app.Finalizers).To(BeEmpty())
	})

	It("invalid release name -> error before applying anything", func() {
		cfg := fullConfig("PR_1")

		Expect(csapps.Install(context.Background(), kubeClient, cfg, bomWithCharts())).
			To(MatchError(ContainSubstring("codesphere.releaseName")))

		apps := &argov1alpha1.ApplicationList{}
		Expect(kubeClient.List(context.Background(), apps)).To(Succeed())
		Expect(apps.Items).To(BeEmpty())
	})

	It("second install -> a new sync trigger, so External Secrets reads the vault again", func() {
		syncTrigger := func() any {
			app := &argov1alpha1.Application{}
			Expect(kubeClient.Get(context.Background(), client.ObjectKey{Name: "pr-1-cs-applications", Namespace: "argocd"}, app)).
				To(Succeed())

			values := map[string]any{}
			Expect(json.Unmarshal(app.Spec.Source.Helm.ValuesObject.Raw, &values)).To(Succeed())

			return at(values, "applications", "codesphere", "valuesObject", "global", "externalSecretsSyncTrigger")
		}

		Expect(installUntilWait()).To(HaveOccurred())

		first := syncTrigger()

		Expect(installUntilWait()).To(HaveOccurred())

		Expect(syncTrigger()).NotTo(Equal(first))
	})

	It("no registry credentials -> error before applying anything", func() {
		Expect(kubeClient.Delete(context.Background(), &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "argocd-codesphere-oci-read", Namespace: "argocd"},
		})).To(Succeed())

		Expect(installUntilWait()).To(MatchError(ContainSubstring("failed to resolve the charts registry")))

		apps := &argov1alpha1.ApplicationList{}
		Expect(kubeClient.List(context.Background(), apps)).To(Succeed())
		Expect(apps.Items).To(BeEmpty())
	})
})
