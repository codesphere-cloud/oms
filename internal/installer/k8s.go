// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	sigyaml "sigs.k8s.io/yaml"
)

// DecodeMultiDocYAML splits a multi-document YAML byte slice into
// individual unstructured objects. This handles the "---" separators.
func DecodeMultiDocYAML(data []byte) ([]*unstructured.Unstructured, error) {
	var objects []*unstructured.Unstructured

	reader := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)
	for {
		obj := &unstructured.Unstructured{}
		if err := reader.Decode(obj); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("decoding yaml document: %w", err)
		}
		if obj.Object == nil {
			continue
		}
		objects = append(objects, obj)
	}

	return objects, nil
}

// RenderTemplate performs simple ${VAR} substitution on a raw byte slice.
func RenderTemplate(raw []byte, vars map[string]string) ([]byte, error) {
	content := string(raw)
	for key, val := range vars {
		content = strings.ReplaceAll(content, "${"+key+"}", val)
	}
	return []byte(content), nil
}

// GvrForUnstructured maps an unstructured object's GVK to the appropriate GVR.
func GvrForUnstructured(obj *unstructured.Unstructured) (schema.GroupVersionResource, error) {
	gvk := obj.GroupVersionKind()

	switch gvk.Kind {
	case "AppProject":
		return schema.GroupVersionResource{
			Group:    gvk.Group,
			Version:  gvk.Version,
			Resource: "appprojects",
		}, nil
	default:
		return schema.GroupVersionResource{}, fmt.Errorf("no GVR mapping for %s", gvk)
	}
}

// ApplyUnstructured creates or updates an unstructured resource using the dynamic client.
func ApplyUnstructured(ctx context.Context, dynClient dynamic.Interface, gvr schema.GroupVersionResource, obj *unstructured.Unstructured) error {
	ns := obj.GetNamespace()
	name := obj.GetName()
	resource := dynClient.Resource(gvr).Namespace(ns)

	existing, err := resource.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		_, err = resource.Create(ctx, obj, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("creating %s %s/%s: %w", gvr.Resource, ns, name, err)
		}
		return nil
	}

	obj.SetResourceVersion(existing.GetResourceVersion())
	_, err = resource.Update(ctx, obj, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("updating %s %s/%s: %w", gvr.Resource, ns, name, err)
	}
	return nil
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

// newClients creates both a typed and dynamic Kubernetes client
// using the current kubeconfig context (respects KUBECONFIG env var
// and defaults to ~/.kube/config).
func newClients() (kubernetes.Interface, dynamic.Interface, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	cfg, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("loading kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("creating kubernetes clientset: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("creating dynamic client: %w", err)
	}

	return clientset, dynClient, nil
}
