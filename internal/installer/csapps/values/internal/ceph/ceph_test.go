// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package ceph_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/ceph"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("Values", func() {
	It("one volume -> its name as MDS namespace and its active MDS count", func() {
		values, err := ceph.Values(files.CephConfig{Volumes: map[string]files.CephVolume{
			"shared": {MDS: files.CephMDS{Active: 3}},
		}}, false)
		Expect(err).NotTo(HaveOccurred())

		Expect(values).To(Equal(map[string]any{
			"ceph": map[string]any{
				"credentialsSecretName":     "ceph-admin-credentials",
				"storageClass":              "codesphere",
				"monEndpointsConfigMapName": "ceph-mon-endpoints",
				"mdsNamespace":              "shared",
				"activeMds":                 "3",
			},
		}))
	})

	It("two volumes -> error", func() {
		_, err := ceph.Values(files.CephConfig{Volumes: map[string]files.CephVolume{
			"a": {MDS: files.CephMDS{Active: 1}},
			"b": {MDS: files.CephMDS{Active: 2}},
		}}, false)

		Expect(err).To(MatchError(ContainSubstring("only one ceph volume is supported")))
	})
})
