// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"testing"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/stretchr/testify/mock"
)

func TestPrerequisiteHTTPChartInstallsWithoutArgoCD(t *testing.T) {
	helm := installer.NewMockHelmClient(t)
	helm.EXPECT().FindRelease("rook-ceph", "rook-ceph").Return(nil, nil)
	helm.EXPECT().InstallChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
		return cfg.ChartName == "rook-ceph" && cfg.RepoURL == "https://charts.rook.io/release" &&
			cfg.Namespace == "rook-ceph" && cfg.CreateNamespace
	}), installer.InstallChartOptions{}).Return(nil)
	b := &LocalBootstrapper{ctx: context.Background(), helmClient: helm}
	if err := b.installPrerequisiteChart(prerequisiteChartConfig{
		Name: "rook-ceph", Chart: "rook-ceph", RepoURL: "https://charts.rook.io/release", Namespace: "rook-ceph",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPrerequisiteChartUpgradesExistingRelease(t *testing.T) {
	helm := installer.NewMockHelmClient(t)
	helm.EXPECT().FindRelease("rook-ceph", "rook-ceph").Return(&installer.ReleaseInfo{Name: "rook-ceph"}, nil)
	helm.EXPECT().UpgradeChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
		return cfg.ReleaseName == "rook-ceph" && cfg.Version == "1.0.0"
	}), installer.UpgradeChartOptions{}).Return(nil)
	b := &LocalBootstrapper{ctx: context.Background(), helmClient: helm}
	if err := b.installPrerequisiteChart(prerequisiteChartConfig{
		Name: "rook-ceph", Chart: "rook-ceph", RepoURL: "https://charts.rook.io/release",
		Namespace: "rook-ceph", TargetRevision: "1.0.0",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPrerequisiteChartRequiresHelmClient(t *testing.T) {
	b := &LocalBootstrapper{}
	if err := b.installPrerequisiteChart(prerequisiteChartConfig{Name: "test"}); err == nil || err.Error() != "helm client is not set" {
		t.Fatalf("expected missing Helm client error, got %v", err)
	}
}
