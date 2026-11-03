// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package argocd_test

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/codesphere-cloud/oms/internal/installer/argocd"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Dependency image pull credentials", func() {
	var (
		kube *MockClient
		cfg  argocd.AppInstallerConfig
		key  client.ObjectKey
	)

	BeforeEach(func() {
		kube = NewMockClient(GinkgoT())
		cfg = argocd.AppInstallerConfig{KubeClient: kube, Config: files.RootConfig{Registry: &files.RegistryConfig{Server: "10.10.0.8"}}, Vault: &files.InstallVault{Secrets: []files.SecretEntry{
			{Name: files.SecretRegistryUsername, Fields: &files.SecretFields{Password: "mirror-user"}},
			{Name: files.SecretRegistryPassword, Fields: &files.SecretFields{Password: "mirror-password"}},
		}}}
		key = client.ObjectKey{Name: "docker-regcred", Namespace: "codesphere"}
	})
	It("creates credentials before dependency hooks with ownership the cluster-config chart can adopt", func() {
		kube.EXPECT().Get(mock.Anything, key, mock.Anything).Return(apierrors.NewNotFound(schema.GroupResource{Resource: "secrets"}, key.Name)).Once()
		kube.EXPECT().Create(mock.Anything, mock.Anything).Run(func(_ context.Context, obj client.Object, _ ...client.CreateOption) {
			secret := obj.(*corev1.Secret)
			Expect(secret.Name).To(Equal(key.Name))
			Expect(secret.Namespace).To(Equal(key.Namespace))
			Expect(secret.Type).To(Equal(corev1.SecretTypeDockerConfigJson))
			Expect(secret.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "Helm"))
			Expect(secret.Annotations).To(HaveKeyWithValue("meta.helm.sh/release-name", "cluster-config"))
			Expect(secret.Annotations).To(HaveKeyWithValue("meta.helm.sh/release-namespace", "codesphere"))

			var auth struct {
				Auths map[string]struct {
					Username string `json:"username"`
					Password string `json:"password"`
				} `json:"auths"`
			}
			Expect(json.Unmarshal(secret.Data[corev1.DockerConfigJsonKey], &auth)).To(Succeed())
			Expect(auth.Auths["10.10.0.8"].Username).To(Equal("mirror-user"))
			Expect(auth.Auths["10.10.0.8"].Password).To(Equal("mirror-password"))
		}).Return(nil).Once()
		Expect(argocd.NewAppInstaller(cfg).SyncDependencyImagePullSecret(context.Background())).To(Succeed())
	})
	It("refreshes credentials while preserving existing Helm and reflector metadata", func() {
		kube.EXPECT().Get(mock.Anything, key, mock.Anything).Run(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) {
			*obj.(*corev1.Secret) = corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: key.Namespace, ResourceVersion: "1", Labels: map[string]string{"keep": "label"}, Annotations: map[string]string{"reflector.v1.k8s.emberstack.com/reflection-allowed": "true", "meta.helm.sh/release-name": "cluster-config"}}, Type: corev1.SecretTypeDockerConfigJson, Data: map[string][]byte{corev1.DockerConfigJsonKey: []byte("old-credentials")}}
		}).Return(nil).Once()
		kube.EXPECT().Update(mock.Anything, mock.Anything).Run(func(_ context.Context, obj client.Object, _ ...client.UpdateOption) {
			secret := obj.(*corev1.Secret)
			Expect(secret.Labels).To(HaveKeyWithValue("keep", "label"))
			Expect(secret.Annotations).To(HaveKeyWithValue("reflector.v1.k8s.emberstack.com/reflection-allowed", "true"))
			Expect(secret.Annotations).To(HaveKeyWithValue("meta.helm.sh/release-name", "cluster-config"))
			Expect(string(secret.Data[corev1.DockerConfigJsonKey])).To(ContainSubstring("mirror-password"))
		}).Return(nil).Once()
		Expect(argocd.NewAppInstaller(cfg).SyncDependencyImagePullSecret(context.Background())).To(Succeed())
	})
	It("does not create an anonymous secret when credentials are missing", func() {
		cfg.Vault = &files.InstallVault{}
		Expect(argocd.NewAppInstaller(cfg).SyncDependencyImagePullSecret(context.Background())).To(MatchError(ContainSubstring("registry password is missing")))
	})
	It("reports API errors without including credential contents", func() {
		kube.EXPECT().Get(mock.Anything, key, mock.Anything).Return(fmt.Errorf("access denied")).Once()

		err := argocd.NewAppInstaller(cfg).SyncDependencyImagePullSecret(context.Background())
		Expect(err).To(MatchError(ContainSubstring("access denied")))
		Expect(err.Error()).NotTo(ContainSubstring("mirror-password"))
	})
})
