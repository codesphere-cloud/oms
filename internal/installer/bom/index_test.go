// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package bom_test

import (
	"github.com/codesphere-cloud/oms/internal/installer/bom"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("BOM index v3", func() {
	It("parses v2 components with all reference and file metadata", func() {
		index, err := bom.ParseIndex([]byte(`{
			"version": "v3", "components": [{
				"version": "2", "name": "app",
				"containerImages": {"api": {
					"ref": "ghcr.io/example/api:v1", "digest": "sha256:abc",
					"sources": ["Deployment/api spec.containers[0]"]
				}},
				"helmCharts": {"dependency": {"ref": "oci://ghcr.io/example/charts/dependency:v1"}},
				"files": {
					"chart": {"ociRef": "ghcr.io/example/charts/app:v1"},
					"script": {"srcPath": "bin/script.sh", "executable": true},
					"download": {"srcUrl": "https://example.com/file"},
					"assets": {"glob": {"cwd": "assets", "include": "**/*", "exclude": ["*.tmp"]}}
				}
			}]
		}`))
		Expect(err).NotTo(HaveOccurred())
		Expect(index).To(Equal(&bom.IndexV3{
			Version: "v3",
			Components: []bom.ComponentV2{{
				Version: "2", Name: "app",
				ContainerImages: map[string]bom.ContainerImageV2{
					"api": {Ref: "ghcr.io/example/api:v1", Digest: "sha256:abc", Sources: []string{"Deployment/api spec.containers[0]"}},
				},
				HelmCharts: map[string]bom.HelmChartV2{
					"dependency": {Ref: "oci://ghcr.io/example/charts/dependency:v1"},
				},
				Files: map[string]bom.FileRef{
					"chart":    {OciRef: "ghcr.io/example/charts/app:v1"},
					"script":   {SrcPath: "bin/script.sh", Executable: true},
					"download": {SrcUrl: "https://example.com/file"},
					"assets":   {Glob: &bom.GlobRef{Cwd: "assets", Include: "**/*", Exclude: []string{"*.tmp"}}},
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
