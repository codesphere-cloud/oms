// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps_test

import (
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// applicationOf is the entry of one application in pc-applications values, empty when there is none.
func applicationOf(values files.ChartValues, name string) files.ChartValues {
	applications, _ := values["applications"].(files.ChartValues)
	app, _ := applications[name].(files.ChartValues)

	return app
}
