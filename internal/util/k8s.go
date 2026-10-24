// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	sigyaml "sigs.k8s.io/yaml"
)

// RenderTemplate performs simple ${VAR} substitution on a raw byte slice.
func RenderTemplate(raw []byte, vars map[string]string) ([]byte, error) {
	content := string(raw)
	for key, val := range vars {
		content = strings.ReplaceAll(content, "${"+key+"}", val)
	}
	return []byte(content), nil
}

// VaultGVR returns the GroupVersionResource for the bank-vaults Vault CRD.
func VaultGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    "vault.banzaicloud.com",
		Version:  "v1alpha1",
		Resource: "vaults",
	}
}

// ApplySecretFromYAML creates or updates a corev1.Secret parsed from YAML bytes.
func ApplySecretFromYAML(ctx context.Context, clientset kubernetes.Interface, data []byte) error {
	secret := &corev1.Secret{}
	if err := sigyaml.Unmarshal(data, secret); err != nil {
		return fmt.Errorf("unmarshaling secret yaml: %w", err)
	}

	secretsClient := clientset.CoreV1().Secrets(secret.Namespace)

	existing, err := secretsClient.Get(ctx, secret.Name, metav1.GetOptions{})
	if err != nil {
		_, err = secretsClient.Create(ctx, secret, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("creating secret %s/%s: %w", secret.Namespace, secret.Name, err)
		}
		return nil
	}

	secret.ResourceVersion = existing.ResourceVersion
	_, err = secretsClient.Update(ctx, secret, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("updating secret %s/%s: %w", secret.Namespace, secret.Name, err)
	}
	return nil
}

// NewClients creates both a typed and dynamic Kubernetes client using the
// current kubeconfig context.
func NewClients() (kubernetes.Interface, dynamic.Interface, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	cfg, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("loading kubeconfig: %w", err)
	}

	return NewClientsFromRESTConfig(cfg)
}

func NewClientsFromRESTConfig(cfg *rest.Config) (kubernetes.Interface, dynamic.Interface, error) {
	if cfg == nil {
		return nil, nil, fmt.Errorf("rest config is required")
	}

	clientset, err := kubernetes.NewForConfig(rest.CopyConfig(cfg))
	if err != nil {
		return nil, nil, fmt.Errorf("creating kubernetes clientset: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(rest.CopyConfig(cfg))
	if err != nil {
		return nil, nil, fmt.Errorf("creating dynamic client: %w", err)
	}

	return clientset, dynClient, nil
}
