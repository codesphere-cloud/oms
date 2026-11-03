// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package argocd_test

import (
	"context"
	"fmt"

	"github.com/codesphere-cloud/oms/internal/installer/argocd"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Gateway bootstrap CRD barrier", func() {
	var (
		kube                                 *MockClient
		cfg                                  argocd.AppInstallerConfig
		ctx                                  context.Context
		cancel                               context.CancelFunc
		missing, notEstablished, unsupported string
	)

	BeforeEach(func() {
		kube = NewMockClient(GinkgoT())
		cfg = argocd.AppInstallerConfig{KubeClient: kube}
		ctx, cancel = context.WithCancel(context.Background())
		missing, notEstablished, unsupported = "", "", ""
	})
	AfterEach(func() { cancel() })

	expectCRDs := func() {
		kube.EXPECT().Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1.CustomResourceDefinition")).RunAndReturn(func(_ context.Context, key client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
			Expect(key.Namespace).To(BeEmpty())
			Expect(key.Name).To(BeElementOf("gatewayclasses.gateway.networking.k8s.io", "gateways.gateway.networking.k8s.io", "httproutes.gateway.networking.k8s.io", "envoyproxies.gateway.envoyproxy.io"))

			version := "v1"
			if key.Name == "envoyproxies.gateway.envoyproxy.io" {
				version = "v1alpha1"

				cancel()
			}

			if key.Name == missing {
				return apierrors.NewNotFound(schema.GroupResource{Group: "apiextensions.k8s.io", Resource: "customresourcedefinitions"}, key.Name)
			}

			crd := obj.(*apiextensionsv1.CustomResourceDefinition)
			crd.Name = key.Name
			crd.Status.Conditions = []apiextensionsv1.CustomResourceDefinitionCondition{
				{Type: apiextensionsv1.Established, Status: apiextensionsv1.ConditionTrue},
				{Type: apiextensionsv1.NamesAccepted, Status: apiextensionsv1.ConditionTrue},
			}

			crd.Spec.Versions = []apiextensionsv1.CustomResourceDefinitionVersion{{Name: version, Served: true}}
			if key.Name == notEstablished {
				crd.Status.Conditions[0].Status = apiextensionsv1.ConditionFalse
			}

			if key.Name == unsupported {
				crd.Spec.Versions[0].Served = false
			}

			return nil
		}).Times(4)
	}

	It("allows cluster setup once API types exist without requiring operator pod health", func() {
		expectCRDs()
		Expect(argocd.NewAppInstaller(cfg).WaitForGatewayCRDs(ctx, nil)).To(Succeed())
	})
	It("keeps waiting for an absent CRD and reports its name", func() {
		missing = "envoyproxies.gateway.envoyproxy.io"

		expectCRDs()
		Expect(argocd.NewAppInstaller(cfg).WaitForGatewayCRDs(ctx, nil)).To(MatchError(ContainSubstring("pending: [envoyproxies.gateway.envoyproxy.io]")))
	})
	It("requires Established rather than only NamesAccepted", func() {
		notEstablished = "gateways.gateway.networking.k8s.io"

		expectCRDs()
		Expect(argocd.NewAppInstaller(cfg).WaitForGatewayCRDs(ctx, nil)).To(MatchError(ContainSubstring("pending: [gateways.gateway.networking.k8s.io]")))
	})
	It("requires the API version used by the installer to be served", func() {
		unsupported = "httproutes.gateway.networking.k8s.io"

		expectCRDs()
		Expect(argocd.NewAppInstaller(cfg).WaitForGatewayCRDs(ctx, nil)).To(MatchError(ContainSubstring("pending: [httproutes.gateway.networking.k8s.io]")))
	})
	It("reports Kubernetes access errors instead of hiding them as missing CRDs", func() {
		kube.EXPECT().Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1.CustomResourceDefinition")).Return(fmt.Errorf("access denied")).Once()
		Expect(argocd.NewAppInstaller(cfg).WaitForGatewayCRDs(ctx, nil)).To(MatchError(ContainSubstring("access denied")))
	})
	It("does not wait when both gateways disable Gateway API", func() {
		cfg.Config.Cluster.Gateway.GatewayAPI = &files.GatewayAPIConfig{Enabled: false}
		cfg.Config.Cluster.PublicGateway.GatewayAPI = &files.GatewayAPIConfig{Enabled: false}
		Expect(argocd.NewAppInstaller(cfg).WaitForGatewayCRDs(ctx, nil)).To(Succeed())
	})
	It("still requires CRDs when only the public gateway uses Gateway API", func() {
		cfg.Config.Cluster.Gateway.GatewayAPI = &files.GatewayAPIConfig{Enabled: false}

		expectCRDs()
		Expect(argocd.NewAppInstaller(cfg).WaitForGatewayCRDs(ctx, nil)).To(Succeed())
	})
})
