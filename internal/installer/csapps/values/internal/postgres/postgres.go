// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package postgres builds the values of the postgres key.
package postgres

import (
	"cmp"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

const (
	codesphereDatabase   = "codesphere"
	internalPostgresPort = 5432
)

// Values builds the database connection values.
func Values(postgres files.PostgresConfig) map[string]any {
	return map[string]any{"postgres": postgresValues(postgres)}
}

func postgresValues(postgres files.PostgresConfig) map[string]any {
	ssl := map[string]any{}
	if postgres.CACertPem != "" {
		ssl["ca"] = postgres.CACertPem
	}

	values := map[string]any{
		"database":           cmp.Or(postgres.Database, codesphereDatabase),
		"rejectUnauthorized": true,
		"ssl":                ssl,
	}

	// Like the old pc-installer, an external server wins when both are configured.
	if postgres.Primary != nil && postgres.ServerAddress == "" {
		values["host"] = postgres.Primary.IP
		values["port"] = internalPostgresPort
		ssl["serverName"] = postgres.Primary.Hostname

		return values
	}

	values["host"] = postgres.ServerAddress
	if postgres.Port != 0 {
		values["port"] = postgres.Port
	}

	ssl["serverName"] = cmp.Or(postgres.AltName, postgres.ServerAddress)

	return values
}
