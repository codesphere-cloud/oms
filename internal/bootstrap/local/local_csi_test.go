// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"testing"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/stretchr/testify/mock"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestLocalCSIProvisionerChart(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := storagev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: localStorageClassName}},
		&storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "standard", Annotations: map[string]string{
			defaultStorageClassAnnotation: "true", "keep": "value",
		}}},
		&storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "legacy", Annotations: map[string]string{
			legacyDefaultClassAnnotation: "TRUE",
		}}},
	).Build()
	helm := installer.NewMockHelmClient(t)
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
	if err := b.InstallLocalCSIProvisioner(); err != nil {
		t.Fatal(err)
	}
	if err := b.setLocalStorageClassDefault(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name        string
		wantDefault bool
	}{
		{localStorageClassName, true}, {"standard", false}, {"legacy", false},
	} {
		class := &storagev1.StorageClass{}
		if err := kube.Get(b.ctx, client.ObjectKey{Name: tc.name}, class); err != nil {
			t.Fatal(err)
		}
		if got := isDefaultStorageClass(class.Annotations); got != tc.wantDefault {
			t.Fatalf("StorageClass %q default = %t, want %t", tc.name, got, tc.wantDefault)
		}
		if tc.name == "standard" && class.Annotations["keep"] != "value" {
			t.Fatal("unrelated StorageClass annotation was removed")
		}
	}
}

func TestLocalStorageValues(t *testing.T) {
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
	if got := b.storageClassName(); got != localStorageClassName {
		t.Fatalf("storage class = %q", got)
	}
	if got := b.Env.InstallConfig.Codesphere.Internal; len(got) != 2 || got[0] != "existing" || got[1] != "workspace-pvc-storage" {
		t.Fatalf("internal flags = %v", got)
	}
	ceph := b.Env.InstallConfig.Codesphere.Override["global"].(map[string]any)["ceph"].(map[string]any)
	if ceph["storageClass"] != localStorageClassName || ceph["other"] != "preserved" {
		t.Fatalf("chart ceph values = %v", ceph)
	}
	if s3 := b.Env.InstallConfig.ManagedServiceBackends.S3; s3 == nil || s3.Enabled == nil || *s3.Enabled {
		t.Fatalf("S3 backend is not explicitly disabled: %+v", s3)
	}
	if services := b.Env.InstallConfig.Codesphere.ManagedServices; len(services) != 1 || services[0].Name != "postgres" {
		t.Fatalf("managed services = %v", services)
	}
	data, err := b.Env.InstallConfig.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	var restored files.RootConfig
	if err := restored.Unmarshal(data); err != nil {
		t.Fatal(err)
	}
	if s3 := restored.ManagedServiceBackends.S3; s3 == nil || s3.Enabled == nil || *s3.Enabled {
		t.Fatalf("serialized S3 backend is not disabled: %+v", s3)
	}
}

func TestRookStorageKeepsS3(t *testing.T) {
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
	if config.ManagedServiceBackends.S3 == nil || config.ManagedServiceBackends.S3.Enabled != nil || len(config.Codesphere.ManagedServices) != 1 || config.Codesphere.ManagedServices[0].Name != "s3" {
		t.Fatal("Rook storage should retain the S3 component")
	}
}
