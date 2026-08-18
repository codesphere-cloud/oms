// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/bootstrap/datacenter"
	"github.com/codesphere-cloud/oms/internal/bootstrap/gcp"
)

var _ = Describe("BuildDataCenters", func() {
	newEnv := func(multiDC bool) *gcp.CodesphereEnvironment {
		return &gcp.CodesphereEnvironment{
			MultiDC:           multiDC,
			BaseDomain:        "example.com",
			DatacenterName:    "dev",
			SecretsDir:        "/etc/codesphere/secrets",
			InstallConfigPath: "config.yaml",
			SecretsFilePath:   "prod.vault.yaml",
		}
	}

	Context("single data center", func() {
		It("keeps the paths, secrets dir and domains a single-DC bootstrap has always used", func() {
			dcs := gcp.BuildDataCenters(newEnv(false))

			Expect(dcs).To(HaveLen(1))
			dc := dcs[0]
			Expect(dc.IsPrimary()).To(BeTrue())
			Expect(dc.ID).To(Equal(1))
			Expect(dc.Name).To(Equal("dev"))
			Expect(dc.Suffix).To(BeEmpty())
			Expect(dc.InstallConfigPath).To(Equal("config.yaml"))
			Expect(dc.SecretsFilePath).To(Equal("prod.vault.yaml"))
			Expect(dc.RemoteConfigPath).To(Equal("/etc/codesphere/config.yaml"))
			Expect(dc.SecretsDir).To(Equal("/etc/codesphere/secrets"))
			Expect(dc.RemoteVaultPath()).To(Equal("/etc/codesphere/secrets/prod.vault.yaml"))
			Expect(dc.RemoteAgeKeyPath()).To(Equal("/etc/codesphere/secrets/age_key.txt"))
			Expect(dc.K0sConfigScriptPath()).To(Equal("configure-k0s.sh"))
			Expect(dc.WorkspaceHostingBaseDomain).To(Equal("ws.example.com"))
			Expect(dc.SSHBaseDomain).To(Equal("ssh.cs.example.com"))
			Expect(dc.ExternalPostgres).To(BeFalse())
			Expect(dc.StepName("Encrypt vault")).To(Equal("Encrypt vault"))
		})
	})

	Context("multi data center", func() {
		var dcs []*datacenter.DataCenter

		BeforeEach(func() {
			dcs = gcp.BuildDataCenters(newEnv(true))
		})

		It("builds two data centers with the second sharing the first's postgres", func() {
			Expect(dcs).To(HaveLen(2))
			Expect(dcs[0].ExternalPostgres).To(BeFalse())
			Expect(dcs[1].ExternalPostgres).To(BeTrue())
		})

		It("leaves the primary data center's resource names unsuffixed", func() {
			Expect(dcs[0].Suffix).To(BeEmpty())
			Expect(dcs[0].InstallConfigPath).To(Equal("config.yaml"))
			Expect(dcs[0].SecretsDir).To(Equal("/etc/codesphere/secrets"))
		})

		It("gives the secondary data center its own name, paths and secrets dir", func() {
			dc := dcs[1]
			Expect(dc.IsPrimary()).To(BeFalse())
			Expect(dc.ID).To(Equal(2))
			// The k0s cluster is named codesphere-<datacenter name>, so the names must differ.
			Expect(dc.Name).To(Equal("dev-dc2"))
			Expect(dc.InstallConfigPath).To(Equal("config-dc2.yaml"))
			Expect(dc.SecretsFilePath).To(Equal("prod-dc2.vault.yaml"))
			Expect(dc.RemoteConfigPath).To(Equal("/etc/codesphere/config-dc2.yaml"))
			// A separate secrets dir, so the installer cannot overwrite the primary's kubeconfig
			// and ceph credentials through config.secrets.baseDir.
			Expect(dc.SecretsDir).To(Equal("/etc/codesphere/secrets-dc2"))
			Expect(dc.RemoteVaultPath()).To(Equal("/etc/codesphere/secrets-dc2/prod.vault.yaml"))
			Expect(dc.RemoteAgeKeyPath()).To(Equal("/etc/codesphere/secrets-dc2/age_key.txt"))
			Expect(dc.K0sConfigScriptPath()).To(Equal("configure-k0s-dc2.sh"))
			Expect(dc.StepName("Encrypt vault")).To(Equal("Encrypt vault (dc 2)"))
		})

		It("scopes the workspace and ssh domains per data center", func() {
			Expect(dcs[0].WorkspaceHostingBaseDomain).To(Equal("1.ws.example.com"))
			Expect(dcs[0].SSHBaseDomain).To(Equal("1.ssh.cs.example.com"))
			Expect(dcs[1].WorkspaceHostingBaseDomain).To(Equal("2.ws.example.com"))
			Expect(dcs[1].SSHBaseDomain).To(Equal("2.ssh.cs.example.com"))
		})
	})

	It("falls back to the dev datacenter name", func() {
		env := newEnv(false)
		env.DatacenterName = ""

		Expect(gcp.BuildDataCenters(env)[0].Name).To(Equal("dev"))
	})
})
