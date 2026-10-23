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
	It("install mode -> primary IP as host, hostname as server name", func() {
		values := postgres.Values(files.PostgresConfig{
			Mode:      files.PostgresModeInstall,
			CACertPem: "-----BEGIN CERTIFICATE-----",
			Primary:   &files.PostgresPrimaryConfig{IP: "10.0.0.5", Hostname: "pg-1"},
		})

		Expect(values).To(Equal(postgres.Postgres{
			Host:               "10.0.0.5",
			Port:               5432,
			Database:           "codesphere",
			RejectUnauthorized: true,
			SSL:                postgres.SSL{ServerName: "pg-1", CA: "-----BEGIN CERTIFICATE-----"},
		}))
	})

	It("external postgres -> alt name as server name", func() {
		values := postgres.Values(files.PostgresConfig{
			Mode:          files.PostgresModeExternal,
			ServerAddress: "pg.example.com",
			AltName:       "pg-rw",
			Port:          5433,
			Database:      "masterdata",
		})

		Expect(values).To(Equal(postgres.Postgres{
			Host:               "pg.example.com",
			Port:               5433,
			Database:           "masterdata",
			RejectUnauthorized: true,
			SSL:                postgres.SSL{ServerName: "pg-rw"},
		}))
	})

	It("external mode with a leftover primary -> external server used", func() {
		values := postgres.Values(files.PostgresConfig{
			Mode:          files.PostgresModeExternal,
			ServerAddress: "pg.example.com",
			Port:          5433,
			Primary:       &files.PostgresPrimaryConfig{IP: "10.0.0.5", Hostname: "pg-1"},
		})

		Expect(values.Host).To(Equal("pg.example.com"))
		Expect(values.Port).To(Equal(5433))
		Expect(values.SSL.ServerName).To(Equal("pg.example.com"))
	})
})
