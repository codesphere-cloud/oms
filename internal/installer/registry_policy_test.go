// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/codesphere-cloud/oms/internal/installer"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	versioninfo "k8s.io/apimachinery/pkg/version"
	"k8s.io/apiserver/pkg/admission"
	plugincel "k8s.io/apiserver/pkg/admission/plugin/cel"
	"k8s.io/apiserver/pkg/admission/plugin/policy/mutating/patch"
	celconfig "k8s.io/apiserver/pkg/apis/cel"
	"k8s.io/apiserver/pkg/cel/environment"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func admissionResourceVersion(kind, apiVersion string) any {
	return mock.MatchedBy(func(obj runtime.ApplyConfiguration) bool {
		data, err := json.Marshal(obj)
		Expect(err).NotTo(HaveOccurred())

		metadata := metav1.TypeMeta{}
		Expect(json.Unmarshal(data, &metadata)).To(Succeed())

		return metadata.Kind == kind && metadata.APIVersion == apiVersion
	})
}

func admissionResource(kind string) any {
	return admissionResourceVersion(kind, admissionv1.SchemeGroupVersion.String())
}

const (
	betaAdmissionAPI = "admissionregistration.k8s.io/v1beta1"
	policyKind       = "MutatingAdmissionPolicy"
	bindingKind      = "MutatingAdmissionPolicyBinding"
)

var v1AdmissionAPI = admissionv1.SchemeGroupVersion.String()

func expectRegistryDelete(client *installer.MockRegistryPolicyClient, kind, apiVersion string, err error) *mock.Call {
	deletes := mock.MatchedBy(func(obj ctrlclient.Object) bool {
		gvk := obj.GetObjectKind().GroupVersionKind()

		return gvk.Kind == kind && gvk.GroupVersion().String() == apiVersion && obj.GetName() == "codesphere-image-registry"
	})

	return client.EXPECT().Delete(mock.Anything, deletes).Return(err).Once()
}

func expectNoRegistryPolicy(client *installer.MockRegistryPolicyClient) {
	expectRegistryDelete(client, bindingKind, v1AdmissionAPI, registryResourceNotFound("mutatingadmissionpolicybindings"))
	expectRegistryDelete(client, policyKind, v1AdmissionAPI, registryResourceNotFound("mutatingadmissionpolicies"))
}

func expectRegistryAPIUnavailable(client *installer.MockRegistryPolicyClient, kind string) {
	expectRegistryDelete(client, kind, v1AdmissionAPI, registryAPIUnavailable(kind, v1AdmissionAPI))
	expectRegistryDelete(client, kind, betaAdmissionAPI, registryAPIUnavailable(kind, betaAdmissionAPI))
}

func registryResourceNotFound(resource string) error {
	return apierrors.NewNotFound(schema.GroupResource{Group: "admissionregistration.k8s.io", Resource: resource}, "codesphere-image-registry")
}

func registryAPIUnavailable(kind, apiVersion string) error {
	gvk := schema.FromAPIVersionAndKind(apiVersion, kind)

	return &meta.NoKindMatchError{GroupKind: gvk.GroupKind(), SearchedVersions: []string{gvk.Version}}
}

func supportedCluster() *installer.MockClusterVersionClient {
	client := installer.NewMockClusterVersionClient(GinkgoT())
	client.EXPECT().ServerVersion().Return(&versioninfo.Info{GitVersion: "v1.36.0"}, nil).Once()

	return client
}

func expectRegistryProbe(client *installer.MockRegistryPolicyClient, registry string) *mock.Call {
	return client.EXPECT().Create(mock.Anything, mock.Anything, mock.Anything).
		Run(func(ctx context.Context, obj ctrlclient.Object, opts ...ctrlclient.CreateOption) {
			_, bounded := ctx.Deadline()
			Expect(bounded).To(BeTrue())

			options := &ctrlclient.CreateOptions{}
			for _, opt := range opts {
				opt.ApplyToCreate(options)
			}

			Expect(options.DryRun).To(Equal([]string{metav1.DryRunAll}))

			pod := obj.(*corev1.Pod)
			Expect(pod.Namespace).To(Equal(metav1.NamespaceDefault))
			Expect(pod.GenerateName).NotTo(BeEmpty())
			Expect(pod.Spec.Containers[0].Name).To(Equal("probe"))
			Expect(pod.Spec.InitContainers[0].Name).NotTo(Equal(pod.Spec.Containers[0].Name))
			Expect(pod.Spec.Containers[0].Image).To(Equal("ghcr.io/codesphere-cloud/registry-probe:admission"))
			Expect(pod.Spec.InitContainers[0].Image).To(Equal("ghcr.io/codesphere-cloud/registry-probe:init"))
			pod.Spec.Containers[0].Image = registry + "/codesphere-cloud/registry-probe:admission"
			pod.Spec.InitContainers[0].Image = registry + "/codesphere-cloud/registry-probe:init"
			// Other admission controllers may inject sidecars into the probe.
			pod.Spec.Containers = append(pod.Spec.Containers, corev1.Container{Name: "sidecar", Image: "example.com/sidecar:v1"})
		}).Return(nil).Once()
}

// Cancellation makes the retry path deterministic without waiting for a poll interval.
func expectTransientProbeRetry(cause error) {
	client := installer.NewMockRegistryPolicyClient(GinkgoT())
	client.EXPECT().Apply(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client.EXPECT().Create(mock.Anything, mock.Anything, mock.Anything).
		Run(func(_ context.Context, _ ctrlclient.Object, _ ...ctrlclient.CreateOption) {
			cancel()
		}).Return(cause).Once()
	err := installer.EnsureRegistryPolicy(ctx, client, "registry.example.com", supportedCluster())
	Expect(errors.Is(err, context.Canceled)).To(BeTrue())
	Expect(errors.Is(err, cause)).To(BeTrue())
}

func installRegistryPolicy(base string) *admissionv1.MutatingAdmissionPolicy {
	client := installer.NewMockRegistryPolicyClient(GinkgoT())

	var policy *admissionv1.MutatingAdmissionPolicy

	first := client.EXPECT().Apply(mock.Anything, admissionResource("MutatingAdmissionPolicy"), mock.Anything).
		Run(func(_ context.Context, obj runtime.ApplyConfiguration, _ ...ctrlclient.ApplyOption) {
			data, err := json.Marshal(obj)
			Expect(err).NotTo(HaveOccurred())

			policy = &admissionv1.MutatingAdmissionPolicy{}
			Expect(json.Unmarshal(data, policy)).To(Succeed())
		}).Return(nil).Once()
	binding := client.EXPECT().Apply(mock.Anything, mock.MatchedBy(func(obj runtime.ApplyConfiguration) bool {
		data, err := json.Marshal(obj)
		Expect(err).NotTo(HaveOccurred())

		binding := &admissionv1.MutatingAdmissionPolicyBinding{}
		Expect(json.Unmarshal(data, binding)).To(Succeed())

		return policy != nil && binding.Spec.PolicyName == policy.Name
	}), mock.Anything).Return(nil).Once().NotBefore(first)
	registry, err := installer.NormalizeRegistry(base)
	Expect(err).NotTo(HaveOccurred())
	expectRegistryProbe(client, registry).NotBefore(binding)
	Expect(installer.EnsureRegistryPolicy(context.Background(), client, base, supportedCluster())).To(Succeed())

	return policy
}

func mutateRegistryPod(policy *admissionv1.MutatingAdmissionPolicy, pod *corev1.Pod) *corev1.Pod {
	return mutateRegistryObject(policy, pod, corev1.SchemeGroupVersion.WithKind("Pod"), corev1.SchemeGroupVersion.WithResource("pods")).(*corev1.Pod)
}

func mutateRegistryObject(policy *admissionv1.MutatingAdmissionPolicy, obj runtime.Object, gvk schema.GroupVersionKind, gvr schema.GroupVersionResource) runtime.Object {
	compiler, err := plugincel.NewCompositedCompiler(environment.MustBaseEnvSet(environment.DefaultCompatibilityVersion()))
	Expect(err).NotTo(HaveOccurred())

	scheme := runtime.NewScheme()
	Expect(corev1.AddToScheme(scheme)).To(Succeed())

	if _, ok := obj.(*unstructured.Unstructured); ok {
		scheme.AddKnownTypeWithName(gvk, &unstructured.Unstructured{})
	}

	for _, mutation := range policy.Spec.Mutations {
		expression := mutation.JSONPatch.Expression
		for _, variable := range policy.Spec.Variables {
			expression = strings.ReplaceAll(expression, "variables."+variable.Name, variable.Expression)
		}

		evaluator := compiler.CompileMutatingEvaluator(&patch.JSONPatchCondition{Expression: expression},
			plugincel.OptionalVariableDeclarations{HasPatchTypes: true}, environment.StoredExpressions)
		attrs := admission.NewAttributesRecord(obj, nil, gvk, "default", "test", gvr, "", admission.Create, &metav1.CreateOptions{}, false, nil)
		obj, err = patch.NewJSONPatcher(evaluator).Patch(context.Background(), patch.Request{
			MatchedResource:     gvr,
			VersionedAttributes: &admission.VersionedAttributes{Attributes: attrs, VersionedKind: gvk, VersionedObject: obj},
			ObjectInterfaces:    admission.NewObjectInterfacesFromScheme(scheme),
		}, celconfig.RuntimeCELCostBudget)
		Expect(err).NotTo(HaveOccurred())
	}

	return obj
}

var _ = Describe("Registry admission policy", func() {
	It("updates the existing policy and binding when switching custom registries", func() {
		client := installer.NewMockRegistryPolicyClient(GinkgoT())

		var policies []*admissionv1.MutatingAdmissionPolicy

		client.EXPECT().Apply(mock.Anything, admissionResource("MutatingAdmissionPolicy"), mock.Anything).
			Run(func(_ context.Context, obj runtime.ApplyConfiguration, _ ...ctrlclient.ApplyOption) {
				data, err := json.Marshal(obj)
				Expect(err).NotTo(HaveOccurred())

				policy := &admissionv1.MutatingAdmissionPolicy{}
				Expect(json.Unmarshal(data, policy)).To(Succeed())
				policies = append(policies, policy)
			}).Return(nil).Twice()
		client.EXPECT().Apply(mock.Anything, admissionResource("MutatingAdmissionPolicyBinding"), mock.Anything).Return(nil).Twice()
		expectRegistryProbe(client, "old.example.com")
		Expect(installer.EnsureRegistryPolicy(context.Background(), client, "old.example.com", supportedCluster())).To(Succeed())
		expectRegistryProbe(client, "new.example.com/team")
		Expect(installer.EnsureRegistryPolicy(context.Background(), client, "new.example.com/team", supportedCluster())).To(Succeed())

		Expect(policies).To(HaveLen(2))
		Expect(policies[1].Name).To(Equal(policies[0].Name))
		pod := &corev1.Pod{Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app", Image: "ghcr.io/codesphere-cloud/api:v1"}},
		}}
		Expect(mutateRegistryPod(policies[1], pod).Spec.Containers[0].Image).To(Equal("new.example.com/team/codesphere-cloud/api:v1"))
	})

	It("rejects an invalid repository path before contacting Kubernetes", func() {
		Expect(installer.EnsureRegistryPolicy(context.Background(), installer.NewMockRegistryPolicyClient(GinkgoT()), "registry.example.com/Team", installer.NewMockClusterVersionClient(GinkgoT()))).To(MatchError(ContainSubstring("invalid image registry")))
	})

	It("rejects clusters older than 1.34 before applying resources", func() {
		versions := installer.NewMockClusterVersionClient(GinkgoT())
		versions.EXPECT().ServerVersion().Return(&versioninfo.Info{GitVersion: "v1.33.9"}, nil).Once()
		Expect(installer.EnsureRegistryPolicy(context.Background(), installer.NewMockRegistryPolicyClient(GinkgoT()), "registry.example.com", versions)).To(MatchError(ContainSubstring("requires Kubernetes 1.34 or newer; configured cluster is v1.33.9")))
	})
	It("uses the beta API on Kubernetes 1.34", func() {
		versions := installer.NewMockClusterVersionClient(GinkgoT())
		versions.EXPECT().ServerVersion().Return(&versioninfo.Info{GitVersion: "v1.34.0"}, nil).Once()

		client := installer.NewMockRegistryPolicyClient(GinkgoT())
		client.EXPECT().Apply(mock.Anything, admissionResourceVersion("MutatingAdmissionPolicy", betaAdmissionAPI), mock.Anything).Return(nil).Once()
		client.EXPECT().Apply(mock.Anything, admissionResourceVersion("MutatingAdmissionPolicyBinding", betaAdmissionAPI), mock.Anything).Return(nil).Once()
		expectRegistryProbe(client, "registry.example.com")
		Expect(installer.EnsureRegistryPolicy(context.Background(), client, "registry.example.com", versions)).To(Succeed())
	})
	It("accepts Kubernetes 1.34 distributions with vendor version suffixes", func() {
		versions := installer.NewMockClusterVersionClient(GinkgoT())
		versions.EXPECT().ServerVersion().Return(&versioninfo.Info{GitVersion: "v1.34.0-eks-example"}, nil).Once()

		client := installer.NewMockRegistryPolicyClient(GinkgoT())
		client.EXPECT().Apply(mock.Anything, admissionResourceVersion("MutatingAdmissionPolicy", betaAdmissionAPI), mock.Anything).Return(nil).Once()
		client.EXPECT().Apply(mock.Anything, admissionResourceVersion("MutatingAdmissionPolicyBinding", betaAdmissionAPI), mock.Anything).Return(nil).Once()
		expectRegistryProbe(client, "registry.example.com")
		Expect(installer.EnsureRegistryPolicy(context.Background(), client, "registry.example.com", versions)).To(Succeed())
	})
	It("reports failures to discover the cluster version before applying resources", func() {
		versions := installer.NewMockClusterVersionClient(GinkgoT())
		versions.EXPECT().ServerVersion().Return(nil, errors.New("unreachable")).Once()
		Expect(installer.EnsureRegistryPolicy(context.Background(), installer.NewMockRegistryPolicyClient(GinkgoT()), "registry.example.com", versions)).To(MatchError(ContainSubstring("failed to check Kubernetes version")))
	})
	It("reports invalid Kubernetes version responses before applying resources", func() {
		versions := installer.NewMockClusterVersionClient(GinkgoT())
		versions.EXPECT().ServerVersion().Return(&versioninfo.Info{GitVersion: "unknown"}, nil).Once()
		Expect(installer.EnsureRegistryPolicy(context.Background(), installer.NewMockRegistryPolicyClient(GinkgoT()), "registry.example.com", versions)).To(MatchError(ContainSubstring("failed to parse Kubernetes server version")))
	})

	It("changes nothing when the registry is unset and no policy exists", func() {
		client := installer.NewMockRegistryPolicyClient(GinkgoT())
		expectNoRegistryPolicy(client)
		Expect(installer.EnsureRegistryPolicy(context.Background(), client, "", installer.NewMockClusterVersionClient(GinkgoT()))).To(Succeed())
	})
	It("changes nothing for the default registry when no policy exists", func() {
		client := installer.NewMockRegistryPolicyClient(GinkgoT())
		expectNoRegistryPolicy(client)
		Expect(installer.EnsureRegistryPolicy(context.Background(), client, "ghcr.io/", installer.NewMockClusterVersionClient(GinkgoT()))).To(Succeed())
	})
	It("succeeds for the default registry on clusters without the policy API", func() {
		client := installer.NewMockRegistryPolicyClient(GinkgoT())
		expectRegistryAPIUnavailable(client, bindingKind)
		expectRegistryAPIUnavailable(client, policyKind)
		Expect(installer.EnsureRegistryPolicy(context.Background(), client, "ghcr.io", installer.NewMockClusterVersionClient(GinkgoT()))).To(Succeed())
	})
	It("removes the binding before the policy when reverting from a custom registry", func() {
		client := installer.NewMockRegistryPolicyClient(GinkgoT())
		client.EXPECT().Apply(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()
		expectRegistryProbe(client, "old.example.com")
		Expect(installer.EnsureRegistryPolicy(context.Background(), client, "old.example.com", supportedCluster())).To(Succeed())

		binding := expectRegistryDelete(client, bindingKind, v1AdmissionAPI, nil)
		policy := expectRegistryDelete(client, policyKind, v1AdmissionAPI, nil).NotBefore(binding)
		expectRegistryProbe(client, "ghcr.io").NotBefore(policy)
		Expect(installer.EnsureRegistryPolicy(context.Background(), client, "ghcr.io", installer.NewMockClusterVersionClient(GinkgoT()))).To(Succeed())
	})
	It("removes beta API resources when reverting to the default registry on Kubernetes 1.34", func() {
		client := installer.NewMockRegistryPolicyClient(GinkgoT())
		expectRegistryDelete(client, bindingKind, v1AdmissionAPI, registryAPIUnavailable(bindingKind, v1AdmissionAPI))
		binding := expectRegistryDelete(client, bindingKind, betaAdmissionAPI, nil)
		expectRegistryDelete(client, policyKind, v1AdmissionAPI, registryAPIUnavailable(policyKind, v1AdmissionAPI))
		policy := expectRegistryDelete(client, policyKind, betaAdmissionAPI, nil).NotBefore(binding)
		expectRegistryProbe(client, "ghcr.io").NotBefore(policy)
		Expect(installer.EnsureRegistryPolicy(context.Background(), client, "ghcr.io", installer.NewMockClusterVersionClient(GinkgoT()))).To(Succeed())
	})
	It("keeps the policy when removing its binding fails", func() {
		client := installer.NewMockRegistryPolicyClient(GinkgoT())
		cause := apierrors.NewForbidden(schema.GroupResource{Group: "admissionregistration.k8s.io", Resource: "mutatingadmissionpolicybindings"}, "codesphere-image-registry", errors.New("denied"))
		expectRegistryDelete(client, bindingKind, v1AdmissionAPI, cause)
		err := installer.EnsureRegistryPolicy(context.Background(), client, "ghcr.io", installer.NewMockClusterVersionClient(GinkgoT()))
		Expect(err).To(MatchError(ContainSubstring("failed to remove image registry policy binding")))
		Expect(errors.Is(err, cause)).To(BeTrue())
	})
	It("reports policy removal failures after removing the binding", func() {
		client := installer.NewMockRegistryPolicyClient(GinkgoT())
		expectRegistryDelete(client, bindingKind, v1AdmissionAPI, nil)

		cause := apierrors.NewServiceUnavailable("API restarting")
		expectRegistryDelete(client, policyKind, v1AdmissionAPI, cause)
		err := installer.EnsureRegistryPolicy(context.Background(), client, "ghcr.io", installer.NewMockClusterVersionClient(GinkgoT()))
		Expect(err).To(MatchError(ContainSubstring("failed to remove image registry policy: failed to delete admissionregistration.k8s.io/v1 MutatingAdmissionPolicy")))
		Expect(errors.Is(err, cause)).To(BeTrue())
	})
	It("reports when dry-run admission fails after removing the policy", func() {
		client := installer.NewMockRegistryPolicyClient(GinkgoT())
		expectRegistryDelete(client, bindingKind, v1AdmissionAPI, nil)
		expectRegistryDelete(client, policyKind, v1AdmissionAPI, registryResourceNotFound("mutatingadmissionpolicies"))

		cause := apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "probe", errors.New("probe forbidden"))
		client.EXPECT().Create(mock.Anything, mock.Anything, mock.Anything).Return(cause).Once()
		err := installer.EnsureRegistryPolicy(context.Background(), client, "ghcr.io", installer.NewMockClusterVersionClient(GinkgoT()))
		Expect(err).To(MatchError(ContainSubstring("policy removal")))
		Expect(errors.Is(err, cause)).To(BeTrue())
	})
	It("rewrites only Codesphere container and init container images, preserving tags and digests", func() {
		policy := installRegistryPolicy("registry.example.com:5000/team//mirror/")
		Expect(policy.Spec.MatchConstraints.ResourceRules[0].Operations).To(Equal([]admissionv1.OperationType{admissionv1.Create}))
		Expect(*policy.Spec.FailurePolicy).To(Equal(admissionv1.Fail))
		pod := &corev1.Pod{Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "codesphere", Image: "ghcr.io/codesphere-cloud/server:v1"},
				{Name: "other", Image: "ghcr.io/other/server:v1"},
				{Name: "similar", Image: "ghcr.io/codesphere-cloud-other/server:v1"},
				{Name: "mirrored", Image: "registry.example.com:5000/team/mirror/codesphere-cloud/server:v1"},
			},
			InitContainers: []corev1.Container{{Name: "init", Image: "ghcr.io/codesphere-cloud/init@sha256:abc"}},
		}}
		result := mutateRegistryPod(policy, pod)
		Expect(result.Spec.Containers[0].Image).To(Equal("registry.example.com:5000/team/mirror/codesphere-cloud/server:v1"))
		Expect(result.Spec.Containers[1:]).To(Equal(pod.Spec.Containers[1:]))
		Expect(result.Spec.InitContainers[0].Image).To(Equal("registry.example.com:5000/team/mirror/codesphere-cloud/init@sha256:abc"))
		Expect(mutateRegistryPod(policy, result)).To(Equal(result))
	})
	It("does not rewrite an overlapping destination twice", func() {
		registry := "ghcr.io/codesphere-cloud/mirror"
		policy := installRegistryPolicy(registry)
		source := "ghcr.io/codesphere-cloud/api:v1"
		destination, err := installer.PackageImageDestination(source, registry)
		Expect(err).NotTo(HaveOccurred())

		pod := &corev1.Pod{Spec: corev1.PodSpec{
			Containers:     []corev1.Container{{Name: "app", Image: source}, {Name: "mirrored", Image: destination}},
			InitContainers: []corev1.Container{{Name: "init", Image: source}},
		}}
		result := mutateRegistryPod(policy, pod)
		Expect(result.Spec.Containers[0].Image).To(Equal(destination))
		Expect(result.Spec.Containers[1].Image).To(Equal(destination))
		Expect(result.Spec.InitContainers[0].Image).To(Equal(destination))
		Expect(mutateRegistryPod(policy, result)).To(Equal(result))
	})

	It("handles pods without init containers", func() {
		policy := installRegistryPolicy("registry.example.com")
		result := mutateRegistryPod(policy, &corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "ghcr.io/codesphere-cloud/server:v1"}}}})
		Expect(result.Spec.Containers[0].Image).To(Equal("registry.example.com/codesphere-cloud/server:v1"))
	})
	It("rejects invalid registries before writing resources", func() {
		Expect(installer.EnsureRegistryPolicy(context.Background(), installer.NewMockRegistryPolicyClient(GinkgoT()), "https://registry.example.com", installer.NewMockClusterVersionClient(GinkgoT()))).To(MatchError(ContainSubstring("must not include a scheme")))
	})
	It("stops before creating a binding when policy installation fails", func() {
		client := installer.NewMockRegistryPolicyClient(GinkgoT())
		cause := errors.New("policy API unavailable")
		client.EXPECT().Apply(mock.Anything, admissionResource("MutatingAdmissionPolicy"), mock.Anything).Return(cause).Once()
		Expect(installer.EnsureRegistryPolicy(context.Background(), client, "registry.example.com", supportedCluster())).To(MatchError(ContainSubstring(cause.Error())))
	})
	It("reports binding installation failures", func() {
		client := installer.NewMockRegistryPolicyClient(GinkgoT())
		client.EXPECT().Apply(mock.Anything, admissionResource("MutatingAdmissionPolicy"), mock.Anything).Return(nil).Once()

		cause := errors.New("forbidden")
		client.EXPECT().Apply(mock.Anything, admissionResource("MutatingAdmissionPolicyBinding"), mock.Anything).Return(cause).Once()
		err := installer.EnsureRegistryPolicy(context.Background(), client, "registry.example.com", supportedCluster())
		Expect(err).To(MatchError(ContainSubstring("policy binding")))
		Expect(errors.Is(err, cause)).To(BeTrue())
	})
	It("reports dry-run admission failures before installing dependencies", func() {
		client := installer.NewMockRegistryPolicyClient(GinkgoT())
		client.EXPECT().Apply(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

		cause := apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "probe", errors.New("probe forbidden"))
		client.EXPECT().Create(mock.Anything, mock.Anything, mock.Anything).Return(cause).Once()
		err := installer.EnsureRegistryPolicy(context.Background(), client, "registry.example.com", supportedCluster())
		Expect(err).To(MatchError(ContainSubstring("policy readiness")))
		Expect(errors.Is(err, cause)).To(BeTrue())
	})
	It("retries rate-limited probes and retains the last error on cancellation", func() {
		expectTransientProbeRetry(apierrors.NewTooManyRequests("API rate limited", 1))
	})
	It("retries timed-out probes and retains the last error on cancellation", func() {
		expectTransientProbeRetry(apierrors.NewTimeoutError("API timed out", 1))
	})
	It("retries temporarily unavailable probes and retains the last error on cancellation", func() {
		expectTransientProbeRetry(apierrors.NewServiceUnavailable("API restarting"))
	})
	It("waits until init-container admission is active and respects cancellation", func() {
		client := installer.NewMockRegistryPolicyClient(GinkgoT())
		client.EXPECT().Apply(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		client.EXPECT().Create(mock.Anything, mock.Anything, mock.Anything).
			Run(func(_ context.Context, obj ctrlclient.Object, _ ...ctrlclient.CreateOption) {
				obj.(*corev1.Pod).Spec.Containers[0].Image = "registry.example.com/codesphere-cloud/registry-probe:admission"

				cancel()
			}).Return(nil).Once()
		err := installer.EnsureRegistryPolicy(ctx, client, "registry.example.com", supportedCluster())
		Expect(errors.Is(err, context.Canceled)).To(BeTrue())
	})
	It("waits until regular container admission is active and respects cancellation", func() {
		client := installer.NewMockRegistryPolicyClient(GinkgoT())
		client.EXPECT().Apply(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		client.EXPECT().Create(mock.Anything, mock.Anything, mock.Anything).
			Run(func(_ context.Context, obj ctrlclient.Object, _ ...ctrlclient.CreateOption) {
				obj.(*corev1.Pod).Spec.InitContainers[0].Image = "registry.example.com/codesphere-cloud/registry-probe:init"

				cancel()
			}).Return(nil).Once()
		err := installer.EnsureRegistryPolicy(ctx, client, "registry.example.com", supportedCluster())
		Expect(errors.Is(err, context.Canceled)).To(BeTrue())
	})
})

func mutateRegistryCR(policy *admissionv1.MutatingAdmissionPolicy, obj *unstructured.Unstructured, resource string) *unstructured.Unstructured {
	gvk := obj.GroupVersionKind()
	return mutateRegistryObject(policy, obj, gvk, gvk.GroupVersion().WithResource(resource)).(*unstructured.Unstructured)
}

var _ = Describe("Registry admission for CNPG resources", func() {
	It("rewrites CNPG desired images and preserves versions and other cluster settings", func() {
		policy := installRegistryPolicy("registry.example.com/team")
		Expect(policy.Spec.MatchConstraints.ResourceRules).To(HaveLen(2))
		Expect(policy.Spec.Mutations).To(HaveLen(2))
		rule := policy.Spec.MatchConstraints.ResourceRules[1]
		Expect(rule.APIGroups).To(Equal([]string{"postgresql.cnpg.io"}))
		Expect(rule.Resources).To(Equal([]string{"clusters"}))
		Expect(rule.Operations).To(Equal([]admissionv1.OperationType{admissionv1.Create, admissionv1.Update}))

		cluster := &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "postgresql.cnpg.io/v1", "kind": "Cluster",
			"metadata": map[string]any{"name": "openfga-pg"},
			"spec":     map[string]any{"imageName": "ghcr.io/codesphere-cloud/docker/postgresql:16@sha256:abc", "instances": int64(2)},
		}}
		result := mutateRegistryCR(policy, cluster, "clusters")
		Expect(result.Object["spec"]).To(Equal(map[string]any{"imageName": "registry.example.com/team/codesphere-cloud/docker/postgresql:16@sha256:abc", "instances": int64(2)}))
		Expect(mutateRegistryCR(policy, result, "clusters")).To(Equal(result))
	})

	It("leaves CNPG image defaults and unrelated registries unchanged", func() {
		policy := installRegistryPolicy("registry.example.com")
		cluster := &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "postgresql.cnpg.io/v1", "kind": "Cluster",
			"spec": map[string]any{"instances": int64(2)},
		}}
		Expect(mutateRegistryCR(policy, cluster, "clusters")).To(Equal(cluster))
		cluster.Object["spec"].(map[string]any)["imageName"] = "ghcr.io/other/postgresql:16"
		Expect(mutateRegistryCR(policy, cluster, "clusters")).To(Equal(cluster))
	})
})
