// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package local

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rookcephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	corev1 "k8s.io/api/core/v1"
)

var _ = Describe("Ceph", func() {
	Describe("cephHostsFromObjectStore", func() {
		It("parses hosts from a valid object store", func() {
			store := &rookcephv1.CephObjectStore{}
			store.Name = "s3-ms-provider"
			store.Status = &rookcephv1.ObjectStoreStatus{
				Endpoints: rookcephv1.ObjectEndpoints{
					Insecure: []string{
						"http://rook-ceph-rgw-s3-ms-provider.rook-ceph.svc:80",
					},
				},
			}

			hosts, err := cephHostsFromObjectStore(store)
			Expect(err).NotTo(HaveOccurred())
			Expect(hosts).To(HaveLen(1))

			expectedHost := "rook-ceph-rgw-s3-ms-provider.rook-ceph.svc"
			Expect(hosts[0].Hostname).To(Equal(expectedHost))
			Expect(hosts[0].IPAddress).To(Equal(expectedHost))
			Expect(hosts[0].IsMaster).To(BeTrue())
		})

		It("rejects invalid endpoint entries", func() {
			store := &rookcephv1.CephObjectStore{}
			store.Name = "s3-ms-provider"
			store.Status = &rookcephv1.ObjectStoreStatus{
				Endpoints: rookcephv1.ObjectEndpoints{
					Insecure: []string{"not-a-url"},
				},
			}

			_, err := cephHostsFromObjectStore(store)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("rgwUserCredentialsFromSecret", func() {
		It("parses credentials from a valid secret", func() {
			secret := &corev1.Secret{}
			secret.Name = "rgw-admin-user"
			secret.Data = map[string][]byte{
				"AccessKey": []byte("access-key"),
				"SecretKey": []byte("secret-key"),
			}

			credentials, err := rgwUserCredentialsFromSecret(secret)
			Expect(err).NotTo(HaveOccurred())
			Expect(credentials.AccessKey).To(Equal("access-key"))
			Expect(credentials.SecretKey).To(Equal("secret-key"))
		})

		It("rejects secrets with missing keys", func() {
			secret := &corev1.Secret{}
			secret.Name = "rgw-admin-user"
			secret.Data = map[string][]byte{
				"username": []byte("foo"),
			}

			_, err := rgwUserCredentialsFromSecret(secret)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("RGW user specs", func() {
		It("creates the admin-ops user without the admin flag or an op mask, and never reconciles it", func() {
			args := rgwUserCreateArgs(rgwAdminUserSpec)
			Expect(args).To(ContainElements("user", "create", "--uid", rgwAdminUserName, "--caps", rgwAdminUserCaps))
			Expect(args).NotTo(ContainElement("--admin"))
			Expect(args).NotTo(ContainElement("--op-mask"))
			Expect(rgwAdminUserSpec.reconcile).To(BeFalse())
		})

		It("creates the backup user with buckets=read, the admin flag and a read-only op mask", func() {
			args := rgwUserCreateArgs(rgwBackupUserSpec)
			Expect(args[:2]).To(Equal([]string{"user", "create"}))
			Expect(args).To(ContainElements("--uid", "sre-s3-backup", "--caps", "buckets=read", "--admin"))
			Expect(args).To(ContainElements("--op-mask", "read"))
			Expect(args).To(ContainElements("--rgw-realm", rgwRealmName, "--rgw-zonegroup", rgwZoneGroupName, "--rgw-zone", rgwZoneName))
			Expect(args[len(args)-2:]).To(Equal([]string{"--format", "json"}))
		})

		It("reconciles an existing backup user: op mask and admin flag in one call, then caps", func() {
			calls := rgwUserReconcileArgs(rgwBackupUserSpec)
			Expect(calls).To(HaveLen(2))
			Expect(calls[0][:7]).To(Equal([]string{"user", "modify", "--uid", "sre-s3-backup", "--admin", "--op-mask", "read"}))
			Expect(calls[1][:6]).To(Equal([]string{"caps", "add", "--uid", "sre-s3-backup", "--caps", "buckets=read"}))
		})

		// The admin flag grants read, write and delete on every bucket. The
		// only thing keeping the backup user read-only is the op mask, so no
		// spec may set one without the other. What RGW does with the two is
		// checked on a cluster (see the PR), not here.
		It("never sets the admin flag without a read-only op mask", func() {
			for _, spec := range []rgwUserSpec{rgwAdminUserSpec, rgwBackupUserSpec} {
				if spec.admin {
					Expect(spec.opMask).To(Equal("read"), "spec %q", spec.uid)
				}
			}
		})
	})

	Describe("rgwUserCredentialsFromAdminJSON", func() {
		It("parses credentials from valid admin JSON", func() {
			credentials, err := rgwUserCredentialsFromAdminJSON(`{"keys":[{"access_key":"access-key","secret_key":"secret-key"}]}`)
			Expect(err).NotTo(HaveOccurred())
			Expect(credentials.AccessKey).To(Equal("access-key"))
			Expect(credentials.SecretKey).To(Equal("secret-key"))
		})

		It("rejects admin JSON with missing keys", func() {
			_, err := rgwUserCredentialsFromAdminJSON(`{"keys":[]}`)
			Expect(err).To(HaveOccurred())
		})
	})
})
