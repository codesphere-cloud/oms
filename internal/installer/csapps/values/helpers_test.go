// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package values_test

import (
	"strings"

	. "github.com/onsi/gomega"
	"go.yaml.in/yaml/v3"

	"github.com/codesphere-cloud/oms/internal/installer/bom"
	csappsvalues "github.com/codesphere-cloud/oms/internal/installer/csapps/values"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

const imagePrefix = "ghcr.io/codesphere-cloud/codesphere-monorepo/"

func testBOM(extraImages map[string]string) *bom.Config {
	images := map[string]string{
		"auth-service":          imagePrefix + "auth-service:v1.2.3",
		"deployment-service":    imagePrefix + "deployment-service:v1.2.3",
		"error-page-server":     imagePrefix + "error-page-server:v1.2.3",
		"ide-frontend":          imagePrefix + "ide-frontend:v1.2.3",
		"ide-service":           imagePrefix + "ide-service:v1.2.3",
		"marketplace":           imagePrefix + "marketplace:v1.2.3",
		"payment-service":       imagePrefix + "payment-service:v1.2.3",
		"public-api-service":    imagePrefix + "public-api-service:v1.2.3",
		"team-service":          imagePrefix + "team-service:v1.2.3",
		"workspace-proxy":       imagePrefix + "workspace-proxy:v1.2.3",
		"workspace-service":     imagePrefix + "workspace-service:v1.2.3",
		"workspace-agent-24.04": imagePrefix + "workspace-agent-24.04:v1.2.3",
		"workspace-agent":       imagePrefix + "workspace-agent:v1.2.3",
		"workspace-server":      imagePrefix + "workspace-server:v1.2.3",
		"vpn":                   imagePrefix + "vpn:v1.2.3",
		"nginx":                 "ghcr.io/codesphere-cloud/docker/nginx:1.31.5",
		"frontendGatewayNginx":  "ghcr.io/codesphere-cloud/docker/nginx-gateway:1.31.5",
		"opensearch":            "ghcr.io/codesphere-cloud/docker/opensearch:3.8.0",
		"opensearchInit":        "ghcr.io/codesphere-cloud/docker/busybox:1.37.0",
		"kubectl":               "ghcr.io/codesphere-cloud/docker/kubectl:1.34.0",
		"observabilityOtel":     "ghcr.io/codesphere-cloud/docker/otel:0.160.0",
		"observabilityOtelInit": "ghcr.io/codesphere-cloud/docker/curl:8.20.0",
	}
	for name, image := range extraImages {
		images[name] = image
	}

	return &bom.Config{Components: map[string]bom.ComponentConfig{
		"codesphere": {ContainerImages: images},
	}}
}

func testConfig(yamlConfig string) *files.RootConfig {
	base := `
		dataCenter: {id: 1, name: main, city: Karlsruhe, countryCode: DE}
		postgres: {serverAddress: postgres.example.com}
		kubernetes: {managedByCodesphere: false, podCidr: 10.1.0.0/16, serviceCidr: 10.2.0.0/16}
		codesphere:
		  domain: codesphere.example.com
		  workspaceHostingBaseDomain: ws.example.com
		  publicIp: 203.0.113.10
		  customDomains: {cNameBaseDomain: custom.example.com}
		  plans:
		    hostingPlans:
		      8: {cpuTenth: 10, gpuParts: 0, memoryMb: 2048, storageMb: 20480, tempStorageMb: 1500}
		    workspacePlans: {}
	`
	config := files.NewRootConfig()
	Expect(config.Unmarshal([]byte(dedent(base)))).To(Succeed())
	Expect(yaml.Unmarshal([]byte(dedent(yamlConfig)), &config)).To(Succeed())

	return &config
}

func build(cfg *files.RootConfig, b *bom.Config) map[string]any {
	values, err := csappsvalues.Build(cfg, b)
	Expect(err).NotTo(HaveOccurred())

	return values
}

// at returns the value at a dot-separated path.
func at(values map[string]any, path string) any {
	var current any = values
	for _, key := range strings.Split(path, ".") {
		m, ok := current.(map[string]any)
		Expect(ok).To(BeTrue(), "%s is not a map at %s", path, key)
		current, ok = m[key]
		Expect(ok).To(BeTrue(), "%s is missing %s", path, key)
	}

	return current
}

// dedent removes the indentation all non-blank lines share, so tests can indent multi-line YAML
// like the code around it.
func dedent(s string) string {
	lines := strings.Split(strings.TrimPrefix(s, "\n"), "\n")
	if last := len(lines) - 1; strings.TrimSpace(lines[last]) == "" {
		lines[last] = ""
	}

	prefix, first := "", true

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		if first {
			prefix, first = indent, false
			continue
		}

		for !strings.HasPrefix(indent, prefix) {
			prefix = prefix[:len(prefix)-1]
		}
	}

	for i, line := range lines {
		lines[i] = strings.TrimPrefix(line, prefix)
	}

	return strings.Join(lines, "\n")
}
