// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package bom_test

import (
	"github.com/codesphere-cloud/oms/internal/installer/bom"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("BOM index v3", func() {
	It("returns sorted unique image refs across components and skips empty refs", func() {
		index := &bom.IndexV3{Components: []bom.ComponentV2{
			{ContainerImages: map[string]bom.ContainerImageV2{
				"worker": {Ref: "ghcr.io/example/worker:v1"},
				"api":    {Ref: "ghcr.io/example/api:v1"},
			}},
			{ContainerImages: map[string]bom.ContainerImageV2{
				"duplicate": {Ref: "ghcr.io/example/api:v1"},
				"empty":     {},
			}},
		}}

		Expect(index.GetContainerImages()).To(Equal([]string{
			"ghcr.io/example/api:v1", "ghcr.io/example/worker:v1",
		}))
	})

	It("pins image refs to their declared digest and deduplicates the resulting refs", func() {
		index := &bom.IndexV3{Components: []bom.ComponentV2{{
			ContainerImages: map[string]bom.ContainerImageV2{
				"tagged":    {Ref: "ghcr.io/example/api:v1", Digest: "sha256:abc"},
				"duplicate": {Ref: "ghcr.io/example/api:v1@sha256:abc"},
				"empty":     {Digest: "sha256:abc"},
			},
		}}}

		Expect(index.GetContainerImages()).To(Equal([]string{"ghcr.io/example/api:v1@sha256:abc"}))
	})

	It("preserves a digest already in the ref when metadata declares a different digest", func() {
		index := &bom.IndexV3{Components: []bom.ComponentV2{{
			ContainerImages: map[string]bom.ContainerImageV2{
				"api": {Ref: "ghcr.io/example/api@sha256:abc", Digest: "sha256:def"},
			},
		}}}

		Expect(index.GetContainerImages()).To(Equal([]string{"ghcr.io/example/api@sha256:abc"}))
	})

	It("returns sorted unique charts from both sources after normalizing OCI schemes", func() {
		index := &bom.IndexV3{Components: []bom.ComponentV2{
			{
				HelmCharts: map[string]bom.HelmChartV2{
					"dependency": {Ref: "oci://ghcr.io/example/charts/dependency:v1"},
					"legacy":     {Ref: "ghcr.io/example/charts/app:v1"},
					"empty":      {},
				},
				Files: map[string]bom.FileRef{
					"chart": {OciRef: "oci://ghcr.io/example/charts/app:v1"},
					"local": {SrcPath: "script.sh"},
				},
			},
			{Files: map[string]bom.FileRef{
				"dependency": {OciRef: "ghcr.io/example/charts/dependency:v1"},
				"other":      {OciRef: "ghcr.io/example/charts/other:v1"},
			}},
		}}

		Expect(index.GetChartRefs()).To(Equal([]string{
			"ghcr.io/example/charts/app:v1",
			"ghcr.io/example/charts/dependency:v1",
			"ghcr.io/example/charts/other:v1",
		}))
	})

	It("parses v2 component names and artifact references", func() {
		index, err := bom.ParseIndex([]byte(`{
			"version": "v3", "components": [{
				"version": "2", "name": "app",
				"containerImages": {"api": {
					"ref": "ghcr.io/example/api:v1", "digest": "sha256:abc"
				}},
				"helmCharts": {"dependency": {"ref": "oci://ghcr.io/example/charts/dependency:v1"}},
				"files": {
					"chart": {"ociRef": "ghcr.io/example/charts/app:v1"}
				}
			}]
		}`))
		Expect(err).NotTo(HaveOccurred())
		Expect(index).To(Equal(&bom.IndexV3{
			Version: "v3",
			Components: []bom.ComponentV2{{
				Version: "2", Name: "app",
				ContainerImages: map[string]bom.ContainerImageV2{
					"api": {Ref: "ghcr.io/example/api:v1", Digest: "sha256:abc"},
				},
				HelmCharts: map[string]bom.HelmChartV2{
					"dependency": {Ref: "oci://ghcr.io/example/charts/dependency:v1"},
				},
				Files: map[string]bom.FileRef{
					"chart": {OciRef: "ghcr.io/example/charts/app:v1"},
				},
			}},
		}))
	})

	It("rejects unsupported index versions", func() {
		_, err := bom.ParseIndex([]byte(`{"version":"v4","components":[]}`))
		Expect(err).To(MatchError(`unsupported BOM index version "v4": expected v3`))
	})

	It("rejects a missing index version", func() {
		_, err := bom.ParseIndex([]byte(`{"components":[]}`))
		Expect(err).To(MatchError(`unsupported BOM index version "": expected v3`))
	})

	It("rejects unsupported components even when preceding components use v2", func() {
		_, err := bom.ParseIndex([]byte(`{
			"version":"v3", "components":[{"name":"app","version":"2"},{"name":"dependency","version":"3"}]
		}`))
		Expect(err).To(MatchError(`unsupported BOM component "dependency" version "3": expected 2`))
	})

	It("rejects a missing component version", func() {
		_, err := bom.ParseIndex([]byte(`{"version":"v3","components":[{"name":"app"}]}`))
		Expect(err).To(MatchError(`unsupported BOM component "app" version "": expected 2`))
	})
})
