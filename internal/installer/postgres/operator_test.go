// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/postgres"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("CloudNativePG operator chart values", func() {
	var (
		cfg        files.RootConfig
		packageBOM *bom.Config
	)

	BeforeEach(func() {
		cfg = files.RootConfig{Registry: &files.RegistryConfig{Server: "registry.codesphere.com"}}
		packageBOM = &bom.Config{Components: map[string]bom.ComponentConfig{
			"postgres-operator": {ContainerImages: map[string]string{
				"cloudnativePg": "ghcr.io/codesphere-cloud/docker/ghcr.io/cloudnative-pg/cloudnative-pg:1.30.0",
			}},
		}}
	})

	It("uses the BOM image and version with only the required chart overrides", func() {
		values, err := postgres.OperatorValues(cfg, packageBOM)
		Expect(err).NotTo(HaveOccurred())
		Expect(values["cloudnative-pg"]).To(Equal(map[string]any{
			"image": map[string]any{
				"repository": "registry.codesphere.com/codesphere-cloud/docker/ghcr.io/cloudnative-pg/cloudnative-pg",
				"tag":        "1.30.0",
			},
			"priorityClassName": "",
			"monitoring":        map[string]any{"podMonitorEnabled": false},
			"imagePullSecrets":  []any{map[string]any{"name": "oms-cnpg-regcred"}},
		}))
		Expect(packageBOM.Components["postgres-operator"].ContainerImages["cloudnativePg"]).To(Equal("ghcr.io/codesphere-cloud/docker/ghcr.io/cloudnative-pg/cloudnative-pg:1.30.0"))
	})

	It("always uses the OMS-managed pull secret instead of a custom operator pull-secret list", func() {
		cfg.Cluster.PgOperator = &files.PgOperatorConfig{Override: files.ChartOverride{"cloudnative-pg": map[string]any{
			"imagePullSecrets": []any{map[string]any{"name": "custom-secret"}},
		}}}
		values, err := postgres.OperatorValues(cfg, packageBOM)
		Expect(err).NotTo(HaveOccurred())
		Expect(values["cloudnative-pg"].(map[string]any)["imagePullSecrets"]).To(Equal([]any{map[string]any{"name": "oms-cnpg-regcred"}}))
	})

	It("uses the original BOM image without redundant config overrides when no mirror is configured", func() {
		cfg.Registry = nil
		values, err := postgres.OperatorValues(cfg, packageBOM)
		Expect(err).NotTo(HaveOccurred())
		Expect(values["cloudnative-pg"].(map[string]any)["image"]).To(Equal(map[string]any{
			"repository": "ghcr.io/codesphere-cloud/docker/ghcr.io/cloudnative-pg/cloudnative-pg", "tag": "1.30.0",
		}))
		Expect(values["cloudnative-pg"]).NotTo(HaveKey("config"))
		Expect(values["cloudnative-pg"]).NotTo(HaveKey("resources"))
	})

	It("resolves explicit image overrides and preserves other operator overrides", func() {
		cfg.Registry.Server = "registry.example.com:5000/team//mirror/"
		cfg.Cluster.PgOperator = &files.PgOperatorConfig{Override: files.ChartOverride{"cloudnative-pg": map[string]any{
			"image":             map[string]any{"repository": "ghcr.io/codesphere-cloud/custom-cnpg", "tag": "1.31.0"},
			"priorityClassName": "database",
			"config":            map[string]any{"data": map[string]any{"INHERITED_LABELS": "teamId,environment"}},
		}}}
		values, err := postgres.OperatorValues(cfg, packageBOM)
		Expect(err).NotTo(HaveOccurred())

		upstream := values["cloudnative-pg"].(map[string]any)
		Expect(upstream["image"]).To(Equal(map[string]any{"repository": "registry.example.com:5000/team/mirror/codesphere-cloud/custom-cnpg", "tag": "1.31.0"}))
		Expect(upstream["priorityClassName"]).To(Equal("database"))
		Expect(upstream["config"]).To(Equal(map[string]any{"data": map[string]any{"INHERITED_LABELS": "teamId,environment"}}))
	})

	It("preserves an explicitly configured image outside the admission policy's source prefix", func() {
		cfg.Cluster.PgOperator = &files.PgOperatorConfig{Override: files.ChartOverride{"cloudnative-pg": map[string]any{
			"image": map[string]any{"repository": "custom.example.com/cnpg"},
		}}}
		values, err := postgres.OperatorValues(cfg, packageBOM)
		Expect(err).NotTo(HaveOccurred())
		Expect(values["cloudnative-pg"].(map[string]any)["image"].(map[string]any)["repository"]).To(Equal("custom.example.com/cnpg"))
	})

	It("does not rewrite an already mirrored image when registry prefixes overlap", func() {
		cfg.Registry.Server = "ghcr.io/codesphere-cloud/mirror"
		repository := "ghcr.io/codesphere-cloud/mirror/codesphere-cloud/cnpg"
		packageBOM.Components["postgres-operator"].ContainerImages["cloudnativePg"] = repository + ":1.30.0"
		values, err := postgres.OperatorValues(cfg, packageBOM)
		Expect(err).NotTo(HaveOccurred())
		Expect(values["cloudnative-pg"].(map[string]any)["image"].(map[string]any)["repository"]).To(Equal(repository))
	})

	It("reports a missing operator image in the BOM", func() {
		_, err := postgres.OperatorValues(cfg, &bom.Config{})
		Expect(err).To(MatchError(ContainSubstring("containerImages.cloudnativePg")))
	})

	It("reports invalid operator image references in the BOM", func() {
		packageBOM.Components["postgres-operator"].ContainerImages["cloudnativePg"] = "not an image"
		_, err := postgres.OperatorValues(cfg, packageBOM)
		Expect(err).To(MatchError(ContainSubstring("parse CloudNativePG image from BOM")))
	})

	It("requires a BOM image tag instead of silently using the chart's version", func() {
		packageBOM.Components["postgres-operator"].ContainerImages["cloudnativePg"] = "ghcr.io/codesphere-cloud/cnpg"
		_, err := postgres.OperatorValues(cfg, packageBOM)
		Expect(err).To(MatchError(ContainSubstring("requires a tag")))
	})

	It("retains a digest pinned alongside the BOM image tag", func() {
		digest := "sha256:a2701eb97cdd2a34b1fdb2cb51987f544b706e40bec72ae7146cd8580efefebb"
		packageBOM.Components["postgres-operator"].ContainerImages["cloudnativePg"] += "@" + digest
		values, err := postgres.OperatorValues(cfg, packageBOM)
		Expect(err).NotTo(HaveOccurred())
		Expect(values["cloudnative-pg"].(map[string]any)["image"].(map[string]any)["tag"]).To(Equal("1.30.0@" + digest))
	})
})
