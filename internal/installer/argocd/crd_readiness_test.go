// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package argocd_test

import (
	"context"
	"fmt"
	"time"

	"github.com/codesphere-cloud/oms/internal/installer"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Reuse the Kubernetes client mock used by the ArgoCD consumer.
var _ = Describe("Shared CRD readiness", func() {
	var (
		kube         *MockClient
		ctx          context.Context
		cancel       context.CancelFunc
		crd          apiextensionsv1.CustomResourceDefinition
		requirements []installer.CRDRequirement
	)

	BeforeEach(func() {
		kube = NewMockClient(GinkgoT())
		ctx, cancel = context.WithCancel(context.Background())
		requirements = []installer.CRDRequirement{{Name: "widgets.example.com", Version: "v2"}}
		crd = apiextensionsv1.CustomResourceDefinition{
			Spec:   apiextensionsv1.CustomResourceDefinitionSpec{Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{Name: "v2", Served: true}}},
			Status: apiextensionsv1.CustomResourceDefinitionStatus{Conditions: []apiextensionsv1.CustomResourceDefinitionCondition{{Type: apiextensionsv1.Established, Status: apiextensionsv1.ConditionTrue}}},
		}
	})
	AfterEach(func() { cancel() })

	expectCRD := func() {
		kube.EXPECT().Get(mock.Anything, client.ObjectKey{Name: "widgets.example.com"}, mock.Anything).
			Run(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) {
				*obj.(*apiextensionsv1.CustomResourceDefinition) = crd

				cancel()
			}).Return(nil).Once()
	}
	wait := func() error { return installer.WaitForCRDs(ctx, kube, requirements, time.Minute, nil) }

	It("accepts an established CRD serving the requested version", func() {
		expectCRD()
		Expect(wait()).To(Succeed())
	})
	It("reports an absent CRD as pending when cancelled", func() {
		kube.EXPECT().Get(mock.Anything, client.ObjectKey{Name: "widgets.example.com"}, mock.Anything).
			Run(func(context.Context, client.ObjectKey, client.Object, ...client.GetOption) { cancel() }).
			Return(apierrors.NewNotFound(schema.GroupResource{Resource: "customresourcedefinitions"}, "widgets.example.com")).Once()
		Expect(wait()).To(MatchError(ContainSubstring("pending: [widgets.example.com]")))
	})
	It("requires Established rather than only NamesAccepted", func() {
		crd.Status.Conditions[0].Type = apiextensionsv1.NamesAccepted

		expectCRD()
		Expect(wait()).To(MatchError(ContainSubstring("pending: [widgets.example.com]")))
	})
	It("requires the requested API version to be served", func() {
		crd.Spec.Versions[0].Served = false

		expectCRD()
		Expect(wait()).To(MatchError(ContainSubstring("pending: [widgets.example.com]")))
	})
	It("rejects a CRD serving a different API version", func() {
		crd.Spec.Versions[0].Name = "v1"

		expectCRD()
		Expect(wait()).To(MatchError(ContainSubstring("pending: [widgets.example.com]")))
	})
	It("returns access errors instead of retrying them as missing CRDs", func() {
		kube.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("access denied")).Once()
		Expect(wait()).To(MatchError(ContainSubstring("access denied")))
	})
})
