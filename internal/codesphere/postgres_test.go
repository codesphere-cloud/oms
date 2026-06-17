// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"testing"
)

func TestPostgresServiceDBUsername_Default(t *testing.T) {
	svc := PostgresService{Name: "auth"}
	if got := svc.DBUsername(); got != "auth_blue" {
		t.Errorf("DBUsername() = %q, want %q", got, "auth_blue")
	}
}

func TestPostgresServiceDBUsername_Override(t *testing.T) {
	tests := []struct {
		name     string
		svc      PostgresService
		expected string
	}{
		{
			name:     "usageAggregationRefresher has override",
			svc:      PostgresService{Name: "usageAggregationRefresher", username: "usage_aggregation_refresher"},
			expected: "usage_aggregation_refresher_blue",
		},
		{
			name:     "usageAggregationReader has override",
			svc:      PostgresService{Name: "usageAggregationReader", username: "usage_aggregation_reader"},
			expected: "usage_aggregation_reader_blue",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.svc.DBUsername(); got != tt.expected {
				t.Errorf("DBUsername() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestPostgresServices_Length(t *testing.T) {
	if len(PostgresServices) == 0 {
		t.Fatal("PostgresServices must not be empty")
	}
	// Current expected count is 10
	const expectedLen = 10
	if got := len(PostgresServices); got != expectedLen {
		t.Errorf("len(PostgresServices) = %d, want %d", got, expectedLen)
	}
}

func TestPostgresServices_AllNamesNonEmpty(t *testing.T) {
	for i, svc := range PostgresServices {
		if svc.Name == "" {
			t.Errorf("PostgresServices[%d].Name is empty", i)
		}
	}
}

func TestPostgresServices_UsageAggregationOverrides(t *testing.T) {
	overrides := map[string]string{
		"usageAggregationRefresher": "usage_aggregation_refresher_blue",
		"usageAggregationReader":    "usage_aggregation_reader_blue",
	}
	for _, svc := range PostgresServices {
		if expected, ok := overrides[svc.Name]; ok {
			if got := svc.DBUsername(); got != expected {
				t.Errorf("service %q: DBUsername() = %q, want %q", svc.Name, got, expected)
			}
		}
	}
}
