// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import "github.com/codesphere-cloud/oms/internal/installer/docker"

func (b *GCPBootstrapper) InstallPostgres() error {
	dockerInstaller := docker.New("root", b.Env.PostgreSQLNode)
	if !dockerInstaller.IsInstalled() {
		dockerInstaller.Install()
	}

	// b.Env.InstallSkipSteps = append(b.Env.InstallSkipSteps, "docker")

	return nil
}
