// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps_test

import (
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// cephWithHosts is a ceph block with one host per address, or with 10.0.0.11 when none is given.
func cephWithHosts(ips ...string) files.CephConfig {
	if len(ips) == 0 {
		ips = []string{"10.0.0.11"}
	}

	hosts := make([]files.CephHost, 0, len(ips))
	for _, ip := range ips {
		hosts = append(hosts, files.CephHost{IPAddress: ip})
	}

	return files.CephConfig{Hosts: hosts}
}

// applicationOf is the entry of one application in pc-applications values, empty when there is none.
func applicationOf(values files.ChartValues, name string) files.ChartValues {
	applications, _ := values["applications"].(files.ChartValues)
	app, _ := applications[name].(files.ChartValues)

	return app
}
