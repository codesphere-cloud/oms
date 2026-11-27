// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/argocd/pcapps"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("ms-backend-s3 application", func() {
	application := func(config *files.RootConfig, skipSteps ...string) files.ChartValues {
		return applicationOf(pcapps.MsBackendS3PcAppsValues(config, skipSteps), pcapps.MsBackendS3)
	}
	withS3 := func(s3 *files.S3ManagedServiceConfig) *files.RootConfig {
		return &files.RootConfig{ManagedServiceBackends: &files.ManagedServiceBackendsConfig{S3: s3}}
	}
	off := files.ChartValues{"enabled": false}

	Describe("whether the config asks for it", func() {
		It("asks for ms-backend-s3 by default", func() {
			Expect(application(&files.RootConfig{})).ToNot(Equal(off))
		})

		It("takes an explicit enabled: true of the config as asked for", func() {
			Expect(application(withS3(&files.S3ManagedServiceConfig{Enabled: new(true)}))).ToNot(Equal(off))
		})

		It("turns the application off when it is disabled", func() {
			config := withS3(&files.S3ManagedServiceConfig{Enabled: new(false), Override: files.ChartOverride{"replicas": 1}})

			Expect(application(config)).To(Equal(off))
		})

		It("turns the application off with workspace PVC storage in codesphere.experiments and no ceph", func() {
			config := &files.RootConfig{Codesphere: files.CodesphereConfig{Experiments: []string{files.WorkspacePVCStorageFlag}}} //nolint:staticcheck

			Expect(application(config)).To(Equal(off))
		})

		It("turns the application off with workspace PVC storage in codesphere.internal, no ceph and no s3 block", func() {
			config := &files.RootConfig{Codesphere: files.CodesphereConfig{Internal: []string{files.WorkspacePVCStorageFlag}}}

			Expect(application(config)).To(Equal(off))
		})

		It("turns the application off with workspace PVC storage as a preview flag and no ceph", func() {
			config := &files.RootConfig{Codesphere: files.CodesphereConfig{Preview: map[string]bool{files.WorkspacePVCStorageFlag: true}}}

			Expect(application(config)).To(Equal(off))
		})

		It("asks for ms-backend-s3 with workspace PVC storage and ceph", func() {
			config := &files.RootConfig{Ceph: cephWithHosts(), Codesphere: files.CodesphereConfig{Internal: []string{files.WorkspacePVCStorageFlag}}}

			Expect(application(config)).ToNot(Equal(off))
		})

		It("asks for ms-backend-s3 with workspace PVC storage and an explicit s3 block", func() {
			config := withS3(&files.S3ManagedServiceConfig{})
			config.Codesphere = files.CodesphereConfig{Internal: []string{files.WorkspacePVCStorageFlag}}

			Expect(application(config)).ToNot(Equal(off))
		})

		It("turns the application off when the run skips the ms-backends step", func() {
			Expect(application(&files.RootConfig{}, installer.MsBackendsStep)).To(Equal(off))
		})

		It("keeps the application when the run skips the set-up-cluster step", func() {
			Expect(application(&files.RootConfig{}, installer.SetUpClusterStep)).ToNot(Equal(off))
		})
	})

	Describe("the values", func() {
		It("never writes enabled: true", func() {
			Expect(application(withS3(&files.S3ManagedServiceConfig{Enabled: new(true)}))).ToNot(HaveKey("enabled"))
		})

		It("reads the admin credentials from the vault entries a private cloud has", func() {
			Expect(application(&files.RootConfig{})["valuesObject"]).To(Equal(files.ChartValues{
				"extraSecrets": []any{map[string]any{
					"name":  "app-secrets",
					"store": "kubernetes-in-cluster",
					"secretKeys": []any{
						map[string]any{"secretKey": "RGW_ADMIN_ACCESS_KEY_ID", "remoteRef": "cs-vault", "property": "rgwAdminAccessKey.password"},
						map[string]any{"secretKey": "RGW_ADMIN_SECRET_ACCESS_KEY", "remoteRef": "cs-vault", "property": "rgwAdminSecretKey.password"},
					},
				}},
			}))
		})

		It("lets the override add to the secrets", func() {
			config := withS3(&files.S3ManagedServiceConfig{Override: files.ChartOverride{"replicas": 1}})

			Expect(application(config)["valuesObject"]).To(HaveKeyWithValue("replicas", 1))
			Expect(application(config)["valuesObject"]).To(HaveKey("extraSecrets"))
		})
	})
})
