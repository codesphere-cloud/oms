// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"fmt"
	"strings"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/util"
	"github.com/distribution/reference"
)

// OperatorValues configures the chart's image from the BOM so its generated
// OPERATOR_IMAGE_NAME agrees with the images used by operator and bootstrap pods.
func OperatorValues(cfg files.RootConfig, packageBOM *bom.Config) (map[string]any, error) {
	image, err := operatorImageValues(packageBOM)
	if err != nil {
		return nil, err
	}

	values := map[string]any{"cloudnative-pg": map[string]any{
		"image":             image,
		"priorityClassName": "",
		"monitoring":        map[string]any{"podMonitorEnabled": false},
	}}
	if cfg.Cluster.PgOperator != nil {
		values = util.DeepMergeMaps(values, cfg.Cluster.PgOperator.Override)
	}

	upstream, _ := values["cloudnative-pg"].(map[string]any)
	image, _ = upstream["image"].(map[string]any)

	repository, _ := image["repository"].(string)
	if repository == "" {
		return nil, fmt.Errorf("CloudNativePG chart requires cloudnative-pg.image.repository")
	}

	registry := ""
	if cfg.Registry != nil {
		registry = cfg.Registry.Server
	}

	registry, err = installer.NormalizeRegistry(registry)
	if err != nil {
		return nil, fmt.Errorf("resolve CloudNativePG image registry: %w", err)
	}

	// Match Pod admission's rewrite rule, including its overlapping-prefix guard.
	if strings.HasPrefix(repository, installer.DefaultRegistryURL+"/codesphere-cloud/") &&
		!strings.HasPrefix(repository, registry+"/codesphere-cloud/") {
		repository = registry + strings.TrimPrefix(repository, installer.DefaultRegistryURL)
		values = util.DeepMergeMaps(values, map[string]any{"cloudnative-pg": map[string]any{
			"image": map[string]any{"repository": repository},
		}})
	}

	// OMS owns operator pull credentials; custom pull-secret lists are intentionally replaced.
	return util.DeepMergeMaps(values, map[string]any{"cloudnative-pg": map[string]any{
		"imagePullSecrets": []any{map[string]any{"name": cnpgImagePullSecretName}},
	}}), nil
}

func operatorImageValues(packageBOM *bom.Config) (map[string]any, error) {
	if packageBOM == nil {
		return nil, fmt.Errorf("installer BOM is not prepared")
	}

	image := packageBOM.Components["postgres-operator"].ContainerImages["cloudnativePg"]
	if image == "" {
		return nil, fmt.Errorf("CloudNativePG image is missing from BOM component postgres-operator (containerImages.cloudnativePg)")
	}

	named, err := reference.ParseNormalizedNamed(image)
	if err != nil {
		return nil, fmt.Errorf("parse CloudNativePG image from BOM: %w", err)
	}

	tagged, ok := named.(reference.Tagged)
	if !ok {
		return nil, fmt.Errorf("CloudNativePG image %q from BOM requires a tag", image)
	}

	tag := tagged.Tag()
	if digested, ok := named.(reference.Digested); ok {
		tag += "@" + digested.Digest().String()
	}

	return map[string]any{"repository": named.Name(), "tag": tag}, nil
}
