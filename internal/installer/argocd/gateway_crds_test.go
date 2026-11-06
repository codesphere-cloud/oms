// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package argocd_test

import (
	"context"
	"fmt"
	"time"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/argocd"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
)

var _ = Describe("Gateway CRD readiness", func() {
	var (
		helm *installer.MockHelmClient
		cfg  argocd.AppInstallerConfig
	)

	BeforeEach(func() {
		helm = installer.NewMockHelmClient(GinkgoT())
		cfg = argocd.AppInstallerConfig{HelmClient: helm}
	})

	expectWait := func(err error) {
		helm.EXPECT().WaitForResources(mock.Anything, "apiextensions.k8s.io/v1", "CustomResourceDefinition", []string{
			"gatewayclasses.gateway.networking.k8s.io", "gateways.gateway.networking.k8s.io",
			"httproutes.gateway.networking.k8s.io", "envoyproxies.gateway.envoyproxy.io",
		}, 30*time.Minute).Return(err).Once()
	}

	It("delegates the required CRDs to the Helm waiter", func() {
		expectWait(nil)
		Expect(argocd.NewAppInstaller(cfg).WaitForGatewayCRDs(context.Background(), nil)).To(Succeed())
	})
	It("skips waiting when both gateways disable Gateway API", func() {
		cfg.Config.Cluster.Gateway.GatewayAPI = &files.GatewayAPIConfig{Enabled: false}
		cfg.Config.Cluster.PublicGateway.GatewayAPI = &files.GatewayAPIConfig{Enabled: false}
		Expect(argocd.NewAppInstaller(cfg).WaitForGatewayCRDs(context.Background(), nil)).To(Succeed())
	})
	It("waits when only the public gateway uses Gateway API", func() {
		cfg.Config.Cluster.Gateway.GatewayAPI = &files.GatewayAPIConfig{Enabled: false}

		expectWait(nil)
		Expect(argocd.NewAppInstaller(cfg).WaitForGatewayCRDs(context.Background(), nil)).To(Succeed())
	})
	It("propagates Helm readiness failures", func() {
		expectWait(fmt.Errorf("timed out"))
		Expect(argocd.NewAppInstaller(cfg).WaitForGatewayCRDs(context.Background(), nil)).To(MatchError(ContainSubstring("timed out")))
	})
})
