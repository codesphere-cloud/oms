// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codespherevalues

import (
	"fmt"
	"maps"

	"go.yaml.in/yaml/v3"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

const defaultProviderScope = "global"

func managedServicesValues(services []files.ManagedServiceConfig) ([]any, error) {
	if services == nil {
		return nil, nil
	}

	values := make([]any, 0, len(services))
	for _, service := range services {
		if isLegacyPlans(service.Plans) {
			upgraded, err := upgradeLegacyPlans(service)
			if err != nil {
				return nil, err
			}

			service = upgraded
		}

		if service.Scope == "" {
			service.Scope = defaultProviderScope
		}

		value, err := toValue(service)
		if err != nil {
			return nil, fmt.Errorf("failed to convert managed service %s: %w", service.Name, err)
		}

		values = append(values, value)
	}

	return values, nil
}

// isLegacyPlans reports whether the plans use the legacy shape, where each plan carries a full
// parameter definition instead of a value.
// TODO(CU-869ev5bn2): delete once legacy support is dropped.
func isLegacyPlans(plans []files.ServicePlan) bool {
	hasParameters := false

	for _, plan := range plans {
		for _, parameter := range plan.Parameters {
			if parameter.IsScalar || (parameter.Legacy.PricedAs == "" && parameter.Legacy.Schema == nil) {
				return false
			}

			hasParameters = true
		}
	}

	return hasParameters
}

// upgradeLegacyPlans moves the parameter definitions to the provider and keeps each plan's default
// value as its value.
// TODO(CU-869ev5bn2): delete once legacy support is dropped.
func upgradeLegacyPlans(service files.ManagedServiceConfig) (files.ManagedServiceConfig, error) {
	resourceParameters := map[string]*files.ResourceParam{}

	plans := make([]files.ServicePlan, 0, len(service.Plans))
	for _, plan := range service.Plans {
		values := map[string]files.PlanParam{}

		for key, parameter := range plan.Parameters {
			if _, ok := resourceParameters[key]; !ok {
				resourceParameters[key] = &files.ResourceParam{
					PricedAs: parameter.Legacy.PricedAs,
					Schema:   withoutDefault(parameter.Legacy.Schema),
				}
			}

			value, err := legacyPlanValue(plan.Name, key, parameter.Legacy.Schema)
			if err != nil {
				return files.ManagedServiceConfig{}, err
			}

			if value != nil {
				values[key] = files.PlanParam{IsScalar: true, Value: value}
			}
		}

		plans = append(plans, files.ServicePlan{
			ID:          plan.ID,
			Name:        plan.Name,
			Description: plan.Description,
			Parameters:  values,
		})
	}

	service.Plans = plans
	service.ResourceParameters = resourceParameters

	return service, nil
}

// legacyPlanValue returns the numeric default a legacy plan pins for a parameter, or nil if it pins
// none.
func legacyPlanValue(planName, key string, schema map[string]interface{}) (any, error) {
	switch value := schema["default"].(type) {
	case int, float64:
		return value, nil
	}

	if readOnly, _ := schema["readOnly"].(bool); readOnly {
		return nil, fmt.Errorf("legacy plan %q parameter %q is read-only but has no numeric schema.default, so it pins no value", planName, key)
	}

	return nil, nil
}

func withoutDefault(schema map[string]interface{}) map[string]interface{} {
	if schema == nil {
		return nil
	}

	stripped := maps.Clone(schema)
	delete(stripped, "default")

	return stripped
}

// toValue converts a config struct into plain values, keeping its YAML field names and shapes.
func toValue(config any) (map[string]any, error) {
	data, err := yaml.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal: %w", err)
	}

	var value map[string]any
	if err := yaml.Unmarshal(data, &value); err != nil {
		return nil, fmt.Errorf("failed to unmarshal: %w", err)
	}

	return value, nil
}
