// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package argocd_test

import (
	"context"
	"fmt"
	"strings"
	"time"

	argov1alpha1 "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/codesphere-cloud/oms/internal/installer/argocd"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Argo CD application readiness", func() {
	var (
		kube    *MockClient
		ctx     context.Context
		cancel  context.CancelFunc
		source  argov1alpha1.ApplicationSource
		app     *argov1alpha1.Application
		lastLog string
	)

	BeforeEach(func() {
		kube = NewMockClient(GinkgoT())
		ctx, cancel = context.WithCancel(context.Background())
		DeferCleanup(cancel)

		lastLog = ""
		source = argov1alpha1.ApplicationSource{
			RepoURL: "mirror.example.com/charts", Chart: "postgres-operator", TargetRevision: "1.2.3",
			Helm: &argov1alpha1.ApplicationSourceHelm{ReleaseName: "cnpg", ValuesObject: &runtime.RawExtension{Raw: []byte(`{"image":"mirror.example.com/cnpg:1.30.0","enabled":true}`)}},
		}
		app = &argov1alpha1.Application{}
		app.Status.Health.Status = "Healthy"
		app.Status.Sync.Status = argov1alpha1.SyncStatusCodeSynced
		app.Status.Sync.ComparedTo.Source = *source.DeepCopy()
	})

	expectStatus := func() {
		GinkgoHelper()
		kube.EXPECT().Get(mock.Anything, client.ObjectKey{Namespace: "argocd", Name: "cnpg"}, mock.Anything).RunAndReturn(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
			*obj.(*argov1alpha1.Application) = *app.DeepCopy()
			return nil
		}).Once()
	}
	waitForStatus := func() error {
		GinkgoHelper()

		return argocd.WaitForApplicationHealthy(ctx, kube, "cnpg", source, 0, func(format string, args ...interface{}) {
			if strings.Contains(format, "source-matches=") {
				lastLog = fmt.Sprintf(format, args...)
				// Cancel after the first rejected status so tests never wait for a poll interval.
				cancel()
			}
		})
	}

	It("accepts healthy synced status for the full source despite JSON formatting differences", func() {
		app.Status.Sync.ComparedTo.Source.Helm.ValuesObject.Raw = []byte(`{ "enabled": true, "image": "mirror.example.com/cnpg:1.30.0" }`)

		expectStatus()
		Expect(waitForStatus()).To(Succeed())
	})

	It("rejects old Helm values even when the chart revision is unchanged", func() {
		app.Status.Sync.ComparedTo.Source.Helm.ValuesObject.Raw = []byte(`{"image":"old.example.com/cnpg:1.29.0","enabled":true}`)

		expectStatus()
		Expect(waitForStatus()).To(MatchError(ContainSubstring("source-matches=false")))
		Expect(lastLog).To(ContainSubstring("source-matches=false"))
	})

	It("rejects status for a different repository at the same revision", func() {
		app.Status.Sync.ComparedTo.Source.RepoURL = "old.example.com/charts"

		expectStatus()
		Expect(waitForStatus()).To(MatchError(ContainSubstring("source-matches=false")))
	})

	It("rejects status for a different chart at the same revision", func() {
		app.Status.Sync.ComparedTo.Source.Chart = "other-operator"

		expectStatus()
		Expect(waitForStatus()).To(MatchError(ContainSubstring("source-matches=false")))
	})

	It("keeps waiting when the requested source has not synced", func() {
		app.Status.Sync.Status = argov1alpha1.SyncStatusCodeOutOfSync

		expectStatus()
		Expect(waitForStatus()).To(MatchError(ContainSubstring("sync=\"OutOfSync\"")))
		Expect(lastLog).To(ContainSubstring("source-matches=true"))
	})

	It("keeps waiting when the requested source is not healthy", func() {
		app.Status.Health.Status = "Progressing"

		expectStatus()
		Expect(waitForStatus()).To(MatchError(ContainSubstring("health=\"Progressing\"")))
		Expect(lastLog).To(ContainSubstring("source-matches=true"))
	})

	It("reports the configured timeout when reading application status times out", func() {
		kube.EXPECT().Get(mock.Anything, client.ObjectKey{Namespace: "argocd", Name: "cnpg"}, mock.Anything).Return(context.DeadlineExceeded).Once()

		var firstLog string

		err := argocd.WaitForApplicationHealthy(ctx, kube, "cnpg", source, 20*time.Millisecond, func(format string, args ...interface{}) {
			if firstLog == "" {
				firstLog = fmt.Sprintf(format, args...)
			}
		})
		Expect(err).To(MatchError(ContainSubstring("timed out waiting")))
		Expect(err).To(MatchError(ContainSubstring("context deadline exceeded")))
		Expect(firstLog).To(ContainSubstring("timeout 20ms"))
	})
})
