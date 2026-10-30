// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere_test

import (
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/cluster"
	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/codesphere"
	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/images"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

const imagePrefix = "ghcr.io/codesphere-cloud/codesphere-monorepo/"

func defaultTestInputs() codesphere.Inputs {
	return codesphere.Inputs{Images: defaultTestImages(), Gateways: defaultTestGateways()}
}

func defaultTestGateways() cluster.Gateways {
	return cluster.GatewayAPIs(files.ClusterConfig{})
}

func defaultTestImages() images.Images {
	return testImagesWith(nil)
}

func testImagesWith(extraImages map[string]string) images.Images {
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

// deployImage returns a deploy config image.
func deployImage(values codesphere.Codesphere, key string) codesphere.DeployImage {
	Expect(values.DeployConfig.Images).To(HaveKey(key))

	return values.DeployConfig.Images[key]
}
