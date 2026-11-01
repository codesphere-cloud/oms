// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package local

import (
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/postgres"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("PostgreSQL migration connection", func() {
	It("uses the forwarded loopback port and restores the previous configuration", func() {
		manager := installer.NewMockInstallConfigManager(GinkgoT())
		manager.EXPECT().WriteInstallConfig("config.yaml", true).Return(nil).Twice()

		previous := &files.MigrationConfig{}
		config := &files.RootConfig{}
		config.Postgres.Database = postgres.DatabaseName
		config.Postgres.ServerAddress = postgres.ServiceAddress
		config.Codesphere.Migration = previous
		bootstrapper := &LocalBootstrapper{
			icg: manager,
			Env: &CodesphereEnvironment{InstallConfig: config, InstallConfigPath: "config.yaml"},
		}

		restore, err := bootstrapper.configurePostgresForMigration("127.0.0.1", 32123)
		Expect(err).NotTo(HaveOccurred())
		Expect(config.Codesphere.Migration.Postgres).To(Equal(&files.MigrationPostgresConfig{
			Host: "127.0.0.1", Port: 32123,
			Database: postgres.DatabaseName, AltName: postgres.ServiceAddress,
		}))
		Expect(restore()).To(Succeed())
		Expect(config.Codesphere.Migration).To(BeIdenticalTo(previous))
	})
})
