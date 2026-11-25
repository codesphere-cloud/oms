// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package csapps_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/installer/csapps"
)

var _ = Describe("ReadBOM", func() {
	// packageWith returns a package whose extracted BOM is b.
	packageWith := func(b *bom.Config) *installer.MockPackageManager {
		data, err := json.Marshal(b)
		Expect(err).NotTo(HaveOccurred())

		path := filepath.Join(GinkgoT().TempDir(), bom.Filename)
		Expect(os.WriteFile(path, data, 0o600)).To(Succeed())

		pm := installer.NewMockPackageManager(GinkgoT())
		pm.EXPECT().ExtractDependency(bom.Filename, false, false).Return(nil)
		pm.EXPECT().GetDependencyPath(bom.Filename).Return(path)

		return pm
	}

	withoutChart := func(component string) *bom.Config {
		b := bomWithCharts()
		c := b.Components[component]
		c.Files = nil
		b.Components[component] = c

		return b
	}

	It("BOM with both charts -> that BOM", func() {
		csBom, err := csapps.ReadBOM(packageWith(bomWithCharts()), false, false)

		Expect(err).NotTo(HaveOccurred())
		Expect(csBom).To(Equal(bomWithCharts()))
	})

	It("BOM without the cs-applications chart -> error naming it", func() {
		_, err := csapps.ReadBOM(packageWith(withoutChart("cs-applications")), false, false)

		Expect(err).To(MatchError("cs-applications chart not found in the package BOM"))
	})

	It("BOM without the codesphere chart -> error naming it", func() {
		_, err := csapps.ReadBOM(packageWith(withoutChart("codesphere")), false, false)

		Expect(err).To(MatchError("codesphere chart not found in the package BOM"))
	})

	It("extraction fails -> that error", func() {
		pm := installer.NewMockPackageManager(GinkgoT())
		pm.EXPECT().ExtractDependency(bom.Filename, false, false).Return(errors.New("disk full"))

		_, err := csapps.ReadBOM(pm, false, false)

		Expect(err).To(MatchError(ContainSubstring("disk full")))
	})
})
