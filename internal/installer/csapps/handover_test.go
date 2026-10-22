// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package csapps_test

import (
	"context"
	"os"
	"path/filepath"

	argov1alpha1 "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.yaml.in/yaml/v3"
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

var _ = Describe("HandOverToInstaller", func() {
	writeConfig := func(content string) string {
		path := filepath.Join(GinkgoT().TempDir(), "config.yaml")
		Expect(os.WriteFile(path, []byte(content), 0o600)).To(Succeed())

		return path
	}

	readConfig := func(path string) map[string]any {
		data, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())

		config := map[string]any{}
		Expect(yaml.Unmarshal(data, &config)).To(Succeed())

		return config
	}

	It("installed version -> the installer skips the chart and migrates from that version", func() {
		path := writeConfig("codesphere:\n  domain: example.com\n")

		Expect(csapps.HandOverToInstaller(path, "1.113.0")).To(Succeed())

		Expect(readConfig(path)["codesphere"]).To(Equal(map[string]any{
			"domain":           "example.com",
			"skipChartDeploy":  true,
			"installedVersion": "1.113.0",
		}))
	})

	It("nothing installed -> no installed version, so the installer looks for a Helm release", func() {
		path := writeConfig("codesphere:\n  domain: example.com\n")

		Expect(csapps.HandOverToInstaller(path, "")).To(Succeed())

		Expect(readConfig(path)["codesphere"]).NotTo(HaveKey("installedVersion"))
	})

	It("integer keys in the config -> kept as integers", func() {
		path := writeConfig("codesphere:\n  plans:\n    hostingPlans:\n      8: {cpuTenth: 10}\n")

		Expect(csapps.HandOverToInstaller(path, "")).To(Succeed())

		data, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(ContainSubstring("8:"))
		Expect(string(data)).NotTo(ContainSubstring(`"8":`))
	})
})
