// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"fmt"
	"maps"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

const (
	defaultProviderScope = "global"

	// Landscape managed services run their workspaces on this plan, and 20 (CS Micro) is the only
	// plan ID every environment has.
	// TODO(CU-869cwbjnx): use a plan read from the database instead.
	defaultManagedServicePlanID = 20
)

// managedServicesValues passes the configured providers on as they are, with legacy plans upgraded
// and the installer's defaults.
func managedServicesValues(services []files.ManagedServiceConfig) ([]files.ManagedServiceConfig, error) {
	if services == nil {
		return nil, nil
	}

	values := make([]files.ManagedServiceConfig, 0, len(services))
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

		// Only landscape providers have versions in the installer's schema; for REST providers, which
		// a provider without a landscape backend is, it drops them.
		if _, landscape := service.Backend["landscape"]; !landscape {
			service.Versions = nil
		}

		values = append(values, service)
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
			legacyDefinition := parameter.Legacy.PricedAs != "" || parameter.Legacy.Schema != nil
			if parameter.IsScalar || !legacyDefinition {
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

			value, pinned, err := legacyPlanValue(plan.Name, key, parameter.Legacy.Schema)
			if err != nil {
				return files.ManagedServiceConfig{}, err
			}

			if pinned {
				values[key] = value
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

// legacyPlanValue returns the numeric default a legacy plan pins for a parameter, and whether it pins
// one.
func legacyPlanValue(planName, key string, schema map[string]interface{}) (files.PlanParam, bool, error) {
	switch value := schema["default"].(type) {
	case int, float64:
		return files.PlanParam{IsScalar: true, Value: value}, true, nil
	}

	if readOnly, _ := schema["readOnly"].(bool); readOnly {
		return files.PlanParam{}, false, fmt.Errorf(
			"legacy plan %q parameter %q is read-only but has no numeric schema.default, so it pins no value",
			planName,
			key,
		)
	}

	return files.PlanParam{}, false, nil
}

func withoutDefault(schema map[string]interface{}) map[string]interface{} {
	if schema == nil {
		return nil
	}

	stripped := maps.Clone(schema)
	delete(stripped, "default")

	return stripped
}
