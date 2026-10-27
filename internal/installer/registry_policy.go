// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"context"
	"fmt"
	"strconv"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/version"
	versioninfo "k8s.io/apimachinery/pkg/version"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const registryPolicyName = "codesphere-image-registry"

// RegistryPolicyClient is the Kubernetes API surface needed to apply admission resources.
//
//mockery:generate: true
type RegistryPolicyClient interface {
	Apply(context.Context, runtime.ApplyConfiguration, ...ctrlclient.ApplyOption) error
}

// ClusterVersionClient reads the configured Kubernetes API server version.
//
//mockery:generate: true
type ClusterVersionClient interface {
	ServerVersion() (*versioninfo.Info, error)
}

// EnsureRegistryPolicy installs the policy and binding before dependency workloads.
// It requires Kubernetes 1.34 or newer and uses the beta API before 1.36.
// The default registry needs no rewriting and causes no Kubernetes API writes.
func EnsureRegistryPolicy(ctx context.Context, client RegistryPolicyClient, base string, versions ClusterVersionClient) error {
	registry, err := NormalizeRegistry(base)
	if err != nil {
		return fmt.Errorf("invalid image registry: %w", err)
	}

	if registry == DefaultRegistryURL {
		return nil
	}

	apiVersion, err := registryPolicyAPIVersion(versions)
	if err != nil {
		return err
	}

	failurePolicy := admissionv1.Fail

	policy := &admissionv1.MutatingAdmissionPolicy{
		TypeMeta:   metav1.TypeMeta{APIVersion: admissionv1.SchemeGroupVersion.String(), Kind: "MutatingAdmissionPolicy"},
		ObjectMeta: metav1.ObjectMeta{Name: registryPolicyName},
		Spec: admissionv1.MutatingAdmissionPolicySpec{
			MatchConstraints: &admissionv1.MatchResources{ResourceRules: []admissionv1.NamedRuleWithOperations{{
				RuleWithOperations: admissionv1.RuleWithOperations{
					Operations: []admissionv1.OperationType{admissionv1.Create},
					Rule:       admissionv1.Rule{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"pods"}},
				},
			}}},
			FailurePolicy:      &failurePolicy,
			ReinvocationPolicy: admissionv1.IfNeededReinvocationPolicy,
			Variables: []admissionv1.Variable{
				{Name: "oldRegistry", Expression: strconv.Quote(DefaultRegistryURL + "/codesphere-cloud/")},
				{Name: "newRegistry", Expression: strconv.Quote(registry + "/codesphere-cloud/")},
			},
			Mutations: []admissionv1.Mutation{{
				PatchType: admissionv1.PatchTypeJSONPatch,
				JSONPatch: &admissionv1.JSONPatch{Expression: registryImagePatches("containers") + " + " +
					"(has(object.spec.initContainers) ? " + registryImagePatches("initContainers") + " : [])"},
			}},
		},
	}
	if err := applyRegistryResource(ctx, client, policy, apiVersion); err != nil {
		return fmt.Errorf("failed to apply image registry policy: %w", err)
	}

	binding := &admissionv1.MutatingAdmissionPolicyBinding{
		TypeMeta:   metav1.TypeMeta{APIVersion: admissionv1.SchemeGroupVersion.String(), Kind: "MutatingAdmissionPolicyBinding"},
		ObjectMeta: metav1.ObjectMeta{Name: registryPolicyName},
		Spec:       admissionv1.MutatingAdmissionPolicyBindingSpec{PolicyName: registryPolicyName},
	}
	if err := applyRegistryResource(ctx, client, binding, apiVersion); err != nil {
		return fmt.Errorf("failed to apply image registry policy binding: %w", err)
	}

	return nil
}

// registryPolicyAPIVersion selects the mutating admission policy API for the server's
// Kubernetes version. It returns "admissionregistration.k8s.io/v1beta1" for 1.34–1.35
// and "admissionregistration.k8s.io/v1" for 1.36 or newer, comparing release numbers
// without vendor suffixes. It returns an empty string and an error if version
// discovery or parsing fails, or if the cluster is older than 1.34.
func registryPolicyAPIVersion(client ClusterVersionClient) (string, error) {
	info, err := client.ServerVersion()
	if err != nil {
		return "", fmt.Errorf("failed to check Kubernetes version for image registry policy: %w", err)
	}

	serverVersion, err := version.ParseSemantic(info.GitVersion)
	if err != nil {
		return "", fmt.Errorf("failed to parse Kubernetes server version %q: %w", info.GitVersion, err)
	}

	// Compare release numbers so vendor suffixes such as -eks-... remain supported.
	if !serverVersion.AtLeast(version.MustParseGeneric("v1.34.0")) {
		return "", fmt.Errorf("image registry policy requires Kubernetes 1.34 or newer; configured cluster is %s", info.GitVersion)
	}

	if !serverVersion.AtLeast(version.MustParseGeneric("v1.36.0")) {
		return "admissionregistration.k8s.io/v1beta1", nil
	}

	return admissionv1.SchemeGroupVersion.String(), nil
}

func applyRegistryResource(ctx context.Context, client RegistryPolicyClient, obj ctrlclient.Object, apiVersion string) error {
	data, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return fmt.Errorf("failed to encode admission resource: %w", err)
	}

	resource := &unstructured.Unstructured{Object: data}
	resource.SetAPIVersion(apiVersion)

	if err := client.Apply(ctx, ctrlclient.ApplyConfigurationFromUnstructured(resource), ctrlclient.FieldOwner(registryPolicyName)); err != nil {
		return fmt.Errorf("failed to apply admission resource: %w", err)
	}

	return nil
}

func registryImagePatches(field string) string {
	return fmt.Sprintf(`object.spec.%[1]s
		.map(c, object.spec.%[1]s.indexOf(c))
		.filter(i, object.spec.%[1]s[i].image.startsWith(variables.oldRegistry))
		.map(i, JSONPatch{
			op: "replace",
			path: "/spec/%[1]s/" + string(i) + "/image",
			value: variables.newRegistry + object.spec.%[1]s[i].image.substring(size(variables.oldRegistry))
		})`, field)
}
