// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package clusteradmin_test

import (
	"context"

	"github.com/codesphere-cloud/oms/internal/clusteradmin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

var _ = Describe("AddClusterAdmin", func() {
	var (
		ctx       context.Context
		clientset *fake.Clientset
		opts      clusteradmin.Opts
	)

	BeforeEach(func() {
		ctx = context.Background()
		clientset = fake.NewClientset()
		opts = clusteradmin.Opts{
			Email:      "niklas@codesphere.com",
			Namespace:  clusteradmin.DefaultNamespace,
			SecretName: clusteradmin.DefaultSecretName,
		}
	})

	getEmail := func() string {
		secret, err := clientset.CoreV1().Secrets(opts.Namespace).Get(ctx, opts.SecretName, metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())
		return string(secret.Data[clusteradmin.EmailKey])
	}

	It("creates the secret when it does not exist yet", func() {
		Expect(clusteradmin.AddClusterAdmin(ctx, clientset, opts)).To(Succeed())

		secret, err := clientset.CoreV1().Secrets(opts.Namespace).Get(ctx, opts.SecretName, metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())
		Expect(secret.Type).To(Equal(corev1.SecretTypeOpaque))
		Expect(getEmail()).To(Equal("niklas@codesphere.com"))
	})

	It("overwrites the email in an existing secret", func() {
		Expect(clusteradmin.AddClusterAdmin(ctx, clientset, opts)).To(Succeed())

		opts.Email = "alice@codesphere.com"
		Expect(clusteradmin.AddClusterAdmin(ctx, clientset, opts)).To(Succeed())

		Expect(getEmail()).To(Equal("alice@codesphere.com"))
	})

	It("is idempotent when setting the same email twice", func() {
		Expect(clusteradmin.AddClusterAdmin(ctx, clientset, opts)).To(Succeed())
		Expect(clusteradmin.AddClusterAdmin(ctx, clientset, opts)).To(Succeed())

		Expect(getEmail()).To(Equal("niklas@codesphere.com"))
	})

	It("lowercases the email", func() {
		opts.Email = "NIKLAS@codesphere.com"
		Expect(clusteradmin.AddClusterAdmin(ctx, clientset, opts)).To(Succeed())

		Expect(getEmail()).To(Equal("niklas@codesphere.com"))
	})

	It("preserves other keys in a pre-existing secret", func() {
		_, err := clientset.CoreV1().Secrets(opts.Namespace).Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: opts.SecretName, Namespace: opts.Namespace},
			Data:       map[string][]byte{"other": []byte("keep")},
		}, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		Expect(clusteradmin.AddClusterAdmin(ctx, clientset, opts)).To(Succeed())

		secret, err := clientset.CoreV1().Secrets(opts.Namespace).Get(ctx, opts.SecretName, metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())
		Expect(string(secret.Data["other"])).To(Equal("keep"))
		Expect(string(secret.Data[clusteradmin.EmailKey])).To(Equal("niklas@codesphere.com"))
	})

	It("rejects an empty email", func() {
		opts.Email = "   "
		Expect(clusteradmin.AddClusterAdmin(ctx, clientset, opts)).To(MatchError(ContainSubstring("must not be empty")))
	})

	It("rejects an invalid email", func() {
		opts.Email = "not-an-email"
		Expect(clusteradmin.AddClusterAdmin(ctx, clientset, opts)).To(MatchError(ContainSubstring("invalid email")))
	})
})
