// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"context"
	"fmt"
	"slices"
	"time"

	"k8s.io/apiextensions-apiserver/pkg/apihelpers"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CRDRequirement identifies a CRD and the API version its consumer needs.
type CRDRequirement struct{ Name, Version string }

// WaitForCRDs waits for CRDs installed by Helm, ArgoCD, or another controller.
// A definition must be Established and serve the requested API version; the
// health of the controller that installed it is deliberately not required.
func WaitForCRDs(ctx context.Context, reader client.Reader, requirements []CRDRequirement, timeout time.Duration, logf func(string, ...interface{})) error {
	var pending []string

	err := wait.PollUntilContextTimeout(ctx, 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		pending = nil

		for _, requirement := range requirements {
			crd := &apiextensionsv1.CustomResourceDefinition{}

			err := reader.Get(ctx, client.ObjectKey{Name: requirement.Name}, crd)
			if err != nil && !apierrors.IsNotFound(err) {
				return false, fmt.Errorf("reading CRD %q: %w", requirement.Name, err)
			}

			served := slices.ContainsFunc(crd.Spec.Versions, func(version apiextensionsv1.CustomResourceDefinitionVersion) bool {
				return version.Name == requirement.Version && version.Served
			})
			if apierrors.IsNotFound(err) || !apihelpers.IsCRDConditionTrue(crd, apiextensionsv1.Established) || !served {
				pending = append(pending, requirement.Name)
			}
		}

		if len(pending) > 0 && logf != nil {
			logf("Waiting for CRDs to become Established and serve required API versions: %v", pending)
		}

		return len(pending) == 0, nil
	})
	if err != nil {
		return fmt.Errorf("waiting for CRDs (pending: %v): %w", pending, err)
	}

	return nil
}
