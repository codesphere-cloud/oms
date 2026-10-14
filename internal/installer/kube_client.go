// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"fmt"

	argov1alpha1 "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// NewKubeClient creates a client that knows the core Kubernetes and the ArgoCD types.
func NewKubeClient(restConfig *rest.Config) (ctrlclient.Client, error) {
	scheme := k8sruntime.NewScheme()
	if err := k8sscheme.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add Kubernetes core scheme: %w", err)
	}

	if err := argov1alpha1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add ArgoCD scheme: %w", err)
	}

	kubeClient, err := ctrlclient.New(restConfig, ctrlclient.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	return kubeClient, nil
}
