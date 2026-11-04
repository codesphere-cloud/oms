// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/codesphere-cloud/oms/internal/installer"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
)

var _ = Describe("Helm registry trust", func() {
	var (
		helm      installer.HelmClient
		server    *httptest.Server
		authority []byte
	)

	BeforeEach(func() {
		// A real TLS handshake verifies the configured CA without a live registry.
		handler := NewMockRegistryHandler(GinkgoT())
		handler.EXPECT().ServeHTTP(mock.Anything, mock.Anything).Run(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}).Return().Maybe()
		server = httptest.NewTLSServer(handler)
		DeferCleanup(server.Close)
		authority = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})

		var err error

		helm, err = installer.NewHelmClient("argocd")
		Expect(err).NotTo(HaveOccurred())
	})

	It("rejects invalid CA data before accessing the registry", func() {
		Expect(helm.ConfigureRegistryCA([]byte("invalid"))).To(MatchError(ContainSubstring("no valid PEM certificates")))
	})

	It("uses the configured CA during login", func() {
		Expect(helm.ConfigureRegistryCA(authority)).To(Succeed())
		// Failed authentication avoids writing Helm credentials to disk.
		err := helm.LoginRegistry(context.Background(), strings.TrimPrefix(server.URL, "https://"), "user", "password")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("403"))
		Expect(err.Error()).NotTo(ContainSubstring("x509"))
	})

	It("uses the configured CA for anonymous chart downloads", func() {
		Expect(helm.ConfigureRegistryCA(authority)).To(Succeed())
		err := helm.InstallChart(context.Background(), installer.ChartConfig{
			ChartName: "oci://" + strings.TrimPrefix(server.URL, "https://") + "/charts/argocd",
			Version:   "1.0.0", Namespace: "argocd", ReleaseName: "argocd",
		}, installer.InstallChartOptions{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("403"))
		Expect(err.Error()).NotTo(ContainSubstring("x509"))
	})

	It("retains CA trust for chart downloads after a login attempt", func() {
		Expect(helm.ConfigureRegistryCA(authority)).To(Succeed())

		host := strings.TrimPrefix(server.URL, "https://")
		Expect(helm.LoginRegistry(context.Background(), host, "user", "password")).NotTo(Succeed())
		err := helm.UpgradeChart(context.Background(), installer.ChartConfig{
			ChartName: "oci://" + host + "/charts/argocd", Version: "1.0.0",
			Namespace: "argocd", ReleaseName: "argocd",
		}, installer.UpgradeChartOptions{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("403"))
		Expect(err.Error()).NotTo(ContainSubstring("x509"))
	})
})
