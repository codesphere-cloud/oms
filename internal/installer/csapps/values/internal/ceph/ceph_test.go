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
	oneVolume := files.CephConfig{Volumes: map[string]files.CephVolume{
		"shared": {MDS: files.CephMDS{Active: 3}},
	}}

	It("one volume without PVC storage -> its name as MDS namespace and its active MDS count", func() {
		values, err := ceph.Values(oneVolume /* workspacePVCStorage */, false)
		Expect(err).NotTo(HaveOccurred())

		Expect(values).To(Equal(ceph.Ceph{
			CredentialsSecretName:     "ceph-admin-credentials",
			StorageClass:              "codesphere",
			MonEndpointsConfigMapName: "ceph-mon-endpoints",
			MDSNamespace:              "shared",
			ActiveMDS:                 "3",
		}))
	})

	It("one volume with PVC storage -> no MDS namespace and no active MDS", func() {
		values, err := ceph.Values(oneVolume /* workspacePVCStorage */, true)
		Expect(err).NotTo(HaveOccurred())

		Expect(values).To(Equal(ceph.Ceph{
			CredentialsSecretName:     "ceph-admin-credentials",
			StorageClass:              "codesphere",
			MonEndpointsConfigMapName: "ceph-mon-endpoints",
			MDSNamespace:              "",
			ActiveMDS:                 "0",
		}))
	})

	It("no volumes without PVC storage -> default MDS namespace and active MDS count", func() {
		values, err := ceph.Values(files.CephConfig{} /* workspacePVCStorage */, false)
		Expect(err).NotTo(HaveOccurred())

		Expect(values).To(Equal(ceph.Ceph{
			CredentialsSecretName:     "ceph-admin-credentials",
			StorageClass:              "codesphere",
			MonEndpointsConfigMapName: "ceph-mon-endpoints",
			MDSNamespace:              "codesphere",
			ActiveMDS:                 "7",
		}))
	})

	It("two volumes -> error", func() {
		_, err := ceph.Values(files.CephConfig{Volumes: map[string]files.CephVolume{
			"a": {MDS: files.CephMDS{Active: 1}},
			"b": {MDS: files.CephMDS{Active: 2}},
		}}, /* workspacePVCStorage */ false)

		Expect(err).To(MatchError(ContainSubstring("only one ceph volume is supported")))
	})
})
