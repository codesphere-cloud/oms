// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package clusteradmin

import (
	"context"
	"fmt"
	"log"
	"net/mail"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	// DefaultNamespace is the namespace the cluster-admin-email secret lives in.
	DefaultNamespace = "codesphere"
	// DefaultSecretName is the name of the secret holding the cluster admin email.
	// It is referenced by the platform deployment via a secretKeyRef.
	DefaultSecretName = "cluster-admin-email"
	// EmailKey is the secret data key under which the admin email is stored.
	EmailKey = "email"
)

// Opts contains the options for adding a cluster admin.
type Opts struct {
	Email      string
	Namespace  string
	SecretName string
}

// AddClusterAdmin writes the given email to the cluster-admin-email secret in
// the target cluster, creating the secret if it does not exist yet or updating
// it otherwise.
//
// The email is stored under the EmailKey data key. Running the command again
// with a different email overwrites the previous value.
func AddClusterAdmin(ctx context.Context, clientset kubernetes.Interface, opts Opts) error {
	email, err := normalizeEmail(opts.Email)
	if err != nil {
		return err
	}

	secrets := clientset.CoreV1().Secrets(opts.Namespace)

	existing, err := secrets.Get(ctx, opts.SecretName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      opts.SecretName,
				Namespace: opts.Namespace,
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				EmailKey: []byte(email),
			},
		}
		if _, err := secrets.Create(ctx, secret, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("creating secret %s/%s: %w", opts.Namespace, opts.SecretName, err)
		}
		log.Printf("Created secret '%s' in namespace '%s' with cluster admin email '%s'", opts.SecretName, opts.Namespace, email)
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading secret %s/%s: %w", opts.Namespace, opts.SecretName, err)
	}

	if existing.Data == nil {
		existing.Data = map[string][]byte{}
	}
	if string(existing.Data[EmailKey]) == email {
		log.Printf("Cluster admin email '%s' already set in secret '%s/%s', nothing to do", email, opts.Namespace, opts.SecretName)
		return nil
	}
	existing.Data[EmailKey] = []byte(email)

	if _, err := secrets.Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("updating secret %s/%s: %w", opts.Namespace, opts.SecretName, err)
	}
	log.Printf("Set cluster admin email '%s' in secret '%s/%s'", email, opts.Namespace, opts.SecretName)
	return nil
}

// normalizeEmail validates and canonicalizes an email address.
func normalizeEmail(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("email must not be empty")
	}
	addr, err := mail.ParseAddress(trimmed)
	if err != nil {
		return "", fmt.Errorf("invalid email %q: %w", raw, err)
	}
	return strings.ToLower(addr.Address), nil
}
