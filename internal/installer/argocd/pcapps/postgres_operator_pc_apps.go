// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps

import (
	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/util"
)

const (
	postgresOperatorBomComponent = "postgres-operator"
	cloudnativePgImageKey        = "cloudnativePg"
)

// postgresOperatorInstalled reports whether the config deploys the CloudNativePG operator. An
// absent cluster.pgOperator is enabled. Mirrors setUpCluster in codesphere-monorepo
// packages/script-lib/src/ts/private-cloud/cluster.ts.
func postgresOperatorInstalled(config *files.RootConfig) bool {
	pg := config.Cluster.PgOperator

	return pg == nil || pg.Enabled
}

// postgresOperatorValues points the operator at the image of the package BOM and the registry
// pull secret, then applies cluster.pgOperator.override on top.
func postgresOperatorValues(config *files.RootConfig, packageBom *bom.Config) (files.ChartValues, error) {
	chart := files.ChartValues{"imagePullSecrets": imagePullSecrets(config)}

	image, found, err := bomImageOf(packageBom, config, postgresOperatorBomComponent, cloudnativePgImageKey)
	if err != nil {
		return nil, err
	}

	if found {
		chart["image"] = files.ChartValues{"repository": image.Name, "tag": image.Tag}
	}

	values := files.ChartValues{"cloudnative-pg": chart}
	if pg := config.Cluster.PgOperator; pg != nil {
		values = util.DeepMergeMaps(values, pg.Override)
	}

	return values, nil
}
