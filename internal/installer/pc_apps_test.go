// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/codesphere-cloud/oms/internal/installer"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
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

		// Derived from the secret
		expectedChartURL = "oci://ghcr.io/codesphere-cloud/charts/pc-applications"
	)

	var (
		helmMock   *installer.MockHelmClient
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

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())

		helmMock = installer.NewMockHelmClient(GinkgoT())
	})

	Context("successful install (secret exists)", func() {
		BeforeEach(func() {
			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(newSecret()).
				Build()
			pcApps = &installer.PCApps{
				Version:   version,
				Namespace: namespace,
				Helm:      helmMock,
				Client:    fakeClient,
			}
		})

		It("reads credentials from K8s secret and calls UpgradeChart with InstallIfNotExist", func() {
			helmMock.EXPECT().LoginRegistry(mock.Anything, "ghcr.io", secretUsername, secretPassword).Return(nil)
			helmMock.EXPECT().UpgradeChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
				return cfg.ReleaseName == "pc-applications" &&
					cfg.ChartName == expectedChartURL &&
					cfg.Namespace == namespace &&
					cfg.Version == version &&
					cfg.CreateNamespace == true
			}), installer.UpgradeChartOptions{InstallIfNotExist: true}).Return(nil)

			err := pcApps.Install(context.Background())
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error when UpgradeChart fails", func() {
			helmMock.EXPECT().LoginRegistry(mock.Anything, "ghcr.io", secretUsername, secretPassword).Return(nil)
			helmMock.EXPECT().UpgradeChart(mock.Anything, mock.Anything, mock.Anything).
				Return(errors.New("upgrade conflict"))

			err := pcApps.Install(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("install/upgrade failed"))
		})
	})

	Context("registry login failure", func() {
		BeforeEach(func() {
			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(newSecret()).
				Build()
			pcApps = &installer.PCApps{
				Version:   version,
				Namespace: namespace,
				Helm:      helmMock,
				Client:    fakeClient,
			}
		})

		It("returns an error without attempting install", func() {
			helmMock.EXPECT().LoginRegistry(mock.Anything, "ghcr.io", secretUsername, secretPassword).
				Return(errors.New("invalid credentials"))

			err := pcApps.Install(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("registry login failed"))
		})
	})

	Context("no K8s secret", func() {
		BeforeEach(func() {
			// No objects in the fake client
			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				Build()
			pcApps = &installer.PCApps{
				Version:   version,
				Namespace: namespace,
				Helm:      helmMock,
				Client:    fakeClient,
			}
		})

		It("returns a clear error", func() {
			err := pcApps.Install(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("argocd-codesphere-oci-read"))
			Expect(err.Error()).To(ContainSubstring("oms beta install argocd"))
		})
	})

	Context("K8s secret exists but missing fields", func() {
		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "argocd-codesphere-oci-read",
					Namespace: "argocd",
				},
				Data: map[string][]byte{
					"url":      []byte(secretURL),
					"username": []byte(secretUsername),
					// password is missing
				},
			}
			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(secret).
				Build()
			pcApps = &installer.PCApps{
				Version:   version,
				Namespace: namespace,
				Helm:      helmMock,
				Client:    fakeClient,
			}
		})

		It("returns an error about missing fields", func() {
			err := pcApps.Install(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing required fields"))
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

		It("merges multiple values files and passes them to the chart config", func() {
			base := filepath.Join(tmpDir, "base.yaml")
			Expect(os.WriteFile(base, []byte("foo: bar\nnested:\n  a: 1\n  b: 2\n"), 0644)).To(Succeed())

			overlay := filepath.Join(tmpDir, "overlay.yaml")
			Expect(os.WriteFile(overlay, []byte("foo: overridden\nnested:\n  b: 99\n  c: 3\n"), 0644)).To(Succeed())

			pcApps = &installer.PCApps{
				Version:     version,
				Namespace:   namespace,
				ValuesFiles: []string{base, overlay},
				Helm:        helmMock,
				Client:      fakeClient,
			}

			helmMock.EXPECT().LoginRegistry(mock.Anything, "ghcr.io", secretUsername, secretPassword).Return(nil)
			helmMock.EXPECT().UpgradeChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
				nested, ok := cfg.Values["nested"].(map[string]any)
				if !ok {
					return false
				}
				return cfg.Values["foo"] == "overridden" &&
					fmt.Sprint(nested["a"]) == "1" &&
					fmt.Sprint(nested["b"]) == "99" &&
					fmt.Sprint(nested["c"]) == "3"
			}), installer.UpgradeChartOptions{InstallIfNotExist: true}).Return(nil)

			err := pcApps.Install(context.Background())
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error for non-existent values file", func() {
			pcApps = &installer.PCApps{
				Version:     version,
				Namespace:   namespace,
				ValuesFiles: []string{"/nonexistent/values.yaml"},
				Helm:        helmMock,
				Client:      fakeClient,
			}

			err := pcApps.Install(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("loading values files"))
		})

		It("returns an error for invalid YAML in values file", func() {
			badFile := filepath.Join(tmpDir, "bad.yaml")
			Expect(os.WriteFile(badFile, []byte("{{invalid yaml"), 0644)).To(Succeed())

			pcApps = &installer.PCApps{
				Version:     version,
				Namespace:   namespace,
				ValuesFiles: []string{badFile},
				Helm:        helmMock,
				Client:      fakeClient,
			}

			err := pcApps.Install(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("loading values files"))
		})
	})
})
