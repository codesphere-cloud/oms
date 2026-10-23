// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere_test

import (
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/bom"
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
	conf     files.CodesphereConfig
	images   images.Images
	registry *files.RegistryConfig
}

func (in input) values() (codesphere.Codesphere, error) {
	imgs := in.images
	if imgs == nil {
		imgs = testImages(nil)
	}

	// Tests check the error Values returns.
	return codesphere.Values(in.conf, imgs, in.registry) //nolint:wrapcheck
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
