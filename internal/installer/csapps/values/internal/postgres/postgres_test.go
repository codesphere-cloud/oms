// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.yaml.in/yaml/v3"

	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/postgres"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("Values", func() {
	It("install mode -> primary IP as host, hostname as server name", func() {
		values, err := postgres.Values(files.PostgresConfig{
			Mode:      "install",
			CACertPem: "-----BEGIN CERTIFICATE-----",
			Primary:   &files.PostgresPrimaryConfig{IP: "10.0.0.5", Hostname: "pg-1"},
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(values).To(Equal(postgres.Postgres{
			Host:               "10.0.0.5",
			Port:               5432,
			Database:           "codesphere",
			RejectUnauthorized: true,
			SSL:                postgres.SSL{ServerName: "pg-1", CA: "-----BEGIN CERTIFICATE-----"},
		}))
	})

	It("install mode without primary -> error", func() {
		_, err := postgres.Values(files.PostgresConfig{Mode: "install"})

		Expect(err).To(MatchError("postgres.primary is required when postgres.mode is install"))
	})

	It("external postgres -> alt name as server name", func() {
		values, err := postgres.Values(files.PostgresConfig{
			Mode:          "external",
			ServerAddress: "pg.example.com",
			AltName:       "pg-rw",
			Port:          5433,
			Database:      "masterdata",
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(values).To(Equal(postgres.Postgres{
			Host:               "pg.example.com",
			Port:               5433,
			Database:           "masterdata",
			RejectUnauthorized: true,
			SSL:                postgres.SSL{ServerName: "pg-rw"},
		}))
	})

	It("external postgres without alt name -> server address as server name", func() {
		values, err := postgres.Values(files.PostgresConfig{
			Mode:          "external",
			ServerAddress: "pg.example.com",
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(values.SSL.ServerName).To(Equal("pg.example.com"))
	})

	It("external postgres without port or CA -> port and ca keys omitted", func() {
		values, err := postgres.Values(files.PostgresConfig{
			Mode:          "external",
			ServerAddress: "pg.example.com",
		})
		Expect(err).NotTo(HaveOccurred())

		out, err := yaml.Marshal(values)
		Expect(err).NotTo(HaveOccurred())

		Expect(out).To(MatchYAML(`
host: pg.example.com
database: codesphere
rejectUnauthorized: true
ssl:
  serverName: pg.example.com
`))
	})

	It("empty mode -> error", func() {
		_, err := postgres.Values(files.PostgresConfig{ServerAddress: "pg.example.com"})

		Expect(err).To(MatchError(`unknown postgres.mode ""`))
	})
})
