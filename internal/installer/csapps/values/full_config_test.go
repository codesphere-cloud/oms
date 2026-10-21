// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package values_test

import (
	"encoding/json"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/bom"
	csappsvalues "github.com/codesphere-cloud/oms/internal/installer/csapps/values"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// full_config_values.json is what the Codesphere installer builds from full_config.yaml and
// full_config_bom.json, in the External Secrets mode of codesphere-cloud/codesphere-monorepo#22339
// (a0e2525783): the values files and the imageTag/hostName layer it hands Helm, merged by Helm. The
// secret layers are left out, as External Secrets provides them.
var _ = Describe("Build with every field set", func() {
	It("full config -> the values the Codesphere installer builds", func() {
		data, err := os.ReadFile(filepath.Join("testdata", "full_config.yaml"))
		Expect(err).NotTo(HaveOccurred())

		cfg := files.NewRootConfig()
		Expect(cfg.Unmarshal(data)).To(Succeed())

		b, err := bom.Parse(filepath.Join("testdata", "full_config_bom.json"))
		Expect(err).NotTo(HaveOccurred())

		values, err := csappsvalues.Build(&cfg, b)
		Expect(err).NotTo(HaveOccurred())

		expected, err := os.ReadFile(filepath.Join("testdata", "full_config_values.json"))
		Expect(err).NotTo(HaveOccurred())

		actual, err := json.Marshal(values)
		Expect(err).NotTo(HaveOccurred())
		Expect(actual).To(MatchJSON(expected))
	})
})
