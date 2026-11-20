// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/argocd/pcapps"
	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// bomWith is a package BOM with the given container images of one component.
func bomWith(component string, images map[string]string) *bom.Config {
	return &bom.Config{Components: map[string]bom.ComponentConfig{component: {ContainerImages: images}}}
}

// installerChartValues renders the values of one application of the installer charts.
func installerChartValues(config *files.RootConfig, packageBom *bom.Config, app string) (files.ChartValues, error) {
	values, err := pcapps.InstallerChartsPcAppsValues(config, nil, packageBom)
	if err != nil {
		return nil, fmt.Errorf("rendering installer charts: %w", err)
	}

	applications, _ := values["applications"].(files.ChartValues)
	application, _ := applications[app].(files.ChartValues)

	return application, nil
}

var _ = Describe("images of the package BOM", func() {
	const operatorImage = "ghcr.io/codesphere-cloud/docker/ghcr.io/cloudnative-pg/cloudnative-pg:1.30.0"

	operatorBom := func(image string) *bom.Config {
		return bomWith("postgres-operator", map[string]string{"cloudnativePg": image})
	}
	withRegistry := func(server string, replace bool) *files.RootConfig {
		return &files.RootConfig{Registry: &files.RegistryConfig{Server: server, ReplaceImagesInBom: replace}}
	}
	repositoryOf := func(config *files.RootConfig, packageBom *bom.Config) any {
		application, err := installerChartValues(config, packageBom, pcapps.PostgresOperator)
		Expect(err).NotTo(HaveOccurred())

		chart := application["valuesObject"].(files.ChartValues)["cloudnative-pg"].(files.ChartValues)

		return chart["image"].(files.ChartValues)["repository"]
	}

	It("keeps the image of the BOM without a registry", func() {
		Expect(repositoryOf(&files.RootConfig{}, operatorBom(operatorImage))).To(Equal("ghcr.io/codesphere-cloud/docker/ghcr.io/cloudnative-pg/cloudnative-pg"))
	})

	It("keeps the image of the BOM when the registry does not replace the images", func() {
		Expect(repositoryOf(withRegistry("registry.example.com/mirror", false), operatorBom(operatorImage))).To(Equal("ghcr.io/codesphere-cloud/docker/ghcr.io/cloudnative-pg/cloudnative-pg"))
	})

	It("moves the image to where oms copy package puts it when the registry replaces the images", func() {
		Expect(repositoryOf(withRegistry("registry.example.com/mirror", true), operatorBom(operatorImage))).
			To(Equal("registry.example.com/mirror/codesphere-cloud/docker/ghcr.io/cloudnative-pg/cloudnative-pg"))
	})

	It("leaves the chart defaults alone for a package whose BOM has no such component", func() {
		application, err := installerChartValues(&files.RootConfig{}, bomWith("other", map[string]string{"x": "y:1"}), pcapps.PostgresOperator)
		Expect(err).NotTo(HaveOccurred())

		Expect(application["valuesObject"].(files.ChartValues)["cloudnative-pg"]).ToNot(HaveKey("image"))
	})

	It("fails on an image without a tag instead of rendering a chart that pulls latest", func() {
		_, err := installerChartValues(&files.RootConfig{}, operatorBom("ghcr.io/cloudnative-pg/cloudnative-pg"), pcapps.PostgresOperator)

		Expect(err).To(MatchError(ContainSubstring("has no tag")))
	})

	It("fails on an image without a tag when the registry replaces the images, instead of pulling latest", func() {
		_, err := installerChartValues(withRegistry("registry.example.com/mirror", true), operatorBom("ghcr.io/cloudnative-pg/cloudnative-pg"), pcapps.PostgresOperator)

		Expect(err).To(MatchError(ContainSubstring("has no tag")))
	})

	It("fails on a digest-only image, which names no tag for the chart", func() {
		digest := "ghcr.io/cloudnative-pg/cloudnative-pg@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
		_, err := installerChartValues(&files.RootConfig{}, operatorBom(digest), pcapps.PostgresOperator)

		Expect(err).To(MatchError(ContainSubstring("has no tag")))
	})

	It("does not read the image of an application the run does not install", func() {
		config := &files.RootConfig{Cluster: files.ClusterConfig{PgOperator: &files.PgOperatorConfig{Enabled: false}}}

		application, err := installerChartValues(config, operatorBom("ghcr.io/cloudnative-pg/cloudnative-pg"), pcapps.PostgresOperator)

		Expect(err).NotTo(HaveOccurred())
		Expect(application).To(Equal(files.ChartValues{"enabled": false}))
	})

	It("fails on a reference it cannot parse", func() {
		_, err := installerChartValues(&files.RootConfig{}, operatorBom("Not A Ref:1"), pcapps.PostgresOperator)

		Expect(err).To(MatchError(ContainSubstring("parsing BOM image postgres-operator.cloudnativePg")))
	})

	It("fails on a registry the destination cannot be built for", func() {
		_, err := installerChartValues(withRegistry("bad registry", true), operatorBom(operatorImage), pcapps.PostgresOperator)

		Expect(err).To(MatchError(ContainSubstring("mapping BOM image postgres-operator.cloudnativePg")))
	})
})
