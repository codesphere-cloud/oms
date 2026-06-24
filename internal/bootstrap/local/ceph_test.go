// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rookcephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	ctrlclientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
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

	Describe("rgwExecPrerequisites", func() {
		It("caches the Ceph monitor hosts and admin auth for repeated RGW execs", func() {
			scheme := runtime.NewScheme()
			Expect(corev1.AddToScheme(scheme)).To(Succeed())

			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cephMonEndpointsConfigMap,
					Namespace: rookNamespace,
				},
				Data: map[string]string{
					"data": "a=10.0.0.1:6789,b=10.0.0.2:6789",
				},
			}
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cephMonSecretName,
					Namespace: rookNamespace,
				},
				Data: map[string][]byte{
					"ceph-username": []byte("client.admin"),
					"ceph-secret":   []byte("secret-1"),
				},
			}

			bootstrapper := &LocalBootstrapper{
				ctx:        context.Background(),
				kubeClient: ctrlclientfake.NewClientBuilder().WithScheme(scheme).WithObjects(configMap, secret).Build(),
			}

			prereqs, err := bootstrapper.rgwExecPrerequisites()
			Expect(err).NotTo(HaveOccurred())
			Expect(prereqs.monHosts).To(Equal("10.0.0.1:6789,10.0.0.2:6789"))
			Expect(prereqs.adminUser).To(Equal("client.admin"))
			Expect(prereqs.adminSecret).To(Equal("secret-1"))

			configMap.Data["data"] = "c=10.0.0.3:6789"
			Expect(bootstrapper.kubeClient.Update(context.Background(), configMap)).To(Succeed())

			secret.Data["ceph-username"] = []byte("client.changed")
			secret.Data["ceph-secret"] = []byte("secret-2")
			Expect(bootstrapper.kubeClient.Update(context.Background(), secret)).To(Succeed())

			cachedPrereqs, err := bootstrapper.rgwExecPrerequisites()
			Expect(err).NotTo(HaveOccurred())
			Expect(cachedPrereqs).To(BeIdenticalTo(prereqs))
			Expect(cachedPrereqs.monHosts).To(Equal("10.0.0.1:6789,10.0.0.2:6789"))
			Expect(cachedPrereqs.adminUser).To(Equal("client.admin"))
			Expect(cachedPrereqs.adminSecret).To(Equal("secret-1"))
		})
	})

	Describe("podExecClientset", func() {
		It("creates the pod exec clientset only once per bootstrap run", func() {
			originalFactory := newKubernetesClientset
			defer func() {
				newKubernetesClientset = originalFactory
			}()

			calls := 0
			clientset := kubefake.NewSimpleClientset()
			newKubernetesClientset = func(_ *rest.Config) (kubernetesClientset, error) {
				calls++
				return clientset, nil
			}

			bootstrapper := &LocalBootstrapper{
				restConfig: &rest.Config{Host: "https://example.invalid"},
			}

			first, err := bootstrapper.podExecClientset()
			Expect(err).NotTo(HaveOccurred())
			second, err := bootstrapper.podExecClientset()
			Expect(err).NotTo(HaveOccurred())

			Expect(first).To(BeIdenticalTo(clientset))
			Expect(second).To(BeIdenticalTo(clientset))
			Expect(calls).To(Equal(1))
		})
	})
})
