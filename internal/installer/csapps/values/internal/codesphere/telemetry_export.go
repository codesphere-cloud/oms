// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"cmp"
	"fmt"
	"strings"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

const defaultCentralOtelEndpoint = "https://otel.tools.codesphere.com"

// Otel is where the services' telemetry goes.
type Otel struct {
	CentralOtelExport CentralOtelExport `yaml:"centralOtelExport"`
	TelemetryExport   TelemetryExport   `yaml:"telemetryExport"`
}

// CentralOtelExport is the export to Codesphere's central collector.
type CentralOtelExport struct {
	Enabled              bool                 `yaml:"enabled"`
	TelemetryCredentials TelemetryCredentials `yaml:"telemetryCredentials"`
	OTLPHTTPEndpoint     string               `yaml:"otlpHttpEndpoint"`
}

// TelemetryCredentials names the secret with the central collector's credentials.
type TelemetryCredentials struct {
	SecretName string `yaml:"secretName"`
}

// TelemetryExport is the export to the customer's own collector.
type TelemetryExport struct {
	SpanMetrics SpanMetrics `yaml:"spanMetrics"`
	Traces      Traces      `yaml:"traces"`
}

// SpanMetrics says whether metrics are derived from spans.
type SpanMetrics struct {
	Enabled bool `yaml:"enabled"`
}

// Traces is the trace export.
type Traces struct {
	Enabled  bool   `yaml:"enabled"`
	Endpoint string `yaml:"endpoint,omitempty"`
}

func validateTelemetryExport(export *files.TelemetryExport) error {
	if export != nil && export.Traces && strings.TrimSpace(export.TraceEndpoint) == "" {
		return fmt.Errorf(
			"telemetryExport.traceEndpoint must be set if telemetryExport.traces is true",
		)
	}

	return nil
}

func otelValues(export *files.TelemetryExport) Otel {
	if export == nil {
		export = &files.TelemetryExport{}
	}

	return Otel{
		CentralOtelExport: CentralOtelExport{
			Enabled:              export.RemoteExport,
			TelemetryCredentials: TelemetryCredentials{SecretName: "otel-collector-central-creds"},
			OTLPHTTPEndpoint:     cmp.Or(export.RemoteEndpoint, defaultCentralOtelEndpoint),
		},
		TelemetryExport: TelemetryExport{
			SpanMetrics: SpanMetrics{Enabled: export.SpanMetrics},
			Traces:      Traces{Enabled: export.Traces, Endpoint: export.TraceEndpoint},
		},
	}
}
