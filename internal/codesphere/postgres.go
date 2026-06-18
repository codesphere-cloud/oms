// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

const blueUserSuffix = "_blue"

// PostgresService describes a Codesphere service that owns a dedicated postgres user.
type PostgresService struct {
	// Name is the service key used to derive vault secret names (e.g. "auth" → "postgresUserAuth").
	Name string
	// username is the actual postgres username. When empty, defaults to Name + "_blue".
	username string
}

// DBUsername returns the postgres username for the service.
func (s PostgresService) DBUsername() string {
	if s.username != "" {
		return s.username + blueUserSuffix
	}
	return s.Name + blueUserSuffix
}

// PostgresServices is the canonical list of Codesphere services that each get a dedicated
// postgres user and password secret.
var PostgresServices = []PostgresService{
	{Name: "auth"},
	{Name: "deployment"},
	{Name: "ide"},
	{Name: "marketplace"},
	{Name: "payment"},
	{Name: "public_api"},
	{Name: "team"},
	{Name: "workspace"},
	{Name: "usageAggregationRefresher", username: "usage_aggregation_refresher"},
	{Name: "usageAggregationReader", username: "usage_aggregation_reader"},
}
