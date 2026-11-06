// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"encoding/json"

	cnpgv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/codesphere-cloud/oms/internal/bootstrap"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("PostgreSQL database", func() {
	It("runs the upstream PostgreSQL image without a pull secret", func() {
		b := &LocalBootstrapper{stlog: bootstrap.NewStepLogger(true), Env: &CodesphereEnvironment{}}
		spec, err := b.postgresClusterSpec()
		Expect(err).NotTo(HaveOccurred())
		Expect(spec.ImageName).To(Equal("ghcr.io/cloudnative-pg/postgresql:15.14-system-trixie"))
		Expect(spec.ImagePullSecrets).To(BeEmpty())
	})

	It("pulls the PostgreSQL image from a configured mirror with its pull secret", func() {
		b := &LocalBootstrapper{
			stlog: bootstrap.NewStepLogger(true),
			Env:   &CodesphereEnvironment{RegistryURL: "registry.example.com:5000/team"},
		}
		spec, err := b.postgresClusterSpec()
		Expect(err).NotTo(HaveOccurred())
		Expect(spec.ImageName).To(Equal("registry.example.com:5000/team/ghcr_io/cloudnative-pg/postgresql:15.14-system-trixie"))
		Expect(spec.ImagePullSecrets).To(Equal([]cnpgv1.LocalObjectReference{{Name: cnpgPullSecretName}}))
	})
})

var _ = Describe("Registry pull secret", func() {
	var kube client.Client

	BeforeEach(func() {
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		kube = fake.NewClientBuilder().WithScheme(scheme).Build()
	})

	It("stores the mirror credentials keyed by the mirror host", func() {
		b := &LocalBootstrapper{
			ctx: context.Background(), kubeClient: kube,
			Env: &CodesphereEnvironment{
				RegistryURL: "registry.example.com:5000/team", RegistryUser: "user", RegistryPassword: "password",
			},
		}
		Expect(b.EnsureRegistryPullSecret()).To(Succeed())

		secret := &corev1.Secret{}
		Expect(kube.Get(b.ctx, client.ObjectKey{Name: cnpgPullSecretName, Namespace: codesphereNamespace}, secret)).To(Succeed())
		Expect(secret.Type).To(Equal(corev1.SecretTypeDockerConfigJson))

		var dockerConfig map[string]map[string]map[string]string
		Expect(json.Unmarshal(secret.Data[corev1.DockerConfigJsonKey], &dockerConfig)).To(Succeed())
		Expect(dockerConfig["auths"]).To(Equal(map[string]map[string]string{
			"registry.example.com:5000": {"username": "user", "password": "password", "auth": "dXNlcjpwYXNzd29yZA=="},
		}))
	})

	It("creates no secret for ghcr.io", func() {
		b := &LocalBootstrapper{ctx: context.Background(), kubeClient: kube, Env: &CodesphereEnvironment{}}
		Expect(b.EnsureRegistryPullSecret()).To(Succeed())

		secrets := &corev1.SecretList{}
		Expect(kube.List(b.ctx, secrets)).To(Succeed())
		Expect(secrets.Items).To(BeEmpty())
	})
})
