// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package argocd_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/argocd"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func packagedChart(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	files := map[string]string{
		"Chart.yaml":                "apiVersion: v2\nname: argocd\nversion: 2.3.0\ndependencies:\n- name: argo-cd\n  version: 10.3.2\n  repository: https://argoproj.github.io/argo-helm\n",
		"values.yaml":               "extraSecrets:\n- secretName: argocd-dex-client\nargo-cd:\n  global:\n    image:\n      repository: ghcr.io/codesphere-cloud/docker/quay.io/argoproj/argocd\n  dex:\n    image:\n      repository: ghcr.io/codesphere-cloud/docker/ghcr.io/dexidp/dex\n  redis:\n    image:\n      repository: ghcr.io/codesphere-cloud/docker/redis\n",
		"charts/argo-cd/Chart.yaml": "apiVersion: v2\nname: argo-cd\nversion: 10.3.2\nappVersion: v3.5.0\n",
	}
	for path, content := range files {
		target := filepath.Join(dir, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o700))
		require.NoError(t, os.WriteFile(target, []byte(content), 0o600))
	}

	return dir
}

func TestPackagedArgoCDReleasePolicy(t *testing.T) {
	for _, tc := range []struct {
		name, chart, version string
		upgrade              bool
	}{
		{"upgrade Codesphere chart", "argocd", "2.2.0", true},
		{"retain equal", "argocd", "2.3.0", false},
		{"retain newer", "argocd", "2.4.0", false},
		{"preserve upstream", "argo-cd", "10.3.2", false},
		{"preserve unknown", "", "unparseable", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			helm := installer.NewMockHelmClient(t)
			resources := argocd.NewMockResources(t)
			a := &argocd.Installer{InstallerConfig: argocd.InstallerConfig{PackageChart: packagedChart(t), Version: "2.3.0", FullInstall: true}, Helm: helm, Resources: resources}
			helm.EXPECT().FindRelease("argocd", "argocd").Return(&installer.ReleaseInfo{Name: "argocd", ChartName: tc.chart, InstalledVersion: tc.version}, nil)

			if tc.upgrade {
				helm.EXPECT().UpgradeChart(mock.Anything, mock.MatchedBy(func(c installer.ChartConfig) bool {
					return c.ChartName == a.PackageChart && c.Version == "2.3.0" && c.RepoURL == ""
				}), mock.Anything).Return(nil)
			}

			resources.EXPECT().ApplyAll(mock.Anything).Return(nil)
			require.NoError(t, a.Install())
		})
	}
}

func TestPackagedArgoCDFreshInstallValues(t *testing.T) {
	helm := installer.NewMockHelmClient(t)
	ca := filepath.Join(t.TempDir(), "ca.pem")
	require.NoError(t, os.WriteFile(ca, []byte("test certificate"), 0o600))
	a := &argocd.Installer{InstallerConfig: argocd.InstallerConfig{PackageChart: packagedChart(t), Version: "2.3.0", ImageRegistry: "mirror.example.com", OciRegistryURL: "mirror.example.com/charts", OciRegistryCAFile: ca}, Helm: helm, Clientset: fake.NewClientset()}
	helm.EXPECT().FindRelease("argocd", "argocd").Return(nil, nil)
	helm.EXPECT().InstallChart(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, c installer.ChartConfig, _ installer.InstallChartOptions) error {
		require.Equal(t, a.PackageChart, c.ChartName)
		require.Empty(t, c.RepoURL)
		require.Empty(t, c.Values["extraSecrets"])
		upstream := c.Values["argo-cd"].(map[string]any)
		require.Equal(t, false, upstream["dex"].(map[string]any)["enabled"])
		image := upstream["global"].(map[string]any)["image"].(map[string]any)
		require.Equal(t, "mirror.example.com/ghcr_io/codesphere-cloud/docker/quay.io/argoproj/argocd", image["repository"])

		certs := upstream["configs"].(map[string]any)["tls"].(map[string]any)["certificates"].(map[string]any)
		require.Equal(t, "test certificate", certs["mirror.example.com"])

		return nil
	})
	require.NoError(t, a.Install())
}

func TestPackagedArgoCDPreservesUnmanagedWorkloads(t *testing.T) {
	for _, object := range []struct {
		name     string
		stateful bool
	}{{"argocd-server", false}, {"argocd-application-controller", true}} {
		t.Run(object.name, func(t *testing.T) {
			helm := installer.NewMockHelmClient(t)
			resources := argocd.NewMockResources(t)
			client := fake.NewClientset()

			meta := metav1.ObjectMeta{Name: object.name, Namespace: "argocd"}
			if object.stateful {
				_, err := client.AppsV1().StatefulSets("argocd").Create(context.Background(), &appsv1.StatefulSet{ObjectMeta: meta}, metav1.CreateOptions{})
				require.NoError(t, err)
			} else {
				_, err := client.AppsV1().Deployments("argocd").Create(context.Background(), &appsv1.Deployment{ObjectMeta: meta}, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			a := &argocd.Installer{InstallerConfig: argocd.InstallerConfig{PackageChart: "unused", Version: "2.3.0", FullInstall: true}, Helm: helm, Resources: resources, Clientset: client}
			helm.EXPECT().FindRelease("argocd", "argocd").Return(nil, nil)
			resources.EXPECT().ApplyAll(mock.Anything).Return(nil)
			require.NoError(t, a.Install())
		})
	}
}

func TestPackagedArgoCDRejectsInvalidPackage(t *testing.T) {
	for _, tc := range []struct {
		name    string
		change  func(*argocd.Installer)
		message string
	}{
		{"missing chart", func(a *argocd.Installer) { a.PackageChart = filepath.Join(t.TempDir(), "missing") }, "loading packaged"},
		{"BOM mismatch", func(a *argocd.Installer) { a.Version = "2.4.0" }, "does not match BOM"},
		{"missing dependency", func(a *argocd.Installer) { require.NoError(t, os.RemoveAll(filepath.Join(a.PackageChart, "charts"))) }, "missing its bundled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			helm := installer.NewMockHelmClient(t)
			a := &argocd.Installer{InstallerConfig: argocd.InstallerConfig{PackageChart: packagedChart(t), Version: "2.3.0"}, Helm: helm, Clientset: fake.NewClientset()}
			tc.change(a)
			helm.EXPECT().FindRelease("argocd", "argocd").Return(nil, nil)
			require.ErrorContains(t, a.Install(), tc.message)
		})
	}
}

func TestPackagedArgoCDUserValuesTakePrecedence(t *testing.T) {
	helm := installer.NewMockHelmClient(t)
	dir := t.TempDir()
	values := filepath.Join(dir, "values.yaml")
	require.NoError(t, os.WriteFile(values, []byte("argo-cd:\n  dex:\n    enabled: true\n  global:\n    image:\n      repository: custom.example.com/argocd\n"), 0o600))
	a := &argocd.Installer{InstallerConfig: argocd.InstallerConfig{PackageChart: packagedChart(t), Version: "2.3.0", ImageRegistry: "mirror.example.com", ValueFiles: []string{values}}, Helm: helm, Clientset: fake.NewClientset()}
	helm.EXPECT().FindRelease("argocd", "argocd").Return(nil, nil)
	helm.EXPECT().InstallChart(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, c installer.ChartConfig, _ installer.InstallChartOptions) error {
		upstream := c.Values["argo-cd"].(map[string]any)
		require.Equal(t, true, upstream["dex"].(map[string]any)["enabled"])
		require.Equal(t, "custom.example.com/argocd", upstream["global"].(map[string]any)["image"].(map[string]any)["repository"])

		return nil
	})
	require.NoError(t, a.Install())
}
