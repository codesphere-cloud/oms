// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package postgres builds the values of the postgres key.
package postgres

import (
	"cmp"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

const codesphereDatabase = "codesphere"

// Postgres is the connection to the Codesphere database.
type Postgres struct {
	Host string `yaml:"host"`
	// Port is left to the chart's default when the config has none.
	Port               int    `yaml:"port,omitempty"`
	Database           string `yaml:"database"`
	RejectUnauthorized bool   `yaml:"rejectUnauthorized"`
	SSL                SSL    `yaml:"ssl"`
}

// SSL verifies the server's certificate against its name and, if the config has one, a CA.
type SSL struct {
	ServerName string `yaml:"serverName"`
	CA         string `yaml:"ca,omitempty"`
}

// Values builds the database connection values.
func Values(postgres files.PostgresConfig) Postgres {
	values := Postgres{
		Database:           cmp.Or(postgres.Database, codesphereDatabase),
		RejectUnauthorized: true,
		SSL:                SSL{CA: postgres.CACertPem},
	}

	// TODO(nate): make the postgres mode an enum.
	if postgres.Mode == "install" {
		values.Host = postgres.Primary.IP
		values.Port = files.PostgresPort
		values.SSL.ServerName = postgres.Primary.Hostname

		return values
	}

	values.Host = postgres.ServerAddress
	values.Port = postgres.Port
	values.SSL.ServerName = cmp.Or(postgres.AltName, postgres.ServerAddress)

	return values
}
