// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/codesphere-cloud/oms/internal/installer"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
)

var _ = Describe("PCApps.Install", func() {
	const (
		chartURL  = "oci://ghcr.io/codesphere-cloud/charts/pc-apps"
		version   = "1.2.3"
		namespace = "argocd"
		username  = "CodesphereBot"
		password  = "super-secret-token"
	)

	var (
		helmMock *installer.MockHelmClient
		pcApps   *installer.PCApps
	)

	BeforeEach(func() {
		helmMock = installer.NewMockHelmClient(GinkgoT())
		pcApps = &installer.PCApps{
			ChartURL:  chartURL,
			Version:   version,
			Namespace: namespace,
			Username:  username,
			Password:  password,
			Helm:      helmMock,
		}
	})

	Context("fresh install (no existing release)", func() {
		BeforeEach(func() {
			helmMock.EXPECT().LoginRegistry(mock.Anything, "ghcr.io", username, password).Return(nil)
			helmMock.EXPECT().FindRelease(namespace, "pc-apps").Return(nil, nil)
		})

		It("logs in to registry and installs the chart", func() {
			helmMock.EXPECT().InstallChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
				return cfg.ReleaseName == "pc-apps" &&
					cfg.ChartName == chartURL &&
					cfg.Namespace == namespace &&
					cfg.Version == version &&
					cfg.CreateNamespace == true
			})).Return(nil)

			err := pcApps.Install(context.Background())
			Expect(err).ToNot(HaveOccurred())
		})

		It("installs latest when version is empty", func() {
			pcApps.Version = ""
			helmMock.EXPECT().InstallChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
				return cfg.Version == ""
			})).Return(nil)

			err := pcApps.Install(context.Background())
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error when InstallChart fails", func() {
			helmMock.EXPECT().InstallChart(mock.Anything, mock.Anything).
				Return(errors.New("timeout"))

			err := pcApps.Install(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("install failed"))
		})
	})

	Context("upgrade (existing release found)", func() {
		BeforeEach(func() {
			helmMock.EXPECT().LoginRegistry(mock.Anything, "ghcr.io", username, password).Return(nil)
			helmMock.EXPECT().FindRelease(namespace, "pc-apps").Return(&installer.ReleaseInfo{
				Name:             "pc-apps",
				InstalledVersion: "1.0.0",
			}, nil)
		})

		It("upgrades the existing release", func() {
			helmMock.EXPECT().UpgradeChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
				return cfg.ReleaseName == "pc-apps" &&
					cfg.ChartName == chartURL &&
					cfg.Version == version
			}), installer.UpgradeChartOptions{}).Return(nil)

			err := pcApps.Install(context.Background())
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error when UpgradeChart fails", func() {
			helmMock.EXPECT().UpgradeChart(mock.Anything, mock.Anything, mock.Anything).
				Return(errors.New("upgrade conflict"))

			err := pcApps.Install(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("upgrade failed"))
		})
	})

	Context("registry login failure", func() {
		It("returns an error without attempting install", func() {
			helmMock.EXPECT().LoginRegistry(mock.Anything, "ghcr.io", username, password).
				Return(errors.New("invalid credentials"))

			err := pcApps.Install(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("registry login failed"))
		})
	})

	Context("invalid chart URL", func() {
		It("rejects URLs without oci:// prefix", func() {
			pcApps.ChartURL = "https://ghcr.io/codesphere-cloud/charts/pc-apps"

			err := pcApps.Install(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("must start with \"oci://\""))
		})

		It("rejects URLs with no host", func() {
			pcApps.ChartURL = "oci:///charts/pc-apps"

			err := pcApps.Install(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no host"))
		})
	})

	Context("values files", func() {
		var tmpDir string

		BeforeEach(func() {
			var err error
			tmpDir, err = os.MkdirTemp("", "pc-apps-test-*")
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			os.RemoveAll(tmpDir)
		})

		It("merges multiple values files in order", func() {
			base := filepath.Join(tmpDir, "base.yaml")
			Expect(os.WriteFile(base, []byte("foo: bar\nnested:\n  a: 1\n  b: 2\n"), 0644)).To(Succeed())

			overlay := filepath.Join(tmpDir, "overlay.yaml")
			Expect(os.WriteFile(overlay, []byte("foo: overridden\nnested:\n  b: 99\n  c: 3\n"), 0644)).To(Succeed())

			pcApps.ValuesFiles = []string{base, overlay}

			helmMock.EXPECT().LoginRegistry(mock.Anything, "ghcr.io", username, password).Return(nil)
			helmMock.EXPECT().FindRelease(namespace, "pc-apps").Return(nil, nil)
			helmMock.EXPECT().InstallChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
				nested, ok := cfg.Values["nested"].(map[string]interface{})
				return ok &&
					cfg.Values["foo"] == "overridden" &&
					nested["a"] == 1 &&
					nested["b"] == 99 &&
					nested["c"] == 3
			})).Return(nil)

			err := pcApps.Install(context.Background())
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error for non-existent values file", func() {
			pcApps.ValuesFiles = []string{"/nonexistent/values.yaml"}

			helmMock.EXPECT().LoginRegistry(mock.Anything, "ghcr.io", username, password).Return(nil)

			err := pcApps.Install(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("reading values file"))
		})

		It("returns an error for invalid YAML in values file", func() {
			badFile := filepath.Join(tmpDir, "bad.yaml")
			Expect(os.WriteFile(badFile, []byte("{{invalid yaml"), 0644)).To(Succeed())

			pcApps.ValuesFiles = []string{badFile}

			helmMock.EXPECT().LoginRegistry(mock.Anything, "ghcr.io", username, password).Return(nil)

			err := pcApps.Install(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("parsing values file"))
		})
	})
})

var _ = Describe("LoadAndMergeValues", func() {
	var tmpDir string

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "merge-values-test-*")
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		os.RemoveAll(tmpDir)
	})

	It("returns empty map for no files", func() {
		result, err := installer.LoadAndMergeValues(nil)
		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(BeEmpty())
	})

	It("returns values from a single file", func() {
		f := filepath.Join(tmpDir, "values.yaml")
		Expect(os.WriteFile(f, []byte("key: value\n"), 0644)).To(Succeed())

		result, err := installer.LoadAndMergeValues([]string{f})
		Expect(err).ToNot(HaveOccurred())
		Expect(result["key"]).To(Equal("value"))
	})

	It("deep merges nested maps from multiple files", func() {
		f1 := filepath.Join(tmpDir, "a.yaml")
		Expect(os.WriteFile(f1, []byte("top:\n  a: 1\n  b: 2\n"), 0644)).To(Succeed())

		f2 := filepath.Join(tmpDir, "b.yaml")
		Expect(os.WriteFile(f2, []byte("top:\n  b: 99\n  c: 3\n"), 0644)).To(Succeed())

		result, err := installer.LoadAndMergeValues([]string{f1, f2})
		Expect(err).ToNot(HaveOccurred())

		top, ok := result["top"].(map[string]interface{})
		Expect(ok).To(BeTrue())
		Expect(top["a"]).To(Equal(1))
		Expect(top["b"]).To(Equal(99))
		Expect(top["c"]).To(Equal(3))
	})
})
