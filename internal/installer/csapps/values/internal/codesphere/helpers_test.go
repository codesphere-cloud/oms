// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere_test

import (
	"strings"

	. "github.com/onsi/gomega"
	"go.yaml.in/yaml/v3"

	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/cluster"
	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/codesphere"
	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/images"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

const imagePrefix = "ghcr.io/codesphere-cloud/codesphere-monorepo/"

func testImages(extraImages map[string]string) images.Images {
	bomImages := map[string]string{
		"workspace-agent-24.04": imagePrefix + "workspace-agent-24.04:v1.2.3",
		"workspace-agent":       imagePrefix + "workspace-agent:v1.2.3",
		"workspace-server":      imagePrefix + "workspace-server:v1.2.3",
		"vpn":                   imagePrefix + "vpn:v1.2.3",
		"frontendGatewayNginx":  "ghcr.io/codesphere-cloud/docker/nginx-gateway:1.31.5",
	}
	for name, image := range extraImages {
		bomImages[name] = image
	}

	b := &bom.Config{Components: map[string]bom.ComponentConfig{
		"codesphere": {ContainerImages: bomImages},
	}}

	imgs, err := images.New(b)
	Expect(err).NotTo(HaveOccurred())

	return imgs
}

// input are the arguments of codesphere.Values, with defaults for everything a test does not set.
type input struct {
	conf     string
	images   images.Images
	gateways *cluster.Gateways
	metalLB  *files.MetalLBConfig
	registry *files.RegistryConfig
}

func (in input) values() (codesphere.Codesphere, error) {
	conf := files.CodesphereConfig{
		Domain:        "codesphere.example.com",
		CustomDomains: files.CustomDomainsConfig{CNameBaseDomain: "custom.example.com"},
		Plans: files.PlansConfig{HostingPlans: map[int]files.HostingPlan{
			8: {CPUTenth: 10, MemoryMb: 2048, StorageMb: 20480, TempStorageMb: 1500},
		}},
	}
	Expect(yaml.Unmarshal([]byte(dedent(in.conf)), &conf)).To(Succeed())

	imgs := in.images
	if imgs == nil {
		imgs = testImages(nil)
	}

	gateways := cluster.GatewayAPIs(files.ClusterConfig{})
	if in.gateways != nil {
		gateways = *in.gateways
	}

	// Tests check the error Values returns.
	return codesphere.Values(conf, imgs, gateways, in.metalLB, in.registry) //nolint:wrapcheck
}

func (in input) mustValues() codesphere.Codesphere {
	values, err := in.values()
	Expect(err).NotTo(HaveOccurred())

	return values
}

// deployImage returns a deploy config image.
func deployImage(values codesphere.Codesphere, key string) codesphere.DeployImage {
	Expect(values.DeployConfig.Images).To(HaveKey(key))

	return values.DeployConfig.Images[key]
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
