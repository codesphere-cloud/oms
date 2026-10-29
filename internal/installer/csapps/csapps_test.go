// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package csapps_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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

	if releaseName != "" {
		cfg.CsApps = map[string]any{"releaseName": releaseName}
	}

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
})

var _ = Describe("Values", func() {
	It("full config -> the Codesphere chart version and values, with secrets from the vault", func() {
		cfg := fullConfig("pr-1")
		b := bomWithCharts()

		values, err := csapps.Values(cfg, b, chartsRegistry, "trigger-1")
		Expect(err).NotTo(HaveOccurred())

		Expect(at(values, "chartsRegistry")).To(Equal(chartsRegistry))
		Expect(at(values, "releaseName")).To(Equal("pr-1"))
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

	It("no release name in the config -> the installer's default release", func() {
		values, err := csapps.Values(fullConfig(""), bomWithCharts(), chartsRegistry, "trigger-1")
		Expect(err).NotTo(HaveOccurred())

		Expect(at(values, "releaseName")).To(Equal("codesphere"))
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
		cfg.CsApps["appProject"] = "previews"
		cfg.CsApps["applications"] = map[string]any{
			"codesphere": map[string]any{"ignoreDifferences": []any{map[string]any{"kind": "HTTPRoute"}}},
		}

		values, err := csapps.Values(cfg, bomWithCharts(), chartsRegistry, "trigger-1")
		Expect(err).NotTo(HaveOccurred())

		Expect(at(values, "appProject")).To(Equal("previews"))
		Expect(at(values, "applications", "codesphere", "ignoreDifferences")).To(HaveLen(1))
		Expect(at(values, "applications", "codesphere", "version")).To(Equal(codesphereVersion))
	})

	It("empty csApps.releaseName -> the default release, in the values and the Application names", func() {
		cfg := fullConfig("")
		cfg.CsApps = map[string]any{"releaseName": ""}

		values, err := csapps.Values(cfg, bomWithCharts(), chartsRegistry, "trigger-1")
		Expect(err).NotTo(HaveOccurred())

		Expect(at(values, "releaseName")).To(Equal("codesphere"))
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

		return csapps.Install(ctx, kubeClient, fullConfig("pr-1"), bomWithCharts(), nil)
	}

	It("release pr-1 -> applies pr-1-cs-applications and waits for it to sync", func() {
		Expect(installUntilWait()).To(MatchError(ContainSubstring(`"pr-1-cs-applications"`)))

		app := &argov1alpha1.Application{}
		Expect(kubeClient.Get(context.Background(), client.ObjectKey{Name: "pr-1-cs-applications", Namespace: "argocd"}, app)).
			To(Succeed())
		Expect(app.Spec.Source.RepoURL).To(Equal(chartsRegistry))
		Expect(app.Spec.Source.Chart).To(Equal("cs-applications"))
		Expect(app.Spec.Source.TargetRevision).To(Equal(csAppsVersion))
		Expect(app.Finalizers).To(ConsistOf(argov1alpha1.ResourcesFinalizerName))

		values := map[string]any{}
		Expect(json.Unmarshal(app.Spec.Source.Helm.ValuesObject.Raw, &values)).To(Succeed())
		Expect(at(values, "chartsRegistry")).To(Equal(chartsRegistry))
		Expect(at(values, "applications", "codesphere", "version")).To(Equal(codesphereVersion))
		Expect(at(values, "applications", "codesphere", "valuesObject", "global", "externalSecretsSyncTrigger")).
			NotTo(BeEmpty())
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
