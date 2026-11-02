// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"context"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("PostgreSQL dependency requirements", func() {
	It("rejects skipping Argo CD through command options before accessing deployment dependencies", func() {
		cfg := files.RootConfig{Postgres: files.PostgresConfig{Mode: "k8s-managed"}}
		opts := &InstallCodesphereOpts{SkipSteps: []string{"argocd"}}
		Expect(installCodesphereDepencies(context.Background(), opts, cfg, nil)).To(MatchError(ContainSubstring("k8s-managed requires Argo CD")))
	})
	It("rejects skipping Argo CD through operations before accessing deployment dependencies", func() {
		cfg := files.RootConfig{
			Postgres:   files.PostgresConfig{Mode: "k8s-managed"},
			Operations: &files.OperationsConfig{Skip: []string{"argocd"}},
		}
		Expect(installCodesphereDepencies(context.Background(), &InstallCodesphereOpts{}, cfg, nil)).To(MatchError(ContainSubstring("k8s-managed requires Argo CD")))
	})
	It("accepts Kubernetes-managed PostgreSQL when Argo CD is enabled", func() {
		cfg := files.RootConfig{Postgres: files.PostgresConfig{Mode: "k8s-managed"}}
		Expect(validatePostgresDependencies(cfg, nil)).To(Succeed())
	})
	It("allows an external database without Argo CD", func() {
		cfg := files.RootConfig{Postgres: files.PostgresConfig{Mode: "external"}}
		Expect(validatePostgresDependencies(cfg, []string{"argocd"})).To(Succeed())
	})
})
