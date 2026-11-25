// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps

import (
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/util"
)

// PostgresOperator is the key of the postgres-operator chart under `applications` in the pc-applications values.
const PostgresOperator = "postgres-operator"

// PostgresOperatorPcAppsValues derives the pc-applications values for the CloudNativePG operator:
// `enabled: false` when cluster.pgOperator turns it off, otherwise the registry pull secret with
// the override of cluster.pgOperator on top. The images are the chart defaults.
func PostgresOperatorPcAppsValues(config *files.RootConfig) files.ChartValues {
	if !postgresOperatorInstalled(config) {
		return applicationValues(PostgresOperator, false, nil)
	}

	values := files.ChartValues{"cloudnative-pg": files.ChartValues{"imagePullSecrets": imagePullSecrets(config)}}
	if pg := config.Cluster.PgOperator; pg != nil {
		values = util.DeepMergeMaps(values, pg.Override)
	}

	return applicationValues(PostgresOperator, true, values)
}

// postgresOperatorInstalled reports whether the config deploys the CloudNativePG operator. An
// absent cluster.pgOperator is enabled.
func postgresOperatorInstalled(config *files.RootConfig) bool {
	pg := config.Cluster.PgOperator

	return pg == nil || pg.Enabled
}
