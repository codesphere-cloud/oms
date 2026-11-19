// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/argocd/pcapps"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("KyvernoPoliciesPcAppsValues", func() {
	It("returns nil when Kyverno is not configured", func() {
		Expect(pcapps.KyvernoPoliciesPcAppsValues(&files.RootConfig{})).To(BeNil())
	})

	It("returns nil when no policies override is configured", func() {
		config := &files.RootConfig{Cluster: files.ClusterConfig{Kyverno: &files.KyvernoConfig{Enabled: new(true)}}}

		Expect(pcapps.KyvernoPoliciesPcAppsValues(config)).To(BeNil())
	})

	It("passes the policies override to the kyverno-policies application", func() {
		override := files.ChartOverride{"pdbPolicy": map[string]any{"failureAction": "Audit"}}
		config := &files.RootConfig{Cluster: files.ClusterConfig{Kyverno: &files.KyvernoConfig{PoliciesOverride: override}}}

		Expect(pcapps.KyvernoPoliciesPcAppsValues(config)).To(Equal(files.ChartValues{
			"applications": files.ChartValues{
				"kyverno-policies": files.ChartValues{"valuesObject": override},
			},
		}))
	})
})
