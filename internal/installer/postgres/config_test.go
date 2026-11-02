// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"gopkg.in/yaml.v3"
	"testing"

	cnpgv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/postgres"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"
)

func TestPostgres(t *testing.T) { RegisterFailHandler(Fail); RunSpecs(t, "PostgreSQL Suite") }

var _ = Describe("Kubernetes-managed PostgreSQL deployment configuration", func() {
	var pg *postgres.Installer

	BeforeEach(func() {
		manager, err := installer.NewInstallConfigManager("plain", "")
		Expect(err).NotTo(HaveOccurred())
		Expect(manager.ApplyProfile(installer.PROFILE_PROD)).To(Succeed())
		pg = &postgres.Installer{Config: manager.GetInstallConfig().Postgres.K8sManagedConfig}
	})
	It("loads database settings from the implementation-independent config block", func() {
		var cfg files.RootConfig
		Expect(yaml.Unmarshal([]byte(`postgres:
  mode: k8s-managed
  k8sManagedConfig:
    instances: 1
    storageClass: local-rwx
    storageSize: 20Gi
    version: "15.14"
    synchronousReplicas: 0
    podAntiAffinityType: preferred
    requests:
      cpu: "0"
      memory: "0"
`), &cfg)).To(Succeed())
		Expect(cfg.Postgres.K8sManagedConfig).NotTo(BeNil())
		pg := &postgres.Installer{Config: cfg.Postgres.K8sManagedConfig}
		spec, err := pg.ClusterSpec()
		Expect(err).NotTo(HaveOccurred())
		Expect(spec.Instances).To(Equal(1))
		Expect(spec.StorageConfiguration.StorageClass).To(HaveValue(Equal("local-rwx")))
		Expect(spec.StorageConfiguration.Size).To(Equal("20Gi"))

		data, err := yaml.Marshal(cfg.Postgres)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(ContainSubstring("k8sManagedConfig:"))
		Expect(string(data)).NotTo(ContainSubstring("cnpg:"))
	})

	It("adapts the existing database connection for the legacy installer", func() {
		Expect(postgres.ConnectionConfig("cluster-ca")).To(Equal(files.PostgresConfig{
			Mode: "external", ServerAddress: postgres.ServiceAddress, Port: files.PostgresPort,
			Database: postgres.DatabaseName, CACertPem: "cluster-ca",
		}))
	})

	It("uses the node address and allocated NodePort and the service TLS identity for local migrations", func() {
		Expect(postgres.MigrationConfig("10.0.0.25", 32123).Postgres).To(Equal(&files.MigrationPostgresConfig{
			Host: "10.0.0.25", Port: 32123, Database: postgres.DatabaseName, AltName: postgres.ServiceAddress,
		}))
	})

	It("builds the production specification supplied by the profile", func() {
		spec, err := pg.ClusterSpec()
		Expect(err).NotTo(HaveOccurred())
		Expect(spec.Instances).To(Equal(3))
		Expect(spec.PostgresConfiguration.Synchronous.Number).To(Equal(1))
		Expect(spec.PostgresConfiguration.Synchronous.DataDurability).To(BeEquivalentTo("required"))
		Expect(spec.Affinity.PodAntiAffinityType).To(Equal("required"))
		Expect(spec.StorageConfiguration.Size).To(Equal("100Gi"))
		Expect(spec.StorageConfiguration.StorageClass).To(BeNil())
		Expect(spec.Resources.Requests.Cpu().String()).To(Equal("1"))
		Expect(spec.ImagePullSecrets).To(ContainElement(cnpgv1.LocalObjectReference{Name: "oms-cnpg-regcred"}))
	})
	It("supports a single instance with custom storage and resources", func() {
		pg.Config.Instances = ptr.To(1)
		pg.Config.SynchronousReplicas = ptr.To(0)
		pg.Config.StorageClass = "fast"
		pg.Config.StorageSize = "20Gi"
		pg.Config.PodAntiAffinityType = "preferred"
		pg.Config.Requests = map[string]string{"cpu": "0", "memory": "0"}
		spec, err := pg.ClusterSpec()
		Expect(err).NotTo(HaveOccurred())
		Expect(spec.Instances).To(Equal(1))
		Expect(spec.PostgresConfiguration.Synchronous).To(BeNil())
		Expect(spec.StorageConfiguration.StorageClass).To(HaveValue(Equal("fast")))
		Expect(spec.StorageConfiguration.Size).To(Equal("20Gi"))
		Expect(spec.Resources.Requests.Cpu().IsZero()).To(BeTrue())
	})
	It("rejects synchronous replication without enough standbys", func() {
		pg.Config.Instances = ptr.To(1)
		pg.Config.SynchronousReplicas = ptr.To(1)
		_, err := pg.ClusterSpec()
		Expect(err).To(MatchError(ContainSubstring("synchronousReplicas")))
	})
	It("rejects invalid storage", func() {
		pg.Config.StorageSize = "invalid"
		_, err := pg.ClusterSpec()
		Expect(err).To(MatchError(ContainSubstring("storageSize")))
	})
	It("rejects requests larger than limits", func() {
		pg.Config.Requests = map[string]string{"cpu": "2"}
		pg.Config.Limits = map[string]string{"cpu": "1"}
		_, err := pg.ClusterSpec()
		Expect(err).To(MatchError(ContainSubstring("exceeds its limit")))
	})
	It("requires a deployment configuration instead of silently using production settings", func() {
		pg.Config = nil
		_, err := pg.ClusterSpec()
		Expect(err).To(MatchError("postgres.k8sManagedConfig is required"))
	})
	It("requires an explicit instance count", func() {
		pg.Config.Instances = nil
		_, err := pg.ClusterSpec()
		Expect(err).To(MatchError(ContainSubstring("instances is required")))
	})
	It("requires an explicit synchronous replica count", func() {
		pg.Config.SynchronousReplicas = nil
		_, err := pg.ClusterSpec()
		Expect(err).To(MatchError(ContainSubstring("synchronousReplicas is required")))
	})
	It("requires an explicit PostgreSQL version", func() {
		pg.Config.Version = ""
		_, err := pg.ClusterSpec()
		Expect(err).To(MatchError(ContainSubstring("version is required")))
	})
	It("requires explicit resource requests", func() {
		pg.Config.Requests = nil
		_, err := pg.ClusterSpec()
		Expect(err).To(MatchError(ContainSubstring("requests is required")))
	})
	It("requires explicit storage size", func() {
		pg.Config.StorageSize = ""
		_, err := pg.ClusterSpec()
		Expect(err).To(MatchError(ContainSubstring("storageSize")))
	})
	It("requires an explicit anti-affinity policy", func() {
		pg.Config.PodAntiAffinityType = ""
		_, err := pg.ClusterSpec()
		Expect(err).To(MatchError(ContainSubstring("podAntiAffinityType")))
	})
})
