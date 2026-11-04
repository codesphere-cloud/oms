// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	admissionv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilnet "k8s.io/apimachinery/pkg/util/net"
	"k8s.io/apimachinery/pkg/util/version"
	"k8s.io/apimachinery/pkg/util/wait"
	versioninfo "k8s.io/apimachinery/pkg/version"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	registryPolicyName         = "codesphere-image-registry"
	registryPolicyKind         = "MutatingAdmissionPolicy"
	registryPolicyBindingKind  = "MutatingAdmissionPolicyBinding"
	codesphereImagePrefix      = DefaultRegistryURL + "/codesphere-cloud/"
	registryPolicyReadyTimeout = 30 * time.Second
	registryPolicyPollInterval = time.Second
)

// RegistryPolicyClient is the Kubernetes API surface needed to apply and remove admission resources.
//
//mockery:generate: true
type RegistryPolicyClient interface {
	Apply(context.Context, runtime.ApplyConfiguration, ...ctrlclient.ApplyOption) error
	Create(context.Context, ctrlclient.Object, ...ctrlclient.CreateOption) error
	Delete(context.Context, ctrlclient.Object, ...ctrlclient.DeleteOption) error
}

// ClusterVersionClient reads the configured Kubernetes API server version.
//
//mockery:generate: true
type ClusterVersionClient interface {
	ServerVersion() (*versioninfo.Info, error)
}

// EnsureRegistryPolicy installs the policy and binding before dependency workloads.
// It also rewrites CNPG Cluster imageName on create/update so operators use the
// same mirrored image as their Pods.
// It requires Kubernetes 1.34 or newer and uses the beta API before 1.36,
// where the MutatingAdmissionPolicy feature and v1beta1 API must be enabled.
// After applying both resources, it verifies container and init-container mutation
// through dry-run Pod admission, waiting at most 30 seconds before returning an error.
// The default registry needs no rewriting: it removes an existing Codesphere policy
// and binding left by a previous custom registry and otherwise changes nothing.
func EnsureRegistryPolicy(ctx context.Context, client RegistryPolicyClient, base string, versions ClusterVersionClient) error {
	registry, err := NormalizeRegistry(base)
	if err != nil {
		return err
	}

	if registry == DefaultRegistryURL {
		return removeRegistryPolicy(ctx, client)
	}

	apiVersion, err := registryPolicyAPIVersion(versions)
	if err != nil {
		return err
	}

	failurePolicy := admissionv1.Fail

	policy := &admissionv1.MutatingAdmissionPolicy{
		TypeMeta:   metav1.TypeMeta{APIVersion: admissionv1.SchemeGroupVersion.String(), Kind: registryPolicyKind},
		ObjectMeta: metav1.ObjectMeta{Name: registryPolicyName},
		Spec: admissionv1.MutatingAdmissionPolicySpec{
			MatchConstraints: &admissionv1.MatchResources{ResourceRules: []admissionv1.NamedRuleWithOperations{{
				RuleWithOperations: admissionv1.RuleWithOperations{
					Operations: []admissionv1.OperationType{admissionv1.Create},
					Rule:       admissionv1.Rule{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"pods"}},
				},
			}, {
				RuleWithOperations: admissionv1.RuleWithOperations{
					Operations: []admissionv1.OperationType{admissionv1.Create, admissionv1.Update},
					Rule:       admissionv1.Rule{APIGroups: []string{"postgresql.cnpg.io"}, APIVersions: []string{"v1"}, Resources: []string{"clusters"}},
				},
			}}},
			FailurePolicy:      &failurePolicy,
			ReinvocationPolicy: admissionv1.IfNeededReinvocationPolicy,
			Variables: []admissionv1.Variable{
				{Name: "oldRegistry", Expression: strconv.Quote(codesphereImagePrefix)},
				{Name: "newRegistry", Expression: strconv.Quote(registry + "/codesphere-cloud/")},
			},
			Mutations: []admissionv1.Mutation{{
				PatchType: admissionv1.PatchTypeJSONPatch,
				JSONPatch: &admissionv1.JSONPatch{Expression: `request.resource.group == "" ? (` + registryImagePatches("containers") + " + " +
					"(has(object.spec.initContainers) ? " + registryImagePatches("initContainers") + " : [])) : []"},
			}, {
				PatchType: admissionv1.PatchTypeJSONPatch,
				JSONPatch: &admissionv1.JSONPatch{Expression: `request.resource.group == "postgresql.cnpg.io" ? (` + registryClusterImagePatch + ") : []"},
			}},
		},
	}
	if err := applyRegistryResource(ctx, client, policy, apiVersion); err != nil {
		return fmt.Errorf("failed to apply image registry policy: %w", err)
	}

	binding := &admissionv1.MutatingAdmissionPolicyBinding{
		TypeMeta:   metav1.TypeMeta{APIVersion: admissionv1.SchemeGroupVersion.String(), Kind: registryPolicyBindingKind},
		ObjectMeta: metav1.ObjectMeta{Name: registryPolicyName},
		Spec:       admissionv1.MutatingAdmissionPolicyBindingSpec{PolicyName: registryPolicyName},
	}
	if err := applyRegistryResource(ctx, client, binding, apiVersion); err != nil {
		return fmt.Errorf("failed to apply image registry policy binding: %w", err)
	}

	return waitForRegistryPolicy(ctx, client, registry)
}

// removeRegistryPolicy deletes the binding before its policy so admission stops rewriting
// images first. Version discovery is skipped so clusters without the policy API succeed.
// When anything was removed, it waits until dry-run Pods keep their default images.
func removeRegistryPolicy(ctx context.Context, client RegistryPolicyClient) error {
	removedBinding, err := deleteRegistryResource(ctx, client, registryPolicyBindingKind)
	if err != nil {
		return fmt.Errorf("failed to remove image registry policy binding: %w", err)
	}

	removedPolicy, err := deleteRegistryResource(ctx, client, registryPolicyKind)
	if err != nil {
		return fmt.Errorf("failed to remove image registry policy: %w", err)
	}

	if !removedBinding && !removedPolicy {
		return nil
	}

	if err := waitForRegistryPolicy(ctx, client, DefaultRegistryURL); err != nil {
		return fmt.Errorf("failed to verify image registry policy removal: %w", err)
	}

	return nil
}

// deleteRegistryResource deletes the named admission resource through the first served
// policy API version. It reports false when the API or the resource does not exist.
func deleteRegistryResource(ctx context.Context, client RegistryPolicyClient, kind string) (bool, error) {
	for _, apiVersion := range []string{admissionv1.SchemeGroupVersion.String(), admissionv1beta1.SchemeGroupVersion.String()} {
		resource := &unstructured.Unstructured{}
		resource.SetGroupVersionKind(schema.FromAPIVersionAndKind(apiVersion, kind))
		resource.SetName(registryPolicyName)

		switch err := client.Delete(ctx, resource); {
		case meta.IsNoMatchError(err):
			continue
		case apierrors.IsNotFound(err):
			return false, nil
		case err != nil:
			return false, fmt.Errorf("failed to delete %s %s: %w", apiVersion, kind, err)
		}

		return true, nil
	}

	return false, nil
}

// waitForRegistryPolicy verifies admission has observed the policy and binding before
// dependency Pods can be created. Dry-run requests never persist the probe Pod.
func waitForRegistryPolicy(ctx context.Context, client RegistryPolicyClient, registry string) error {
	var lastProbeErr error

	err := wait.PollUntilContextTimeout(ctx, registryPolicyPollInterval, registryPolicyReadyTimeout, true, func(ctx context.Context) (bool, error) {
		pod := registryProbePod()
		if err := client.Create(ctx, pod, ctrlclient.DryRunAll); err != nil {
			if isTransientRegistryProbeError(err) {
				lastProbeErr = fmt.Errorf("dry-run registry policy probe: %w", err)
				return false, nil
			}

			lastProbeErr = nil

			return false, fmt.Errorf("dry-run registry policy probe: %w", err)
		}

		lastProbeErr = nil

		return slices.ContainsFunc(pod.Spec.Containers, func(c corev1.Container) bool {
			return c.Name == "probe" && c.Image == registry+"/codesphere-cloud/registry-probe:admission"
		}) && slices.ContainsFunc(pod.Spec.InitContainers, func(c corev1.Container) bool {
			return c.Name == "probe-init" && c.Image == registry+"/codesphere-cloud/registry-probe:init"
		}), nil
	})
	if err != nil {
		return fmt.Errorf("failed to verify image registry policy readiness (timeout %s): %w", registryPolicyReadyTimeout, errors.Join(err, lastProbeErr))
	}

	return nil
}

func isTransientRegistryProbeError(err error) bool {
	return apierrors.IsTooManyRequests(err) || apierrors.IsTimeout(err) ||
		apierrors.IsServerTimeout(err) || apierrors.IsServiceUnavailable(err) ||
		apierrors.IsInternalError(err) || utilnet.IsTimeout(err) ||
		utilnet.IsConnectionReset(err) || utilnet.IsProbableEOF(err)
}

func registryProbePod() *corev1.Pod {
	runAsNonRoot, allowPrivilegeEscalation := true, false
	container := corev1.Container{
		Name: "probe", Image: codesphereImagePrefix + "registry-probe:admission",
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: &allowPrivilegeEscalation,
			Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
		},
	}
	initContainer := *container.DeepCopy()
	initContainer.Name = "probe-init"
	initContainer.Image = codesphereImagePrefix + "registry-probe:init"

	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{GenerateName: registryPolicyName + "-probe-", Namespace: metav1.NamespaceDefault},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			SecurityContext: &corev1.PodSecurityContext{
				RunAsNonRoot:   &runAsNonRoot,
				SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
			},
			Containers: []corev1.Container{container}, InitContainers: []corev1.Container{initContainer},
		},
	}
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
		return admissionv1beta1.SchemeGroupVersion.String(), nil
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
		.filter(i, object.spec.%[1]s[i].image.startsWith(variables.oldRegistry) &&
			!object.spec.%[1]s[i].image.startsWith(variables.newRegistry))
		.map(i, JSONPatch{
			op: "replace",
			path: "/spec/%[1]s/" + string(i) + "/image",
			value: variables.newRegistry + object.spec.%[1]s[i].image.substring(size(variables.oldRegistry))
		})`, field)
}

// Only explicit imageName references are changed. ImageCatalog-based selection needs
// its own catalog policy if it is introduced in the installer.
const registryClusterImagePatch = `
	(has(object.spec.imageName) &&
	 object.spec.imageName.startsWith(variables.oldRegistry) &&
	 !object.spec.imageName.startsWith(variables.newRegistry)) ?
	[JSONPatch{
		op: "replace", path: "/spec/imageName",
		value: variables.newRegistry + object.spec.imageName.substring(size(variables.oldRegistry))
	}] : []`
