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

func validateTelemetryExport(export *files.TelemetryExport) error {
	if export != nil && export.Traces && strings.TrimSpace(export.TraceEndpoint) == "" {
		return fmt.Errorf(
			"telemetryExport.traceEndpoint must be set if telemetryExport.traces is true",
		)
	}

	return nil
}

func otelValues(export *files.TelemetryExport) map[string]any {
	if export == nil {
		export = &files.TelemetryExport{}
	}

	traces := map[string]any{"enabled": export.Traces}
	if export.TraceEndpoint != "" {
		traces["endpoint"] = export.TraceEndpoint
	}

	return map[string]any{
		"centralOtelExport": map[string]any{
			"enabled":              export.RemoteExport,
			"telemetryCredentials": map[string]any{"secretName": "otel-collector-central-creds"},
			"otlpHttpEndpoint":     cmp.Or(export.RemoteEndpoint, defaultCentralOtelEndpoint),
		},
		"telemetryExport": map[string]any{
			"spanMetrics": map[string]any{"enabled": export.SpanMetrics},
			"traces":      traces,
		},
	}
}
