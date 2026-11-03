// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package argocd_test

import (
	"context"

	"github.com/codesphere-cloud/oms/internal/installer/argocd"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Gateway bootstrap CRD barrier", func() {
	var (
		kube *MockClient
		cfg  argocd.AppInstallerConfig
	)

	BeforeEach(func() {
		kube = NewMockClient(GinkgoT())
		cfg = argocd.AppInstallerConfig{KubeClient: kube}
	})

	expectCRDs := func() {
		kube.EXPECT().Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1.CustomResourceDefinition")).RunAndReturn(func(_ context.Context, key client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
			Expect(key.Namespace).To(BeEmpty())
			Expect(key.Name).To(BeElementOf("gatewayclasses.gateway.networking.k8s.io", "gateways.gateway.networking.k8s.io", "httproutes.gateway.networking.k8s.io", "envoyproxies.gateway.envoyproxy.io"))

			version := "v1"
			if key.Name == "envoyproxies.gateway.envoyproxy.io" {
				version = "v1alpha1"
			}

			crd := obj.(*apiextensionsv1.CustomResourceDefinition)
			crd.Name = key.Name
			crd.Status.Conditions = []apiextensionsv1.CustomResourceDefinitionCondition{
				{Type: apiextensionsv1.Established, Status: apiextensionsv1.ConditionTrue},
			}

			crd.Spec.Versions = []apiextensionsv1.CustomResourceDefinitionVersion{{Name: version, Served: true}}

			return nil
		}).Times(4)
	}

	It("allows cluster setup once API types exist without requiring operator pod health", func() {
		expectCRDs()
		Expect(argocd.NewAppInstaller(cfg).WaitForGatewayCRDs(context.Background(), nil)).To(Succeed())
	})
	It("does not wait when both gateways disable Gateway API", func() {
		cfg.Config.Cluster.Gateway.GatewayAPI = &files.GatewayAPIConfig{Enabled: false}
		cfg.Config.Cluster.PublicGateway.GatewayAPI = &files.GatewayAPIConfig{Enabled: false}
		Expect(argocd.NewAppInstaller(cfg).WaitForGatewayCRDs(context.Background(), nil)).To(Succeed())
	})
	It("still requires CRDs when only the public gateway uses Gateway API", func() {
		cfg.Config.Cluster.Gateway.GatewayAPI = &files.GatewayAPIConfig{Enabled: false}

		expectCRDs()
		Expect(argocd.NewAppInstaller(cfg).WaitForGatewayCRDs(context.Background(), nil)).To(Succeed())
	})
})
