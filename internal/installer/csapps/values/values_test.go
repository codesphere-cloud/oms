// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package values_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	csappsvalues "github.com/codesphere-cloud/oms/internal/installer/csapps/values"
)

const authServiceDigest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

var _ = Describe("Build", func() {
	It("minimal config -> installer defaults", func() {
		values := build(testConfig(""), testBOM(nil))

		Expect(at(values, "global.dnsServers")).To(Equal([]string{"8.8.8.8"}))
		Expect(at(values, "global.prePullImages")).To(BeTrue())
		Expect(at(values, "global.managedServiceWorkspacePlanId")).To(Equal(20))
		Expect(at(values, "global.ceph.mdsNamespace")).To(Equal("codesphere"))
		Expect(at(values, "global.ceph.activeMds")).To(Equal("7"))
		Expect(at(values, "global.otel.centralOtelExport.otlpHttpEndpoint")).
			To(Equal("https://otel.tools.codesphere.com"))
		Expect(at(values, "global.frontendGateway.issuer")).To(Equal("self-signed"))
		Expect(at(values, "global.frontendGateway.gatewayApi.enabled")).To(BeTrue())
		Expect(at(values, "global.customDomains.gatewayApi.useGateway")).To(BeTrue())
		Expect(at(values, "global.experiments")).To(HaveKey("gateway-domains"))
		Expect(at(values, "global.certIssuer")).To(Equal(map[string]any{
			"acme": map[string]any{"enabled": false},
		}))
		Expect(at(values, "global.imageTag")).To(Equal("v1.2.3"))
		Expect(at(values, "global.hostName")).To(Equal("codesphere.example.com"))
	})

	It("auth-service image without a tag -> release tag error", func() {
		untagged := map[string]string{"auth-service": imagePrefix + "auth-service@" + authServiceDigest}

		_, err := csappsvalues.Build(testConfig(""), testBOM(untagged))

		Expect(err).To(MatchError(ContainSubstring("missing a tag")))
	})

	It("override -> wins over the installer's values, null kept for Helm", func() {
		values := build(testConfig(`
			codesphere:
			  override:
			    global:
			      prePullImages: false
			      metrics: null
			      hostName: override.example.com
			      imageTag: from-override
		`), testBOM(nil))

		Expect(at(values, "global.prePullImages")).To(BeFalse())
		Expect(at(values, "global")).To(HaveKeyWithValue("metrics", BeNil()))
		Expect(at(values, "global.hostName")).To(Equal("override.example.com"))
		Expect(at(values, "global.imageTag")).To(Equal("v1.2.3"))
	})

	It("override of one workspace env var -> the others stay", func() {
		values := build(testConfig(`
			codesphere:
			  extraWorkspaceEnvVars: {FOO: foo, BAR: bar}
			  override:
			    global:
			      extraWorkspaceEnvVars: {FOO: changed}
		`), testBOM(nil))

		Expect(at(values, "global.extraWorkspaceEnvVars")).To(Equal(map[string]any{
			"FOO": "changed",
			"BAR": "bar",
		}))
	})

	It("override with numeric keys -> merged into the built map", func() {
		values := build(testConfig(`
			codesphere:
			  plans:
			    hostingPlans:
			      20: {cpuTenth: 10, gpuParts: 0, memoryMb: 2048, storageMb: 20480, tempStorageMb: 1500, pooledInstances: 1}
			      21: {cpuTenth: 20, gpuParts: 0, memoryMb: 4096, storageMb: 20480, tempStorageMb: 1500, pooledInstances: 2}
			    workspacePlans: {}
			  override:
			    global:
			      deployConfig:
			        images:
			          ubuntu-24.04:
			            flavors:
			              default:
			                pool: {20: 5}
		`), testBOM(nil))

		ubuntu := at(values, "global.deployConfig.images").(map[string]any)["ubuntu-24.04"]
		flavors := ubuntu.(map[string]any)["flavors"].(map[string]any)
		Expect(flavors["default"]).To(HaveKeyWithValue("pool", And(
			HaveKeyWithValue("20", 5),
			HaveKeyWithValue("21", 2),
		)))
	})

	It("override -> config is not changed through the values", func() {
		cfg := testConfig(`
			codesphere:
			  override:
			    global:
			      ceph: {storageClass: rbd}
		`)
		values := build(cfg, testBOM(nil))
		at(values, "global.ceph").(map[string]any)["storageClass"] = "changed"

		globalOverride := cfg.Codesphere.Override["global"].(map[string]any)
		Expect(globalOverride["ceph"]).To(Equal(map[string]any{"storageClass": "rbd"}))
	})
})
