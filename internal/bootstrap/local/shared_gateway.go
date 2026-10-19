// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package local

import (
	"fmt"
	"strings"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwalpha "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

const (
	sharedSSHPort = 2222
	edgeName      = "codesphere-edge"
)

// Keep the two existing GatewayClasses and EnvoyProxy configurations intact.
// Only their generated Services become internal backends of the edge Gateway.
func configureSharedExposure(config *files.RootConfig) {
	for _, gateway := range []*files.GatewayConfig{&config.Cluster.Gateway, &config.Cluster.PublicGateway} {
		gateway.ServiceType = "ClusterIP"
		gateway.Annotations = nil
		gateway.IPAddresses = nil
		gateway.Override = util.DeepMergeMaps(gateway.Override, map[string]any{
			"gateway": map[string]any{
				"proxy": map[string]any{
					"replicas": 1,
					"service":  map[string]any{"externalTrafficPolicy": nil},
				},
			},
		})
	}

	config.PcApps = util.DeepMergeMaps(config.PcApps, map[string]any{
		"applications": map[string]any{
			"ssh-workspace-proxy": map[string]any{
				"enabled": true,
				"valuesObject": map[string]any{
					"replicas": 1,
					"service":  map[string]any{"enabled": true, "type": "ClusterIP", "port": 22},
				},
			},
		},
	})
}

// ExposeSharedGateway creates an Envoy data plane that binds the host's
// 80, 443, and 2222 ports. TLS is forwarded to the existing Envoys unchanged.
func (b *LocalBootstrapper) ExposeSharedGateway() error {
	domain := b.Env.InstallConfig.Codesphere.Domain
	if domain == "" || strings.ContainsAny(domain, " \t\n\r\"'/:*") {
		return fmt.Errorf("invalid Codesphere domain for shared gateway: %q", domain)
	}
	workspaceDomain := b.Env.InstallConfig.Codesphere.WorkspaceHostingBaseDomain
	if workspaceDomain == "" || strings.ContainsAny(workspaceDomain, " \t\n\r\"'/:*") {
		return fmt.Errorf("invalid workspace hosting base domain for shared gateway: %q", workspaceDomain)
	}
	customCNameDomain := b.Env.InstallConfig.Codesphere.CustomDomains.CNameBaseDomain
	if customCNameDomain != "" && strings.ContainsAny(customCNameDomain, " \t\n\r\"'/:*") {
		return fmt.Errorf("invalid custom CNAME base domain for shared gateway: %q", customCNameDomain)
	}

	if err := b.createEdgeEnvoyProxy(); err != nil {
		return err
	}

	if err := b.createEdgeGatewayClass(); err != nil {
		return err
	}

	if err := b.createEdgeGateway(); err != nil {
		return err
	}

	for _, name := range []string{"codesphere-edge-platform-http", "codesphere-edge-workspace-http"} {
		route := &gwv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: codesphereNamespace}}
		if _, err := controllerutil.CreateOrUpdate(b.ctx, b.kubeClient, route, func() error {
			route.Spec = edgeHTTPRouteSpec(name, domain)
			return nil
		}); err != nil {
			return fmt.Errorf("create HTTPRoute %s: %w", name, err)
		}
	}

	for _, name := range []string{"codesphere-edge-platform-tls", "codesphere-edge-workspace-tls"} {
		route := &gwv1.TLSRoute{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: codesphereNamespace}}
		if _, err := controllerutil.CreateOrUpdate(b.ctx, b.kubeClient, route, func() error {
			route.Spec = edgeTLSRouteSpec(name, domain, workspaceDomain, customCNameDomain)
			return nil
		}); err != nil {
			return fmt.Errorf("create TLSRoute %s: %w", name, err)
		}
	}

	ssh := &gwalpha.TCPRoute{ObjectMeta: metav1.ObjectMeta{Name: "codesphere-edge-ssh", Namespace: codesphereNamespace}}
	if _, err := controllerutil.CreateOrUpdate(b.ctx, b.kubeClient, ssh, func() error {
		ssh.Spec = gwalpha.TCPRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{ParentRefs: []gwv1.ParentReference{edgeParent("ssh")}},
			Rules:           []gwalpha.TCPRouteRule{{BackendRefs: []gwv1.BackendRef{edgeBackend("ssh-workspace-proxy", 22)}}},
		}

		return nil
	}); err != nil {
		return fmt.Errorf("create TCPRoute %s: %w", ssh.Name, err)
	}

	return nil
}

func (b *LocalBootstrapper) createEdgeEnvoyProxy() error {
	proxy := &unstructured.Unstructured{}
	proxy.SetGroupVersionKind(schema.GroupVersionKind{Group: "gateway.envoyproxy.io", Version: "v1alpha1", Kind: "EnvoyProxy"})
	proxy.SetName(edgeName + "-config")
	proxy.SetNamespace(codesphereNamespace)

	_, err := controllerutil.CreateOrUpdate(b.ctx, b.kubeClient, proxy, func() error {
		return unstructured.SetNestedMap(proxy.Object, edgeEnvoyProxySpec(), "spec")
	})
	if err != nil {
		return fmt.Errorf("create edge EnvoyProxy: %w", err)
	}

	return nil
}

// Envoy Gateway normally shifts privileged listener ports by 10000. On a
// host-network pod the listener must instead bind the real port directly.
func edgeEnvoyProxySpec() map[string]any {
	return map[string]any{
		"mergeGateways": false,
		"provider": map[string]any{
			"type": "Kubernetes",
			"kubernetes": map[string]any{
				"useListenerPortAsContainerPort": true,
				"envoyService":                   map[string]any{"name": edgeName, "type": "ClusterIP"},
				"envoyDeployment": map[string]any{
					"name": edgeName, "replicas": int64(1),
					"pod": map[string]any{"imagePullSecrets": []any{map[string]any{"name": "docker-regcred"}}},
					"container": map[string]any{
						"image": "ghcr.io/codesphere-cloud/docker/envoyproxy/envoy:distroless-v1.37.1@sha256:db9573e5ea4fc00fed0fa22cd1bf57eaba8f3bd5dd085ecb8fe37dedfe11fa51",
					},
					"patch": map[string]any{
						"type": "StrategicMerge",
						"value": map[string]any{"spec": map[string]any{"template": map[string]any{"spec": map[string]any{
							"hostNetwork": true,
							"dnsPolicy":   "ClusterFirstWithHostNet",
							"containers": []any{map[string]any{
								"name":      "envoy",
								"lifecycle": map[string]any{"preStop": map[string]any{"httpGet": map[string]any{"host": "127.0.0.1"}}},
							}},
						}}}},
					},
				},
			},
		},
	}
}

func (b *LocalBootstrapper) createEdgeGatewayClass() error {
	class := &gwv1.GatewayClass{ObjectMeta: metav1.ObjectMeta{Name: edgeName}}

	_, err := controllerutil.CreateOrUpdate(b.ctx, b.kubeClient, class, func() error {
		ns := gwv1.Namespace(codesphereNamespace)
		class.Spec = gwv1.GatewayClassSpec{
			ControllerName: "gateway.envoyproxy.io/gatewayclass-controller",
			ParametersRef: &gwv1.ParametersReference{
				Group: "gateway.envoyproxy.io", Kind: "EnvoyProxy", Name: edgeName + "-config", Namespace: &ns,
			},
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("create edge GatewayClass: %w", err)
	}

	return nil
}

func (b *LocalBootstrapper) createEdgeGateway() error {
	gateway := &gwv1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: edgeName, Namespace: codesphereNamespace}}

	_, err := controllerutil.CreateOrUpdate(b.ctx, b.kubeClient, gateway, func() error {
		passthrough := gwv1.TLSModePassthrough
		gateway.Spec = gwv1.GatewaySpec{
			GatewayClassName: edgeName,
			Listeners: []gwv1.Listener{
				{Name: "http", Protocol: gwv1.HTTPProtocolType, Port: 80},
				{Name: "tls", Protocol: gwv1.TLSProtocolType, Port: 443, TLS: &gwv1.ListenerTLSConfig{Mode: &passthrough}},
				{Name: "ssh", Protocol: gwv1.TCPProtocolType, Port: sharedSSHPort},
			},
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("create edge Gateway: %w", err)
	}

	return nil
}

func edgeParent(section gwv1.SectionName) gwv1.ParentReference {
	return gwv1.ParentReference{Name: edgeName, SectionName: &section}
}

func edgeBackend(name string, port gwv1.PortNumber) gwv1.BackendRef {
	return gwv1.BackendRef{BackendObjectReference: gwv1.BackendObjectReference{Name: gwv1.ObjectName(name), Port: &port}}
}

func edgeHTTPRouteSpec(name, domain string) gwv1.HTTPRouteSpec {
	backend := "public-gateway-controller"

	var hosts []gwv1.Hostname

	if name == "codesphere-edge-platform-http" {
		backend = "gateway-controller"
		hosts = []gwv1.Hostname{gwv1.Hostname(domain), gwv1.Hostname("*." + domain)}
	}

	return gwv1.HTTPRouteSpec{
		CommonRouteSpec: gwv1.CommonRouteSpec{ParentRefs: []gwv1.ParentReference{edgeParent("http")}},
		Hostnames:       hosts,
		Rules:           []gwv1.HTTPRouteRule{{BackendRefs: []gwv1.HTTPBackendRef{{BackendRef: edgeBackend(backend, 80)}}}},
	}
}

func edgeTLSRouteSpec(name, domain, workspaceDomain, customCNameDomain string) gwv1.TLSRouteSpec {
	backend := "public-gateway-controller"

	hosts := []gwv1.Hostname{gwv1.Hostname(workspaceDomain), gwv1.Hostname("*." + workspaceDomain)}
	if customCNameDomain != "" && customCNameDomain != workspaceDomain {
		hosts = append(hosts, gwv1.Hostname(customCNameDomain), gwv1.Hostname("*."+customCNameDomain))
	}

	if name == "codesphere-edge-platform-tls" {
		backend = "gateway-controller"
		hosts = []gwv1.Hostname{gwv1.Hostname(domain), gwv1.Hostname("*." + domain)}
	}

	return gwv1.TLSRouteSpec{
		CommonRouteSpec: gwv1.CommonRouteSpec{ParentRefs: []gwv1.ParentReference{edgeParent("tls")}},
		Hostnames:       hosts,
		Rules:           []gwv1.TLSRouteRule{{BackendRefs: []gwv1.BackendRef{edgeBackend(backend, 443)}}},
	}
}
