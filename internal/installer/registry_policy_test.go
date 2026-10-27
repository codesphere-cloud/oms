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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
	expression := policy.Spec.Mutations[0].JSONPatch.Expression
	for _, variable := range policy.Spec.Variables {
		expression = strings.ReplaceAll(expression, "variables."+variable.Name, variable.Expression)
	}

	compiler, err := plugincel.NewCompositedCompiler(environment.MustBaseEnvSet(environment.DefaultCompatibilityVersion()))
	Expect(err).NotTo(HaveOccurred())

	evaluator := compiler.CompileMutatingEvaluator(&patch.JSONPatchCondition{Expression: expression},
		plugincel.OptionalVariableDeclarations{HasPatchTypes: true}, environment.StoredExpressions)
	scheme := runtime.NewScheme()
	Expect(corev1.AddToScheme(scheme)).To(Succeed())

	gvk := corev1.SchemeGroupVersion.WithKind("Pod")
	gvr := corev1.SchemeGroupVersion.WithResource("pods")
	attrs := admission.NewAttributesRecord(pod, nil, gvk, "default", "test", gvr, "", admission.Create, &metav1.CreateOptions{}, false, nil)
	result, err := patch.NewJSONPatcher(evaluator).Patch(context.Background(), patch.Request{
		MatchedResource:     gvr,
		VersionedAttributes: &admission.VersionedAttributes{Attributes: attrs, VersionedKind: gvk, VersionedObject: pod},
		ObjectInterfaces:    admission.NewObjectInterfacesFromScheme(scheme),
	}, celconfig.RuntimeCELCostBudget)
	Expect(err).NotTo(HaveOccurred())

	return result.(*corev1.Pod)
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
		client.EXPECT().Apply(mock.Anything, admissionResourceVersion("MutatingAdmissionPolicy", "admissionregistration.k8s.io/v1beta1"), mock.Anything).Return(nil).Once()
		client.EXPECT().Apply(mock.Anything, admissionResourceVersion("MutatingAdmissionPolicyBinding", "admissionregistration.k8s.io/v1beta1"), mock.Anything).Return(nil).Once()
		expectRegistryProbe(client, "registry.example.com")
		Expect(installer.EnsureRegistryPolicy(context.Background(), client, "registry.example.com", versions)).To(Succeed())
	})
	It("accepts Kubernetes 1.34 distributions with vendor version suffixes", func() {
		versions := installer.NewMockClusterVersionClient(GinkgoT())
		versions.EXPECT().ServerVersion().Return(&versioninfo.Info{GitVersion: "v1.34.0-eks-example"}, nil).Once()

		client := installer.NewMockRegistryPolicyClient(GinkgoT())
		client.EXPECT().Apply(mock.Anything, admissionResourceVersion("MutatingAdmissionPolicy", "admissionregistration.k8s.io/v1beta1"), mock.Anything).Return(nil).Once()
		client.EXPECT().Apply(mock.Anything, admissionResourceVersion("MutatingAdmissionPolicyBinding", "admissionregistration.k8s.io/v1beta1"), mock.Anything).Return(nil).Once()
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

	It("does not contact Kubernetes when the registry is unset", func() {
		Expect(installer.EnsureRegistryPolicy(context.Background(), installer.NewMockRegistryPolicyClient(GinkgoT()), "", installer.NewMockClusterVersionClient(GinkgoT()))).To(Succeed())
	})
	It("does not contact Kubernetes for the default registry", func() {
		Expect(installer.EnsureRegistryPolicy(context.Background(), installer.NewMockRegistryPolicyClient(GinkgoT()), "ghcr.io/", installer.NewMockClusterVersionClient(GinkgoT()))).To(Succeed())
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

		cause := errors.New("probe forbidden")
		client.EXPECT().Create(mock.Anything, mock.Anything, mock.Anything).Return(cause).Once()
		err := installer.EnsureRegistryPolicy(context.Background(), client, "registry.example.com", supportedCluster())
		Expect(err).To(MatchError(ContainSubstring("policy readiness")))
		Expect(errors.Is(err, cause)).To(BeTrue())
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
