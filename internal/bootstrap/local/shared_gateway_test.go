// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"testing"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwalpha "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func TestSharedGatewayCreatesAndUpdatesResources(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := gwv1.Install(scheme); err != nil {
		t.Fatal(err)
	}

	if err := gwalpha.Install(scheme); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	kube := fake.NewClientBuilder().WithScheme(scheme).Build()
	config := files.NewRootConfig()
	config.Codesphere.Domain = "cs.example.com"
	config.Codesphere.WorkspaceHostingBaseDomain = "ws.example.com"
	config.Codesphere.CustomDomains.CNameBaseDomain = "custom.example.com"

	b := &LocalBootstrapper{ctx: ctx, kubeClient: kube, Env: &CodesphereEnvironment{InstallConfig: &config}}
	for range 2 {
		if err := b.ExposeSharedGateway(); err != nil {
			t.Fatal(err)
		}
	}

	proxy := &unstructured.Unstructured{}
	proxy.SetGroupVersionKind(schema.GroupVersionKind{Group: "gateway.envoyproxy.io", Version: "v1alpha1", Kind: "EnvoyProxy"})

	if err := kube.Get(ctx, client.ObjectKey{Name: edgeName + "-config", Namespace: codesphereNamespace}, proxy); err != nil {
		t.Fatal(err)
	}

	hostNetwork, found, err := unstructured.NestedBool(proxy.Object, "spec", "provider", "kubernetes", "envoyDeployment", "patch", "value", "spec", "template", "spec", "hostNetwork")
	if err != nil || !found || !hostNetwork {
		t.Fatalf("edge Envoy must use host networking: %v, %v, %v", hostNetwork, found, err)
	}

	containers, found, err := unstructured.NestedSlice(proxy.Object, "spec", "provider", "kubernetes", "envoyDeployment", "patch", "value", "spec", "template", "spec", "containers")
	if err != nil || !found || len(containers) != 1 {
		t.Fatalf("unexpected edge Envoy deployment patch containers: %v, %v, %v", containers, found, err)
	}

	if _, found := containers[0].(map[string]any)["securityContext"]; found {
		t.Fatal("edge Envoy must not request port-binding capabilities")
	}

	portShift, _, err := unstructured.NestedBool(proxy.Object, "spec", "provider", "kubernetes", "useListenerPortAsContainerPort")
	if err != nil || !portShift {
		t.Fatalf("edge Envoy must bind listener ports directly: %v, %v", portShift, err)
	}

	serviceType, _, err := unstructured.NestedString(proxy.Object, "spec", "provider", "kubernetes", "envoyService", "type")
	if err != nil || serviceType != "ClusterIP" {
		t.Fatalf("edge Envoy service should be internal: %q, %v", serviceType, err)
	}

	gateway := &gwv1.Gateway{}
	if err := kube.Get(ctx, client.ObjectKey{Name: edgeName, Namespace: codesphereNamespace}, gateway); err != nil {
		t.Fatal(err)
	}

	if len(gateway.Spec.Listeners) != 3 || *gateway.Spec.Listeners[1].TLS.Mode != gwv1.TLSModePassthrough || gateway.Spec.Listeners[2].Port != sharedSSHPort {
		t.Fatalf("unexpected edge listeners: %+v", gateway.Spec.Listeners)
	}

	tls := &gwv1.TLSRoute{}
	if err := kube.Get(ctx, client.ObjectKey{Name: "codesphere-edge-platform-tls", Namespace: codesphereNamespace}, tls); err != nil {
		t.Fatal(err)
	}

	if len(tls.Spec.Hostnames) != 2 || tls.Spec.Rules[0].BackendRefs[0].Name != "gateway-controller" {
		t.Fatalf("unexpected platform TLS route: %+v", tls.Spec)
	}

	if err := kube.Get(ctx, client.ObjectKey{Name: "codesphere-edge-workspace-tls", Namespace: codesphereNamespace}, tls); err != nil {
		t.Fatal(err)
	}

	if len(tls.Spec.Hostnames) != 4 ||
		tls.Spec.Hostnames[0] != "ws.example.com" ||
		tls.Spec.Hostnames[1] != "*.ws.example.com" ||
		tls.Spec.Hostnames[2] != "custom.example.com" ||
		tls.Spec.Hostnames[3] != "*.custom.example.com" ||
		tls.Spec.Rules[0].BackendRefs[0].Name != "public-gateway-controller" {
		t.Fatalf("unexpected workspace TLS route: %+v", tls.Spec)
	}

	http := &gwv1.HTTPRoute{}
	if err := kube.Get(ctx, client.ObjectKey{Name: "codesphere-edge-workspace-http", Namespace: codesphereNamespace}, http); err != nil {
		t.Fatal(err)
	}

	if len(http.Spec.Hostnames) != 0 || http.Spec.Rules[0].BackendRefs[0].Name != "public-gateway-controller" {
		t.Fatalf("unexpected catch-all HTTP route: %+v", http.Spec)
	}

	ssh := &gwalpha.TCPRoute{}
	if err := kube.Get(ctx, client.ObjectKey{Name: "codesphere-edge-ssh", Namespace: codesphereNamespace}, ssh); err != nil {
		t.Fatal(err)
	}

	if ssh.Spec.Rules[0].BackendRefs[0].Name != "ssh-workspace-proxy" {
		t.Fatalf("unexpected SSH route: %+v", ssh.Spec)
	}
}

func TestConfigureSharedExposure(t *testing.T) {
	config := files.NewRootConfig()
	configureSharedExposure(&config)

	if config.Cluster.Gateway.ServiceType != "ClusterIP" || config.Cluster.PublicGateway.ServiceType != "ClusterIP" {
		t.Fatal("downstream gateways should be ClusterIP")
	}

	apps := config.PcApps["applications"].(map[string]any)
	ssh := apps["ssh-workspace-proxy"].(map[string]any)

	service := ssh["valuesObject"].(map[string]any)["service"].(map[string]any)
	if service["type"] != "ClusterIP" || service["port"] != 22 {
		t.Fatalf("unexpected SSH backend: %#v", service)
	}
}

func TestWorkspaceTLSRouteUsesWorkspaceDomainOnce(t *testing.T) {
	spec := edgeTLSRouteSpec("codesphere-edge-workspace-tls", "cs.example.com", "ws.example.com", "ws.example.com")
	if len(spec.Hostnames) != 2 || spec.Hostnames[0] != "ws.example.com" || spec.Hostnames[1] != "*.ws.example.com" {
		t.Fatalf("unexpected workspace TLS hostnames: %v", spec.Hostnames)
	}
}
