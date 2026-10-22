// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/postgres"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("Values", func() {
	It("external postgres -> alt name as server name", func() {
		values := postgres.Values(files.PostgresConfig{
			ServerAddress: "pg.example.com",
			AltName:       "pg-rw",
			Port:          5433,
			Database:      "masterdata",
		})

		Expect(values).To(Equal(map[string]any{"postgres": map[string]any{
			"host":               "pg.example.com",
			"port":               5433,
			"database":           "masterdata",
			"rejectUnauthorized": true,
			"ssl":                map[string]any{"serverName": "pg-rw"},
		}}))
	})

	It("external server and primary both set -> external server used", func() {
		values := postgres.Values(files.PostgresConfig{
			ServerAddress: "pg.example.com",
			Port:          5433,
			Primary:       &files.PostgresPrimaryConfig{IP: "10.0.0.5", Hostname: "pg-1"},
		})

		Expect(values).To(HaveKeyWithValue("postgres", And(
			HaveKeyWithValue("host", "pg.example.com"),
			HaveKeyWithValue("port", 5433),
		)))
	})
})
