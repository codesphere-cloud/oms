// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"context"
	"errors"

	cnpgv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/postgres"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Masterdata database readiness", func() {
	var (
		kube    *MockClient
		pg      *postgres.Installer
		cluster *cnpgv1.Cluster
		pods    []corev1.Pod
		service *corev1.Service
	)

	BeforeEach(func() {
		kube = NewMockClient(GinkgoT())
		manager, err := installer.NewInstallConfigManager("plain", "")
		Expect(err).NotTo(HaveOccurred())
		Expect(manager.ApplyProfile(installer.PROFILE_DEV)).To(Succeed())
		pg = &postgres.Installer{Context: context.Background(), KubeClient: kube, Config: manager.GetInstallConfig().Postgres.K8sManagedConfig}
		spec, err := pg.ClusterSpec()
		Expect(err).NotTo(HaveOccurred())

		cluster = &cnpgv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{Name: "masterdata", Namespace: "codesphere", UID: "current-cluster", Generation: 3},
			Spec:       spec,
			Status: cnpgv1.ClusterStatus{Instances: 1, ReadyInstances: 1, Phase: cnpgv1.PhaseHealthy, CurrentPrimary: "masterdata-1", TargetPrimary: "masterdata-1",
				Conditions: []metav1.Condition{{Type: string(cnpgv1.ConditionClusterReady), Status: metav1.ConditionTrue, ObservedGeneration: 3}},
			},
		}
		pods = []corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{Name: "masterdata-1", Namespace: "codesphere",
				Labels:          map[string]string{"cnpg.io/cluster": "masterdata", "cnpg.io/podRole": "instance", "cnpg.io/instanceRole": "primary"},
				OwnerReferences: []metav1.OwnerReference{{APIVersion: cnpgv1.SchemeGroupVersion.String(), Kind: "Cluster", Name: cluster.Name, UID: cluster.UID, Controller: ptr.To(true)}},
			},
			Spec:   corev1.PodSpec{ImagePullSecrets: []corev1.LocalObjectReference{{Name: "oms-cnpg-regcred"}}, Containers: []corev1.Container{{Name: "postgres", Image: spec.ImageName, Resources: spec.Resources}}},
			Status: corev1.PodStatus{Phase: corev1.PodRunning, Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}},
		}}
		service = &corev1.Service{Spec: corev1.ServiceSpec{Selector: map[string]string{"cnpg.io/cluster": "masterdata", "cnpg.io/instanceRole": "primary"}}}
	})

	expectCluster := func() {
		GinkgoHelper()
		kube.EXPECT().Get(mock.Anything, client.ObjectKey{Namespace: "codesphere", Name: "masterdata"}, mock.AnythingOfType("*v1.Cluster")).RunAndReturn(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
			*obj.(*cnpgv1.Cluster) = *cluster.DeepCopy()
			return nil
		}).Once()
	}
	expectPods := func() {
		GinkgoHelper()
		kube.EXPECT().List(mock.Anything, mock.AnythingOfType("*v1.PodList"), mock.AnythingOfType("[]client.ListOption")).RunAndReturn(func(_ context.Context, obj client.ObjectList, opts ...client.ListOption) error {
			options := &client.ListOptions{}
			options.ApplyOptions(opts)
			Expect(options.Namespace).To(Equal("codesphere"))
			Expect(options.LabelSelector.String()).To(Equal("cnpg.io/cluster=masterdata,cnpg.io/podRole=instance"))

			obj.(*corev1.PodList).Items = pods

			return nil
		}).Once()
	}
	expectService := func() {
		GinkgoHelper()
		kube.EXPECT().Get(mock.Anything, client.ObjectKey{Namespace: "codesphere", Name: "masterdata-rw"}, mock.AnythingOfType("*v1.Service")).RunAndReturn(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
			*obj.(*corev1.Service) = *service.DeepCopy()
			return nil
		}).Once()
	}

	It("accepts a reconciled cluster with a ready primary behind its read-write service", func() {
		expectCluster()
		expectPods()
		expectService()

		primary, err := pg.CheckDatabaseReady(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(primary).To(Equal("masterdata-1"))
	})
	It("does not trust an old Ready condition after the cluster spec changes", func() {
		cluster.Status.Conditions[0].ObservedGeneration = 2

		expectCluster()

		_, err := pg.CheckDatabaseReady(context.Background())
		Expect(err).To(MatchError(ContainSubstring("observes generation 2, want 3")))
	})
	It("requires a true Ready condition even when stale instance counts look healthy", func() {
		cluster.Status.Conditions[0].Status = metav1.ConditionFalse

		expectCluster()

		_, err := pg.CheckDatabaseReady(context.Background())
		Expect(err).To(MatchError(ContainSubstring("Ready condition is not true")))
	})
	It("waits for status to reflect a newly requested instance count", func() {
		cluster.Spec.Instances = 2

		expectCluster()

		_, err := pg.CheckDatabaseReady(context.Background())
		Expect(err).To(MatchError(ContainSubstring("desired=2")))
	})
	It("rejects healthy cluster status when no actual instance pods exist", func() {
		pods = nil

		expectCluster()
		expectPods()

		_, err := pg.CheckDatabaseReady(context.Background())
		Expect(err).To(MatchError(ContainSubstring("found 0 instance pods, want 1")))
	})
	It("requires every desired instance to be ready even when the primary is ready", func() {
		cluster.Spec.Instances = 2
		cluster.Status.Instances = 2
		cluster.Status.ReadyInstances = 2
		replica := pods[0].DeepCopy()
		replica.Name = "masterdata-2"
		replica.Labels["cnpg.io/instanceRole"] = "replica"
		replica.Status.Conditions[0].Status = corev1.ConditionFalse
		pods = append(pods, *replica)

		expectCluster()
		expectPods()

		_, err := pg.CheckDatabaseReady(context.Background())
		Expect(err).To(MatchError(ContainSubstring("pod \"masterdata-2\" is not ready")))
	})

	It("reports image pull failures instead of trusting healthy cluster status", func() {
		pods[0].Status.Phase = corev1.PodPending
		pods[0].Status.Conditions[0].Status = corev1.ConditionFalse
		pods[0].Status.ContainerStatuses = []corev1.ContainerStatus{{Name: "postgres", State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"}}}}

		expectCluster()
		expectPods()

		_, err := pg.CheckDatabaseReady(context.Background())
		Expect(err).To(MatchError(ContainSubstring("ImagePullBackOff")))
	})
	It("rejects a running pod whose readiness probe is failing", func() {
		pods[0].Status.Conditions[0].Status = corev1.ConditionFalse

		expectCluster()
		expectPods()

		_, err := pg.CheckDatabaseReady(context.Background())
		Expect(err).To(MatchError(ContainSubstring("pod \"masterdata-1\" is not ready")))
	})
	It("waits for terminating pods to be replaced", func() {
		pods[0].DeletionTimestamp = &metav1.Time{}

		expectCluster()
		expectPods()

		_, err := pg.CheckDatabaseReady(context.Background())
		Expect(err).To(MatchError(ContainSubstring("is not ready")))
	})
	It("checks live pods when the operator does not report observedGeneration", func() {
		cluster.Status.Conditions[0].ObservedGeneration = 0
		pods[0].Spec.Containers[0].Image = "old-postgres-image"

		expectCluster()
		expectPods()

		_, err := pg.CheckDatabaseReady(context.Background())
		Expect(err).To(MatchError(ContainSubstring("uses image \"old-postgres-image\"")))
	})
	It("waits for pods to receive the cluster image pull secret", func() {
		pods[0].Spec.ImagePullSecrets = nil

		expectCluster()
		expectPods()

		_, err := pg.CheckDatabaseReady(context.Background())
		Expect(err).To(MatchError(ContainSubstring("missing image pull secret")))
	})
	It("waits for resource changes to reach the pods", func() {
		pods[0].Spec.Containers[0].Resources.Requests = corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1")}

		expectCluster()
		expectPods()

		_, err := pg.CheckDatabaseReady(context.Background())
		Expect(err).To(MatchError(ContainSubstring("resources do not match")))
	})
	It("does not accept pods left over from a replaced cluster", func() {
		pods[0].OwnerReferences[0].UID = "previous-cluster"

		expectCluster()
		expectPods()

		_, err := pg.CheckDatabaseReady(context.Background())
		Expect(err).To(MatchError(ContainSubstring("does not belong to the current cluster")))
	})
	It("waits until the primary service selects the ready primary pod", func() {
		service.Spec.Selector["cnpg.io/instanceRole"] = "replica"

		expectCluster()
		expectPods()
		expectService()

		_, err := pg.CheckDatabaseReady(context.Background())
		Expect(err).To(MatchError(ContainSubstring("no ready primary pod backs service codesphere/masterdata-rw")))

		service.Spec.Selector["cnpg.io/instanceRole"] = "primary"

		expectCluster()
		expectPods()
		expectService()

		primary, err := pg.CheckDatabaseReady(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(primary).To(Equal("masterdata-1"))
	})
	It("finishes the wait immediately when the cluster and primary pod are ready", func() {
		expectCluster()
		expectPods()
		expectService()
		Expect(pg.WaitForDatabaseReady()).To(Succeed())
	})
	It("waits before exposing migrations and respects cancellation while pods are missing", func() {
		kube.EXPECT().List(mock.Anything, mock.AnythingOfType("*v1.ServiceList"), []client.ListOption{client.InNamespace("codesphere")}).Return(nil).Once()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		expectCluster()
		kube.EXPECT().List(mock.Anything, mock.AnythingOfType("*v1.PodList"), mock.AnythingOfType("[]client.ListOption")).RunAndReturn(func(_ context.Context, obj client.ObjectList, _ ...client.ListOption) error {
			obj.(*corev1.PodList).Items = nil

			cancel()

			return nil
		}).Once()

		endpoint, err := pg.CreateMigrationEndpoint(ctx)
		Expect(endpoint).To(BeNil())
		Expect(errors.Is(err, context.Canceled)).To(BeTrue())
	})
	It("returns Kubernetes access failures without retrying them", func() {
		kube.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(errors.New("access denied")).Once()
		Expect(pg.WaitForDatabaseReady()).To(MatchError(ContainSubstring("access denied")))
	})

	Context("temporary migration NodePort", func() {
		var (
			created       *corev1.Service
			staleServices []corev1.Service
			listError     error
		)

		BeforeEach(func() {
			staleServices = nil
			listError = nil

			kube.EXPECT().List(mock.Anything, mock.AnythingOfType("*v1.ServiceList"), []client.ListOption{client.InNamespace("codesphere")}).RunAndReturn(func(_ context.Context, obj client.ObjectList, _ ...client.ListOption) error {
				obj.(*corev1.ServiceList).Items = staleServices
				return listError
			}).Once()
		})

		prepareDatabase := func() {
			GinkgoHelper()

			service.Spec.Ports = []corev1.ServicePort{{Port: 5432, TargetPort: intstr.FromString("postgresql")}}

			expectCluster()
			expectPods()
			expectService()
			expectService()
		}

		expectNodes := func(addressType corev1.NodeAddressType, address string) {
			GinkgoHelper()
			kube.EXPECT().List(mock.Anything, mock.AnythingOfType("*v1.NodeList")).RunAndReturn(func(_ context.Context, obj client.ObjectList, _ ...client.ListOption) error {
				obj.(*corev1.NodeList).Items = []corev1.Node{{Status: corev1.NodeStatus{Addresses: []corev1.NodeAddress{{Type: addressType, Address: address}}}}}
				return nil
			}).Once()
		}
		expectCreate := func() *mock.Call {
			GinkgoHelper()

			return kube.EXPECT().Create(mock.Anything, mock.AnythingOfType("*v1.Service")).RunAndReturn(func(_ context.Context, obj client.Object, _ ...client.CreateOption) error {
				created = obj.(*corev1.Service)
				Expect(created.GenerateName).To(Equal("oms-masterdata-nodeport-"))
				Expect(created.Namespace).To(Equal("codesphere"))
				Expect(created.Spec.Type).To(Equal(corev1.ServiceTypeNodePort))
				Expect(created.Spec.Selector).To(Equal(service.Spec.Selector))
				Expect(created.Spec.Ports).To(Equal([]corev1.ServicePort{{Name: "postgres", Protocol: corev1.ProtocolTCP, Port: 5432, TargetPort: service.Spec.Ports[0].TargetPort}}))
				created.Name = "oms-masterdata-nodeport-test"
				created.Spec.Ports[0].NodePort = 32123

				return nil
			}).Once()
		}

		It("deletes leftover migration Services before creating a replacement and preserves other Services", func() {
			staleServices = []corev1.Service{
				{ObjectMeta: metav1.ObjectMeta{Name: "oms-masterdata-nodeport-old-1", Namespace: "codesphere"}, Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeNodePort, Selector: map[string]string{"cnpg.io/cluster": "masterdata"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "oms-masterdata-nodeport-old-2", Namespace: "codesphere"}, Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeNodePort, Selector: map[string]string{"cnpg.io/cluster": "masterdata"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "user-postgres", Namespace: "codesphere"}, Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeNodePort, Selector: map[string]string{"cnpg.io/cluster": "masterdata"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "oms-masterdata-nodeport-other-db", Namespace: "codesphere"}, Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeNodePort, Selector: map[string]string{"cnpg.io/cluster": "other"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "oms-masterdata-nodeport-clusterip", Namespace: "codesphere"}, Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP, Selector: map[string]string{"cnpg.io/cluster": "masterdata"}}},
			}
			firstDelete := kube.EXPECT().Delete(mock.Anything, &staleServices[0]).Return(nil).Once()
			secondDelete := kube.EXPECT().Delete(mock.Anything, &staleServices[1]).Return(nil).Once()

			prepareDatabase()
			expectNodes(corev1.NodeInternalIP, "10.0.0.25")
			expectCreate().NotBefore(firstDelete, secondDelete)

			endpoint, err := pg.CreateMigrationEndpoint(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(endpoint.Port).To(Equal(32123))
		})

		It("cleans up leftovers even when the database readiness check then fails", func() {
			staleServices = []corev1.Service{{ObjectMeta: metav1.ObjectMeta{Name: "oms-masterdata-nodeport-old", Namespace: "codesphere"}, Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeNodePort, Selector: map[string]string{"cnpg.io/cluster": "masterdata"}}}}
			deleted := kube.EXPECT().Delete(mock.Anything, &staleServices[0]).Return(nil).Once()
			failure := errors.New("database status unavailable")
			kube.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(failure).Once().NotBefore(deleted)

			endpoint, err := pg.CreateMigrationEndpoint(context.Background())
			Expect(endpoint).To(BeNil())
			Expect(errors.Is(err, failure)).To(BeTrue())
		})

		It("continues when a leftover Service has already been deleted", func() {
			staleServices = []corev1.Service{{ObjectMeta: metav1.ObjectMeta{Name: "oms-masterdata-nodeport-old", Namespace: "codesphere"}, Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeNodePort, Selector: map[string]string{"cnpg.io/cluster": "masterdata"}}}}
			deleted := kube.EXPECT().Delete(mock.Anything, &staleServices[0]).Return(apierrors.NewNotFound(schema.GroupResource{Resource: "services"}, staleServices[0].Name)).Once()

			prepareDatabase()
			expectNodes(corev1.NodeInternalIP, "10.0.0.25")
			expectCreate().NotBefore(deleted)

			_, err := pg.CreateMigrationEndpoint(context.Background())
			Expect(err).NotTo(HaveOccurred())
		})

		It("does not create a new endpoint when listing leftovers fails", func() {
			listError = errors.New("cannot list services")

			endpoint, err := pg.CreateMigrationEndpoint(context.Background())
			Expect(endpoint).To(BeNil())
			Expect(errors.Is(err, listError)).To(BeTrue())
			Expect(err).To(MatchError(ContainSubstring("list temporary PostgreSQL NodePort services")))
		})

		It("does not create a new endpoint when deleting a leftover fails", func() {
			staleServices = []corev1.Service{{ObjectMeta: metav1.ObjectMeta{Name: "oms-masterdata-nodeport-old", Namespace: "codesphere"}, Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeNodePort, Selector: map[string]string{"cnpg.io/cluster": "masterdata"}}}}
			failure := errors.New("cannot delete service")
			kube.EXPECT().Delete(mock.Anything, &staleServices[0]).Return(failure).Once()

			endpoint, err := pg.CreateMigrationEndpoint(context.Background())
			Expect(endpoint).To(BeNil())
			Expect(errors.Is(err, failure)).To(BeTrue())
			Expect(err).To(MatchError(ContainSubstring("delete stale PostgreSQL NodePort service codesphere/oms-masterdata-nodeport-old")))
		})

		It("exposes the primary through a node address and cleans up after cancellation", func() {
			prepareDatabase()
			expectNodes(corev1.NodeInternalIP, "10.0.0.25")
			expectCreate()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			endpoint, err := pg.CreateMigrationEndpoint(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(endpoint.Host).To(Equal("10.0.0.25"))
			Expect(endpoint.Port).To(Equal(32123))
			cancel()
			kube.EXPECT().Delete(mock.Anything, created).RunAndReturn(func(ctx context.Context, _ client.Object, _ ...client.DeleteOption) error {
				Expect(ctx.Err()).NotTo(HaveOccurred())
				_, hasDeadline := ctx.Deadline()
				Expect(hasDeadline).To(BeTrue())

				return nil
			}).Once()
			Expect(endpoint.Close()).To(Succeed())
		})

		It("falls back to an external node address and accepts an already deleted Service", func() {
			prepareDatabase()
			expectNodes(corev1.NodeExternalIP, "203.0.113.25")
			expectCreate()

			endpoint, err := pg.CreateMigrationEndpoint(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(endpoint.Host).To(Equal("203.0.113.25"))
			kube.EXPECT().Delete(mock.Anything, created).Return(apierrors.NewNotFound(schema.GroupResource{Resource: "services"}, created.Name)).Once()
			Expect(endpoint.Close()).To(Succeed())
		})

		It("reports deletion failures", func() {
			prepareDatabase()
			expectNodes(corev1.NodeInternalIP, "10.0.0.25")
			expectCreate()

			endpoint, err := pg.CreateMigrationEndpoint(context.Background())
			Expect(err).NotTo(HaveOccurred())

			failure := errors.New("access denied")
			kube.EXPECT().Delete(mock.Anything, created).Return(failure).Once()
			Expect(endpoint.Close()).To(MatchError(ContainSubstring("delete temporary PostgreSQL NodePort service")))
		})

		It("does not create a Service when nodes have no usable address", func() {
			prepareDatabase()
			expectNodes(corev1.NodeHostName, "node-1")

			endpoint, err := pg.CreateMigrationEndpoint(context.Background())
			Expect(endpoint).To(BeNil())
			Expect(err).To(MatchError(ContainSubstring("no node IP address")))
		})

		It("reports Service creation failures", func() {
			prepareDatabase()
			expectNodes(corev1.NodeInternalIP, "10.0.0.25")

			failure := errors.New("nodePort range exhausted")
			kube.EXPECT().Create(mock.Anything, mock.AnythingOfType("*v1.Service")).Return(failure).Once()

			endpoint, err := pg.CreateMigrationEndpoint(context.Background())
			Expect(endpoint).To(BeNil())
			Expect(errors.Is(err, failure)).To(BeTrue())
		})

		It("cleans up if the API response has no allocated NodePort", func() {
			prepareDatabase()
			expectNodes(corev1.NodeInternalIP, "10.0.0.25")
			kube.EXPECT().Create(mock.Anything, mock.AnythingOfType("*v1.Service")).Return(nil).Once()
			kube.EXPECT().Delete(mock.Anything, mock.AnythingOfType("*v1.Service")).Return(nil).Once()

			endpoint, err := pg.CreateMigrationEndpoint(context.Background())
			Expect(endpoint).To(BeNil())
			Expect(err).To(MatchError(ContainSubstring("no allocated nodePort")))
		})
	})
})
