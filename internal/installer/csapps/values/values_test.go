// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package values_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"

	csappsvalues "github.com/codesphere-cloud/oms/internal/installer/csapps/values"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

const authServiceDigest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

var _ = Describe("Build", func() {
	It("minimal config -> installer defaults", func() {
		values := build(testConfig(), testBOM(nil))

		Expect(at(values, "global.dnsServers")).To(Equal([]any{"8.8.8.8"}))
		Expect(at(values, "global.prePullImages")).To(BeTrue())
		Expect(at(values, "global.managedServiceWorkspacePlanId")).To(Equal(20))
		Expect(at(values, "global.customDomains.gatewayApi.useGateway")).To(BeTrue())
		Expect(at(values, "global.imageTag")).To(Equal("v1.2.3"))
		Expect(at(values, "global.hostName")).To(Equal("codesphere.example.com"))
	})

	It("auth-service image without a tag -> release tag error", func() {
		untagged := map[string]string{"auth-service": imagePrefix + "auth-service@" + authServiceDigest}

		_, err := csappsvalues.Build(testConfig(), testBOM(untagged))

		Expect(err).To(MatchError(ContainSubstring("missing a tag")))
	})

	It("override -> wins over the installer's values, null kept for Helm", func() {
		cfg := testConfig()
		cfg.Codesphere.Override = files.ChartOverride{"global": map[string]any{
			"prePullImages": false,
			"metrics":       nil,
			"hostName":      "override.example.com",
			"imageTag":      "from-override",
		}}
		values := build(cfg, testBOM(nil))

		Expect(at(values, "global.prePullImages")).To(BeFalse())
		Expect(at(values, "global")).To(HaveKeyWithValue("metrics", BeNil()))
		Expect(at(values, "global.hostName")).To(Equal("override.example.com"))
		Expect(at(values, "global.imageTag")).To(Equal("v1.2.3"))
	})

	It("override of one workspace env var -> the others stay", func() {
		cfg := testConfig()
		cfg.Codesphere.ExtraWorkspaceEnvVars = map[string]string{"FOO": "foo", "BAR": "bar"}
		cfg.Codesphere.Override = files.ChartOverride{"global": map[string]any{
			"extraWorkspaceEnvVars": map[string]any{"FOO": "changed"},
		}}
		values := build(cfg, testBOM(nil))

		Expect(at(values, "global.extraWorkspaceEnvVars")).To(Equal(map[string]any{
			"FOO": "changed",
			"BAR": "bar",
		}))
	})

	It("override with numeric keys -> merged into the built map", func() {
		cfg := testConfig()
		cfg.Codesphere.Plans.HostingPlans = map[int]files.HostingPlan{
			20: {CPUTenth: 10, MemoryMb: 2048, StorageMb: 20480, TempStorageMb: 1500, PooledInstances: ptr.To(1)},
			21: {CPUTenth: 20, MemoryMb: 4096, StorageMb: 20480, TempStorageMb: 1500, PooledInstances: ptr.To(2)},
		}
		// YAML decodes the numeric pool keys of an override to a map with int keys.
		cfg.Codesphere.Override = files.ChartOverride{"global": map[string]any{
			"deployConfig": map[string]any{"images": map[string]any{"ubuntu-24.04": map[string]any{
				"flavors": map[string]any{"default": map[string]any{"pool": map[any]any{20: 5}}},
			}}},
		}}
		values := build(cfg, testBOM(nil))

		ubuntu := at(values, "global.deployConfig.images").(map[string]any)["ubuntu-24.04"]
		flavors := ubuntu.(map[string]any)["flavors"].(map[string]any)
		Expect(flavors["default"]).To(HaveKeyWithValue("pool", And(
			HaveKeyWithValue("20", 5),
			HaveKeyWithValue("21", 2),
		)))
	})

	It("override -> config is not changed through the values", func() {
		cfg := testConfig()
		cfg.Codesphere.Override = files.ChartOverride{"global": map[string]any{
			"ceph": map[string]any{"storageClass": "rbd"},
		}}
		values := build(cfg, testBOM(nil))
		at(values, "global.ceph").(map[string]any)["storageClass"] = "changed"

		globalOverride := cfg.Codesphere.Override["global"].(map[string]any)
		Expect(globalOverride["ceph"]).To(Equal(map[string]any{"storageClass": "rbd"}))
	})
})
