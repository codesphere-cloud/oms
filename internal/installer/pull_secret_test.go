// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	"encoding/json"

	"github.com/codesphere-cloud/oms/internal/installer"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("DockerConfigJSON", func() {
	It("keys the credentials by the registry host", func() {
		config, err := installer.DockerConfigJSON("registry.example.com:5000", "user", "password")
		Expect(err).NotTo(HaveOccurred())

		var decoded map[string]map[string]map[string]string
		Expect(json.Unmarshal(config, &decoded)).To(Succeed())
		Expect(decoded["auths"]).To(Equal(map[string]map[string]string{
			"registry.example.com:5000": {"username": "user", "password": "password", "auth": "dXNlcjpwYXNzd29yZA=="},
		}))
	})
})
