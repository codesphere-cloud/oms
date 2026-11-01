// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"context"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("database setup chart", func() {
	It("with no PostgreSQL address, preparing chart values returns an error", func() {
		_, err := databaseSetupValues(files.RootConfig{}, "cs-vault", "", false)
		Expect(err).To(MatchError(ContainSubstring("PostgreSQL address is required")))
	})

	It("with a PostgreSQL Service, preparing chart values preserves its address", func() {
		cfg := files.RootConfig{Postgres: files.PostgresConfig{ServerAddress: "postgres.codesphere.svc", Port: 5432}}
		values, err := databaseSetupValues(cfg, "cs-vault", "", false)
		Expect(err).NotTo(HaveOccurred())

		postgres := values["postgres"].(map[string]interface{})
		Expect(postgres["host"]).To(Equal("postgres.codesphere.svc"))
	})

	It("with an ide deployment, preparing maintenance adds the maintenance flag", func() {
		scheme := runtime.NewScheme()
		Expect(appsv1.AddToScheme(scheme)).To(Succeed())

		labels := map[string]string{"app.kubernetes.io/part-of": "codesphere", "app": "ide-service"}
		client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "ide", Namespace: databaseSetupNamespace, Labels: labels}, Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "ide", Env: []corev1.EnvVar{{Name: "CODESPHERE_EXPERIMENTS", Value: "foo"}}}}}}}},
		).Build()
		Expect(prepareCodesphereMaintenance(context.Background(), client)).To(Succeed())

		deployment := &appsv1.Deployment{}
		Expect(client.Get(context.Background(), ctrlclient.ObjectKey{Namespace: databaseSetupNamespace, Name: "ide"}, deployment)).To(Succeed())
		Expect(deployment.Spec.Template.Spec.Containers[0].Env[0].Value).To(Equal("foo,maintenance-mode"))
	})
})
