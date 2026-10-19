// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("local CSI storage", func() {
	It("installs the provisioner and makes its storage class the default", func() {
		scheme := runtime.NewScheme()
		Expect(storagev1.AddToScheme(scheme)).To(Succeed())
		kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			&storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: localStorageClassName}},
			&storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "standard", Annotations: map[string]string{
				defaultStorageClassAnnotation: "true", "keep": "value",
			}}},
			&storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "legacy", Annotations: map[string]string{
				legacyDefaultClassAnnotation: "TRUE",
			}}},
		).Build()
		helm := installer.NewMockHelmClient(GinkgoT())
		helm.EXPECT().LoginRegistry(mock.Anything, "ghcr.io", "user", "password").Return(nil)
		helm.EXPECT().FindRelease(localCSINamespace, localCSIReleaseName).Return(nil, nil)
		helm.EXPECT().InstallChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
			storageClass, ok := cfg.Values["storageClass"].(map[string]interface{})

			return ok && cfg.ChartName == "oci://ghcr.io/codesphere-cloud/charts/local-csi-provisioner" &&
				cfg.Version == "0.2.0" && cfg.Namespace == "local-csi" && cfg.CreateNamespace &&
				storageClass["name"] == "local-rwx" && storageClass["isDefault"] == true
		}), installer.InstallChartOptions{}).Return(nil)
		b := &LocalBootstrapper{
			ctx: context.Background(), helmClient: helm, kubeClient: kube,
			Env: &CodesphereEnvironment{RegistryUser: "user", RegistryPassword: "password"},
		}
		Expect(b.InstallLocalCSIProvisioner()).To(Succeed())
		Expect(b.setLocalStorageClassDefault()).To(Succeed())

		for _, tc := range []struct {
			name        string
			wantDefault bool
		}{
			{localStorageClassName, true}, {"standard", false}, {"legacy", false},
		} {
			class := &storagev1.StorageClass{}
			Expect(kube.Get(b.ctx, client.ObjectKey{Name: tc.name}, class)).To(Succeed())
			Expect(isDefaultStorageClass(class.Annotations)).To(Equal(tc.wantDefault), "StorageClass %q", tc.name)

			if tc.name == "standard" && class.Annotations["keep"] != "value" {
				Fail("unrelated StorageClass annotation was removed")
			}
		}
	})

	It("configures local storage values idempotently", func() {
		b := &LocalBootstrapper{Env: &CodesphereEnvironment{
			StorageEngine: StorageEngineLocal,
			InternalFlags: []string{"existing"},
			InstallConfig: &files.RootConfig{Codesphere: files.CodesphereConfig{
				ManagedServices: []files.ManagedServiceConfig{{Name: "postgres"}, {Name: "s3"}},
				Override: files.ChartOverride{"global": map[string]any{
					"ceph": map[string]any{"other": "preserved"},
				}},
			}, ManagedServiceBackends: &files.ManagedServiceBackendsConfig{S3: &files.S3ManagedServiceConfig{}}},
		}}
		b.configureStorageValues()
		b.configureStorageValues()
		Expect(b.storageClassName()).To(Equal(localStorageClassName))
		Expect(b.Env.InstallConfig.Codesphere.Internal).To(Equal([]string{"existing", "workspace-pvc-storage"}))
		ceph := b.Env.InstallConfig.Codesphere.Override["global"].(map[string]any)["ceph"].(map[string]any)
		Expect(ceph).To(HaveKeyWithValue("storageClass", localStorageClassName))
		Expect(ceph).To(HaveKeyWithValue("other", "preserved"))

		s3 := b.Env.InstallConfig.ManagedServiceBackends.S3
		Expect(s3).NotTo(BeNil())
		Expect(s3.Enabled).NotTo(BeNil())
		Expect(*s3.Enabled).To(BeFalse())
		Expect(b.Env.InstallConfig.Codesphere.ManagedServices).To(HaveLen(1))
		Expect(b.Env.InstallConfig.Codesphere.ManagedServices[0].Name).To(Equal("postgres"))
		data, err := b.Env.InstallConfig.Marshal()
		Expect(err).NotTo(HaveOccurred())

		var restored files.RootConfig
		Expect(restored.Unmarshal(data)).To(Succeed())
		Expect(restored.ManagedServiceBackends.S3).NotTo(BeNil())
		Expect(restored.ManagedServiceBackends.S3.Enabled).NotTo(BeNil())
		Expect(*restored.ManagedServiceBackends.S3.Enabled).To(BeFalse())
	})

	It("keeps S3 with Rook storage", func() {
		config := &files.RootConfig{
			Codesphere: files.CodesphereConfig{ManagedServices: []files.ManagedServiceConfig{{Name: "s3"}}},
			ManagedServiceBackends: &files.ManagedServiceBackendsConfig{
				S3: &files.S3ManagedServiceConfig{},
			},
		}
		b := &LocalBootstrapper{Env: &CodesphereEnvironment{
			StorageEngine: StorageEngineRookCeph,
			InstallConfig: config,
		}}
		b.configureStorageValues()
		Expect(config.ManagedServiceBackends.S3).NotTo(BeNil())
		Expect(config.ManagedServiceBackends.S3.Enabled).To(BeNil())
		Expect(config.Codesphere.ManagedServices).To(HaveLen(1))
		Expect(config.Codesphere.ManagedServices[0].Name).To(Equal("s3"))
	})
})
