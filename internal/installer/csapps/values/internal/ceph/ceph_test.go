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
	It("two volumes -> error", func() {
		_, err := ceph.Values(files.CephConfig{Volumes: map[string]files.CephVolume{
			"a": {MDS: files.CephMDS{Active: 1}},
			"b": {MDS: files.CephMDS{Active: 2}},
		}}, false)

		Expect(err).To(MatchError(ContainSubstring("only one ceph volume is supported")))
	})
})
