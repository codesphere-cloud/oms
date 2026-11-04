// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"fmt"

	argov1alpha1 "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// NewKubeClient creates a client that knows the core Kubernetes and the ArgoCD types.
func NewKubeClient(restConfig *rest.Config) (ctrlclient.Client, error) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add Kubernetes core scheme: %w", err)
	}

	if err := argov1alpha1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add ArgoCD scheme: %w", err)
	}

	kubeClient, err := ctrlclient.New(restConfig, ctrlclient.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to build controller-runtime client: %w", err)
	}

	return kubeClient, nil
}
