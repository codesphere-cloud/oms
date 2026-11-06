// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package argocd

import (
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("AppInstaller getPcAppsValues", func() {
	var (
		installerConfig map[string]any
		config          files.RootConfig
		skipSteps       []string
	)

	// vaultWithRegistryPassword is the vault cluster-config needs: its registry credentials come from it.
	vaultWithRegistryPassword := func() *files.InstallVault {
		return &files.InstallVault{Secrets: []files.SecretEntry{
			{Name: files.SecretRegistryPassword, Fields: &files.SecretFields{Password: "registry-password"}},
		}}
	}
	pcAppsValues := func() files.ChartValues {
		install := NewAppInstaller(AppInstallerConfig{
			Config:          config,
			Vault:           vaultWithRegistryPassword(),
			InstallerConfig: installerConfig,
			SkipSteps:       skipSteps,
		})

		values, err := install.getPcAppsValues()
		Expect(err).NotTo(HaveOccurred())

		return values
	}
	application := func(chart string) files.ChartValues {
		applications, _ := pcAppsValues()["applications"].(files.ChartValues)
		app, _ := applications[chart].(files.ChartValues)

		return app
	}

	BeforeEach(func() {
		config = files.RootConfig{}
		skipSteps = nil
		installerConfig = map[string]any{
			"ceph":    map[string]any{"hosts": []any{map[string]any{"ipAddress": "10.0.0.11"}}},
			"cluster": map[string]any{"rgw": map[string]any{}},
		}
	})

	It("turns off an installer chart the config does not ask for", func() {
		delete(installerConfig, "cluster")

		Expect(application("rgw")).To(Equal(files.ChartValues{"enabled": false}))
	})

	It("leaves the enablement of an installer chart the config asks for to the pc-applications default", func() {
		Expect(application("rgw-load-balancer")).ToNot(HaveKey("enabled"))
		Expect(application("ms-backend-s3")).ToNot(HaveKey("enabled"))
	})

	It("derives the values of an installer chart the config asks for", func() {
		Expect(application("rgw-load-balancer")).To(HaveKey("valuesObject"))
		Expect(application("ms-backend-s3")).To(HaveKey("valuesObject"))
	})

	Context("with a pcApps block in the config", func() {
		BeforeEach(func() {
			config.PcApps = files.ChartValues{"applications": files.ChartValues{"rgw-load-balancer": files.ChartValues{
				"enabled":      false,
				"valuesObject": files.ChartValues{"externalRgwEndpoints": []any{map[string]any{"ip": "10.9.9.9"}}},
			}}}
		})

		It("lets the block's enablement win over the derived one", func() {
			Expect(application("rgw-load-balancer")).To(HaveKeyWithValue("enabled", false))
		})

		It("lets the block's values win over the derived values", func() {
			Expect(application("rgw-load-balancer")["valuesObject"]).To(HaveKeyWithValue("externalRgwEndpoints", []any{map[string]any{"ip": "10.9.9.9"}}))
		})
	})

	Context("when the run skips an installer step", func() {
		It("turns off ms-backend-s3 for --skip-steps ms-backends", func() {
			skipSteps = []string{installer.MsBackendsStep}

			Expect(application("ms-backend-s3")).To(Equal(files.ChartValues{"enabled": false}))
			Expect(application("rgw-load-balancer")).ToNot(HaveKey("enabled"))
		})

		It("turns off rgw and its load balancer for operations.skip set-up-cluster", func() {
			config.Operations = &files.OperationsConfig{Skip: []string{installer.SetUpClusterStep}}

			Expect(application("rgw")).To(Equal(files.ChartValues{"enabled": false}))
			Expect(application("rgw-load-balancer")).To(Equal(files.ChartValues{"enabled": false}))
			Expect(application("ms-backend-s3")).ToNot(HaveKey("enabled"))
		})
	})
})
