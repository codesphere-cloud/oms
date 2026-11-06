// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps_test

import (
	"github.com/codesphere-cloud/oms/internal/installer/argocd/pcapps"
	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.yaml.in/yaml/v3"
	"k8s.io/utils/ptr"
)

var _ = Describe("GatewayPcAppsValues", func() {
	var (
		config     files.RootConfig
		packageBOM *bom.Config
	)

	BeforeEach(func() {
		config = files.RootConfig{Codesphere: files.CodesphereConfig{Domain: "codesphere.example.test", WorkspaceHostingBaseDomain: "ws.example.test"}}
		packageBOM = &bom.Config{Components: map[string]bom.ComponentConfig{
			"gateway":        {ContainerImages: map[string]string{"envoyProxy": "unused.example.test/envoy:v1"}},
			"public-gateway": {},
		}}
	})

	chartValues := func(name string) files.ChartValues {
		GinkgoHelper()

		values := pcapps.GatewayPcAppsValues(&config, packageBOM)

		application := values["applications"].(files.ChartValues)[name].(files.ChartValues)
		Expect(application["enabled"]).To(BeTrue())

		return application["valuesObject"].(files.ChartValues)
	}

	It("leaves both legacy Helm installations untouched without requiring images", func() {
		packageBOM.Components["gateway"] = bom.ComponentConfig{Files: map[string]bom.FileRef{"chart": {}}}
		packageBOM.Components["public-gateway"] = bom.ComponentConfig{Files: map[string]bom.FileRef{"chart": {}}}
		values := pcapps.GatewayPcAppsValues(&config, packageBOM)
		Expect(values).To(BeNil())
	})

	It("does not infer a handover without a package BOM", func() {
		values := pcapps.GatewayPcAppsValues(&config, nil)
		Expect(values).To(BeNil())
	})

	It("hands gateways over independently for a mixed package", func() {
		packageBOM.Components["gateway"] = bom.ComponentConfig{Files: map[string]bom.FileRef{"chart": {}}}
		values := pcapps.GatewayPcAppsValues(&config, packageBOM)
		Expect(values["applications"]).To(HaveKey("public-gateway"))
		Expect(values["applications"]).NotTo(HaveKey("gateway"))
	})

	It("preserves Gateway API defaults and the private-cloud resource profile", func() {
		values := chartValues("gateway")
		gateway := values["gateway"].(files.ChartValues)
		Expect(gateway["enabled"]).To(BeTrue())
		Expect(gateway["overrideIngressNginxService"]).To(BeTrue())
		Expect(gateway["proxy"].(files.ChartValues)["imagePullSecrets"]).To(Equal([]any{files.ChartValues{"name": "docker-regcred"}}))

		controller := values["ingress-nginx"].(files.ChartValues)["controller"].(files.ChartValues)
		Expect(controller["replicaCount"]).To(Equal(0))
		Expect(controller["resources"]).To(Equal(files.ChartValues{
			"requests": files.ChartValues{"cpu": "400m", "memory": "1Gi"},
			"limits":   files.ChartValues{"cpu": "2", "memory": "3Gi"},
		}))
		Expect(controller["config"].(files.ChartValues)["worker-processes"]).To(Equal("2"))
	})

	It("keeps nginx in charge while Gateway API is staged", func() {
		config.Cluster.Gateway.GatewayAPI = &files.GatewayAPIConfig{Enabled: true, Use: ptr.To(false)}
		config.Cluster.Gateway.ServiceType = "NodePort"
		values := chartValues("gateway")
		gateway := values["gateway"].(files.ChartValues)
		Expect(gateway["enabled"]).To(BeTrue())
		Expect(gateway["overrideIngressNginxService"]).To(BeFalse())
		Expect(gateway["proxy"].(files.ChartValues)["service"]).To(Equal(files.ChartValues{"type": "ClusterIP"}))

		controller := values["ingress-nginx"].(files.ChartValues)["controller"].(files.ChartValues)
		Expect(controller["replicaCount"]).To(Equal(3))
		Expect(controller["service"].(files.ChartValues)["enabled"]).To(BeTrue())
	})

	It("preserves explicit Gateway API disablement and public nginx replicas", func() {
		config.Cluster.PublicGateway.GatewayAPI = &files.GatewayAPIConfig{Enabled: false, Use: ptr.To(false)}
		values := chartValues("public-gateway")
		Expect(values["gateway"].(files.ChartValues)["enabled"]).To(BeFalse())
		Expect(values["ingress-nginx"].(files.ChartValues)["controller"].(files.ChartValues)["replicaCount"]).To(Equal(2))
	})

	It("preserves service addresses, annotations, and node selection", func() {
		config.Cluster.Gateway = files.GatewayConfig{
			ServiceType: "LoadBalancer", IPAddresses: []string{"192.0.2.10"},
			Annotations:  map[string]string{"loadbalancer.example.test/id": "gateway"},
			NodeSelector: map[string]string{"role": "gateway"},
		}
		values := chartValues("gateway")
		proxy := values["gateway"].(files.ChartValues)["proxy"].(files.ChartValues)
		Expect(proxy["externalIPs"]).To(Equal([]string{"192.0.2.10"}))
		Expect(proxy["service"]).To(Equal(files.ChartValues{"type": "LoadBalancer", "annotations": config.Cluster.Gateway.Annotations}))

		controller := values["ingress-nginx"].(files.ChartValues)["controller"].(files.ChartValues)
		Expect(controller["nodeSelector"]).To(Equal(config.Cluster.Gateway.NodeSelector))
		Expect(controller["service"].(files.ChartValues)["externalIPs"]).To(Equal(config.Cluster.Gateway.IPAddresses))
	})

	It("leaves image selection to charts and registry rewriting to Pod admission", func() {
		config.Registry = &files.RegistryConfig{Server: "mirror.example.test:5000/prefix", ReplaceImagesInBom: true}
		values := chartValues("gateway")
		proxy := values["gateway"].(files.ChartValues)["proxy"].(files.ChartValues)
		Expect(proxy).NotTo(HaveKey("image"))
		Expect(proxy["imagePullSecrets"]).To(Equal([]any{files.ChartValues{"name": "docker-regcred"}}))

		controller := values["ingress-nginx"].(files.ChartValues)["controller"].(files.ChartValues)
		Expect(controller).NotTo(HaveKey("image"))
	})

	It("uses public pull credentials without explicit registry configuration", func() {
		values := chartValues("public-gateway")
		Expect(values["gateway"].(files.ChartValues)["proxy"].(files.ChartValues)).NotTo(HaveKey("image"))
		Expect(values["ingress-nginx"].(files.ChartValues)["controller"].(files.ChartValues)).NotTo(HaveKey("admissionWebhooks"))
		Expect(values["nginx"].(files.ChartValues)["image"]).To(Equal(files.ChartValues{"pullSecrets": []string{"docker-regcred"}}))
	})

	It("keeps public nginx chart defaults separate from internal gateway overrides", func() {
		values := chartValues("public-gateway")
		Expect(values).NotTo(HaveKey("global"))
		ingress := values["ingress-nginx"].(files.ChartValues)
		Expect(ingress["imagePullSecrets"]).To(Equal([]any{files.ChartValues{"name": "docker-regcred"}}))
		controller := ingress["controller"].(files.ChartValues)
		Expect(controller).NotTo(HaveKey("resources"))
		Expect(controller["config"].(files.ChartValues)).NotTo(HaveKey("worker-processes"))
	})

	It("preserves public gateway domains, authentication, issuer, and proxy environment", func() {
		config.Codesphere.CertIssuer = &files.CertIssuerConfig{Type: files.CertIssuerTypeACME}
		values := chartValues("public-gateway")
		host := values["workspacesDevGateway"].(files.ChartValues)["host"].(files.ChartValues)
		Expect(host["name"]).To(Equal("*.ws.example.test"))

		nginx := values["nginx"].(files.ChartValues)
		ingress := nginx["ingress"].(files.ChartValues)
		Expect(ingress["issuer"]).To(Equal("acme"))
		Expect(ingress["hostname"]).To(Equal("*.ws.example.test"))
		Expect(ingress["annotations"].(files.ChartValues)["nginx.ingress.kubernetes.io/auth-signin"]).To(Equal("https://codesphere.example.test/ide/signin?devDomain=$scheme://$host$request_uri"))
		Expect(nginx["extraEnvVars"]).To(ContainElement(files.ChartValues{"name": "DOMAIN_NAME", "value": `ws\\.example\\.test`}))
		Expect(nginx["extraEnvVars"]).To(ContainElement(files.ChartValues{"name": "CPU_LIMIT", "valueFrom": files.ChartValues{"resourceFieldRef": files.ChartValues{"resource": "limits.cpu", "divisor": "1"}}}))
		Expect(nginx["resources"].(files.ChartValues)["requests"]).To(Equal(files.ChartValues{"cpu": "400m", "memory": "256Mi"}))
		Expect(values["ingress-nginx"].(files.ChartValues)["controller"].(files.ChartValues)["replicaCount"]).To(Equal(1))
	})

	It("preserves the legacy custom workspace certificate value", func() {
		config.Codesphere.WorkspaceHostingCustomCertificate = &files.CustomCertificate{Cert: "certificate", Key: "private-key"}
		values := chartValues("public-gateway")
		Expect(values["nginx"].(files.ChartValues)["certificate"]).To(Equal(files.ChartValues{"cert": "certificate", "key": "private-key"}))
	})

	It("lets component overrides and explicit pc-apps configuration take precedence", func() {
		config.Cluster.Gateway.Override = files.ChartOverride{"gateway": files.ChartValues{"proxy": files.ChartValues{"replicas": 7}}, "ingress-nginx": files.ChartValues{"controller": files.ChartValues{"replicaCount": 4}}}
		values := pcapps.GatewayPcAppsValues(&config, packageBOM)

		gateway := values["applications"].(files.ChartValues)["gateway"].(files.ChartValues)["valuesObject"].(files.ChartValues)
		Expect(gateway["gateway"].(files.ChartValues)["proxy"].(files.ChartValues)["replicas"]).To(Equal(7))
		Expect(gateway["ingress-nginx"].(files.ChartValues)["controller"].(files.ChartValues)["replicaCount"]).To(Equal(4))

		merged := util.DeepMergeMaps(values, files.ChartValues{"applications": files.ChartValues{"gateway": files.ChartValues{"enabled": false}}})
		Expect(merged["applications"].(files.ChartValues)["gateway"].(files.ChartValues)["enabled"]).To(BeFalse())
	})

	It("preserves an explicit image override", func() {
		config.Cluster.Gateway.Override = files.ChartOverride{"gateway": files.ChartValues{"proxy": files.ChartValues{"image": "custom.example.test/envoy:v2"}}}
		values := chartValues("gateway")
		Expect(values["gateway"].(files.ChartValues)["proxy"].(files.ChartValues)["image"]).To(Equal("custom.example.test/envoy:v2"))
	})

	It("retains node selectors and workspace certificates when loading and saving install config", func() {
		input := "cluster:\n  gateway:\n    nodeSelector:\n      role: gateway\ncodesphere:\n  workspaceHostingCustomCertificate:\n    cert: certificate\n    key: private-key\n"
		Expect(yaml.Unmarshal([]byte(input), &config)).To(Succeed())
		Expect(config.Cluster.Gateway.NodeSelector).To(Equal(map[string]string{"role": "gateway"}))
		Expect(config.Codesphere.WorkspaceHostingCustomCertificate).To(Equal(&files.CustomCertificate{Cert: "certificate", Key: "private-key"}))
		encoded, err := yaml.Marshal(config)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(encoded)).To(ContainSubstring("workspaceHostingCustomCertificate:"))
		Expect(string(encoded)).To(ContainSubstring("nodeSelector:"))
	})
})
