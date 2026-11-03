// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/codesphere"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("Flags", func() {
	It("flags -> experiments and features", func() {
		conf := files.CodesphereConfig{
			Experiments: []string{"legacy"}, //nolint:staticcheck
			Internal:    []string{"internal-flag"},
			Preview:     map[string]bool{"preview-on": true, "preview-off": false},
			Features:    map[string]bool{"feature-on": true, "feature-off": false},
		}

		values, err := codesphere.Values(conf, defaultTestInputs())
		Expect(err).NotTo(HaveOccurred())

		experiments := values.Experiments
		Expect(experiments).To(HaveKeyWithValue("legacy", []codesphere.FlagEnv{codesphere.FlagEnvProd}))
		Expect(experiments).To(HaveKeyWithValue("internal-flag", []codesphere.FlagEnv{codesphere.FlagEnvProd}))
		Expect(experiments).To(HaveKeyWithValue("preview-on", []codesphere.FlagEnv{codesphere.FlagEnvProd}))
		Expect(experiments).NotTo(HaveKey("preview-off"))

		features := values.Features
		Expect(features).To(HaveKeyWithValue("feature-on", []codesphere.FlagEnv{codesphere.FlagEnvProd}))
		Expect(features).To(HaveKeyWithValue("feature-off", []codesphere.FlagEnv{}))
	})

	It("disabled workspaces Gateway API -> no gateway domains experiment", func() {
		values, err := codesphere.Values(files.CodesphereConfig{}, codesphere.Inputs{Images: defaultTestImages()})
		Expect(err).NotTo(HaveOccurred())

		Expect(values.Experiments).NotTo(HaveKey("gateway-domains"))
	})

	It("internal, experiments, enabled preview and features -> enabled", func() {
		conf := files.CodesphereConfig{
			Internal:    []string{"internal-flag"},
			Experiments: []string{"legacy"}, //nolint:staticcheck
			Preview:     map[string]bool{"preview-on": true},
			Features:    map[string]bool{"feature-on": true},
		}

		Expect(codesphere.EnabledFlags(conf)).To(Equal(map[string]bool{
			"internal-flag": true,
			"legacy":        true,
			"preview-on":    true,
			"feature-on":    true,
		}))
	})

	It("disabled preview and features -> absent", func() {
		conf := files.CodesphereConfig{
			Preview:  map[string]bool{"preview-off": false},
			Features: map[string]bool{"feature-off": false},
		}

		Expect(codesphere.EnabledFlags(conf)).To(BeEmpty())
	})
})
