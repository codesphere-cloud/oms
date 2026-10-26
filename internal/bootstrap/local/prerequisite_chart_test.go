// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"

	"github.com/codesphere-cloud/oms/internal/installer"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
)

var _ = Describe("prerequisite charts", func() {
	It("installs an HTTP chart before ArgoCD is available", func() {
		helm := installer.NewMockHelmClient(GinkgoT())
		helm.EXPECT().FindRelease("rook-ceph", "rook-ceph").Return(nil, nil)
		helm.EXPECT().InstallChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
			return cfg.ChartName == "rook-ceph" && cfg.RepoURL == "https://charts.rook.io/release" &&
				cfg.Namespace == "rook-ceph" && cfg.CreateNamespace
		}), installer.InstallChartOptions{}).Return(nil)
		b := &LocalBootstrapper{ctx: context.Background(), helmClient: helm}
		Expect(b.installPrerequisiteChart(prerequisiteChartConfig{
			Name: "rook-ceph", Chart: "rook-ceph", RepoURL: "https://charts.rook.io/release", Namespace: "rook-ceph",
		})).To(Succeed())
	})

	It("upgrades an existing release", func() {
		helm := installer.NewMockHelmClient(GinkgoT())
		helm.EXPECT().FindRelease("rook-ceph", "rook-ceph").Return(&installer.ReleaseInfo{Name: "rook-ceph"}, nil)
		helm.EXPECT().UpgradeChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
			return cfg.ReleaseName == "rook-ceph" && cfg.Version == "1.0.0"
		}), installer.UpgradeChartOptions{ForceConflicts: true, TakeOwnership: true}).Return(nil)
		b := &LocalBootstrapper{ctx: context.Background(), helmClient: helm}
		Expect(b.installPrerequisiteChart(prerequisiteChartConfig{
			Name: "rook-ceph", Chart: "rook-ceph", RepoURL: "https://charts.rook.io/release",
			Namespace: "rook-ceph", TargetRevision: "1.0.0",
		})).To(Succeed())
	})

	It("requires a Helm client", func() {
		b := &LocalBootstrapper{}
		Expect(b.installPrerequisiteChart(prerequisiteChartConfig{Name: "test"})).To(MatchError("helm client is not set"))
	})
})
