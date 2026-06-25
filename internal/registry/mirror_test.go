// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package registry_test

import (
	"errors"

	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/registry"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type fakeCopier struct {
	calls [][2]string
	err   error
}

func (c *fakeCopier) Copy(sourceRef string, destinationRef string) error {
	c.calls = append(c.calls, [2]string{sourceRef, destinationRef})
	return c.err
}

var _ = Describe("TargetImageRef", func() {
	It("maps a Codesphere image reference into the target registry", func() {
		target, err := registry.TargetImageRef(
			"ghcr.io/codesphere-cloud/charts/gateway:0.13.3",
			"registry.internal.example.com/mirror",
		)

		Expect(err).NotTo(HaveOccurred())
		Expect(target).To(Equal("registry.internal.example.com/mirror/codesphere-cloud/charts/gateway:0.13.3"))
	})

	It("rejects references outside the Codesphere registry", func() {
		_, err := registry.TargetImageRef("quay.io/prometheus/prometheus:v2.51.0", "registry.internal.example.com")

		Expect(err).To(HaveOccurred())
	})

	It("rejects an empty target registry", func() {
		_, err := registry.TargetImageRef("ghcr.io/codesphere-cloud/charts/gateway:0.13.3", "")

		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("MirrorGHCRImages", func() {
	var config *bom.Config

	BeforeEach(func() {
		config = &bom.Config{
			Components: map[string]bom.ComponentConfig{
				"cluster-pki": {
					ContainerImages: map[string]string{
						"cronjob": "ghcr.io/codesphere-cloud/docker/alpine/kubectl:1.34.2",
					},
				},
			},
		}
	})

	It("copies all refs", func() {
		copier := &fakeCopier{}
		mirror := &registry.Mirror{Copier: copier}

		count, err := mirror.MirrorImages(config.ImageReferencesForCodesphereRegistry(), "registry.internal.example.com")
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(Equal(1))
		Expect(copier.calls).To(Equal([][2]string{{
			"ghcr.io/codesphere-cloud/docker/alpine/kubectl:1.34.2",
			"registry.internal.example.com/codesphere-cloud/docker/alpine/kubectl:1.34.2",
		}}))
	})

	It("does not copy on dry run", func() {
		copier := &fakeCopier{}
		mirror := &registry.Mirror{Copier: copier, DryRun: true}

		count, err := mirror.MirrorImages(config.ImageReferencesForCodesphereRegistry(), "registry.internal.example.com")
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(Equal(1))
		Expect(copier.calls).To(BeEmpty())
	})

	It("propagates copy errors", func() {
		mirror := &registry.Mirror{Copier: &fakeCopier{err: errors.New("copy failed")}}

		_, err := mirror.MirrorImages(config.ImageReferencesForCodesphereRegistry(), "registry.internal.example.com")
		Expect(err).To(MatchError(ContainSubstring("copy failed")))
	})
})
