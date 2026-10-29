// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/codesphere"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("Managed services", func() {
	legacyParam := func(pricedAs string, schema map[string]any) files.PlanParam {
		return files.PlanParam{Legacy: files.ResourceParam{PricedAs: pricedAs, Schema: schema}}
	}

	scalarParam := func(value any) files.PlanParam {
		return files.PlanParam{IsScalar: true, Value: value}
	}

	withPlans := func(plans []files.ServicePlan) files.CodesphereConfig {
		return files.CodesphereConfig{ManagedServices: []files.ManagedServiceConfig{{
			Name:          "postgres",
			SchemaVersion: "v1",
			Plans:         plans,
		}}}
	}

	managedServices := func(plans []files.ServicePlan) []files.ManagedServiceConfig {
		values, err := codesphere.Values(withPlans(plans), defaultTestImages(), defaultTestGateways(), noMetalLB, noRegistry)
		Expect(err).NotTo(HaveOccurred())

		return values.ManagedServices
	}

	It("legacy plans -> plan values from the defaults, shared definitions on the provider", func() {
		services := managedServices([]files.ServicePlan{
			{
				ID:          0,
				Name:        "Small",
				Description: "Small Plan",
				Parameters: map[string]files.PlanParam{
					"storage": legacyParam("storage-mib", map[string]any{
						"type": "integer", "default": 512, "readOnly": false, "minimum": 512,
					}),
					"cpu":    legacyParam("cpu-tenths", map[string]any{"type": "number", "default": 1, "readOnly": true}),
					"memory": legacyParam("ram-mib", map[string]any{"type": "integer", "default": 256, "readOnly": true}),
				},
			},
			{
				ID:          1,
				Name:        "Regular",
				Description: "Regular Plan",
				Parameters: map[string]files.PlanParam{
					"storage": legacyParam("storage-mib", map[string]any{
						"type": "integer", "default": 2048, "readOnly": false, "minimum": 512,
					}),
					"cpu":    legacyParam("cpu-tenths", map[string]any{"type": "number", "default": 2.5, "readOnly": true}),
					"memory": legacyParam("ram-mib", map[string]any{"type": "integer", "default": 256, "readOnly": true}),
				},
			},
		})

		Expect(services).To(HaveLen(1))
		Expect(services[0].Scope).To(Equal("global"))
		Expect(services[0].ResourceParameters).To(Equal(map[string]*files.ResourceParam{
			"storage": {
				PricedAs: "storage-mib",
				Schema:   map[string]any{"type": "integer", "readOnly": false, "minimum": 512},
			},
			"cpu":    {PricedAs: "cpu-tenths", Schema: map[string]any{"type": "number", "readOnly": true}},
			"memory": {PricedAs: "ram-mib", Schema: map[string]any{"type": "integer", "readOnly": true}},
		}))
		Expect(services[0].Plans).To(Equal([]files.ServicePlan{
			{
				ID:          0,
				Name:        "Small",
				Description: "Small Plan",
				Parameters: map[string]files.PlanParam{
					"storage": scalarParam(512), "cpu": scalarParam(1), "memory": scalarParam(256),
				},
			},
			{
				ID:          1,
				Name:        "Regular",
				Description: "Regular Plan",
				Parameters: map[string]files.PlanParam{
					"storage": scalarParam(2048), "cpu": scalarParam(2.5), "memory": scalarParam(256),
				},
			},
		}))
	})

	It("legacy parameter without a default -> left out of the plan, definition kept", func() {
		services := managedServices([]files.ServicePlan{{
			ID:          0,
			Name:        "Small",
			Description: "Small Plan",
			Parameters: map[string]files.PlanParam{
				"storage": legacyParam("storage-mib", map[string]any{"type": "integer", "default": 512, "minimum": 512}),
				"cpu":     legacyParam("cpu-tenths", map[string]any{"type": "number", "readOnly": false, "minimum": 100}),
			},
		}})

		Expect(services[0].Plans[0].Parameters).To(Equal(map[string]files.PlanParam{"storage": scalarParam(512)}))
		Expect(services[0].ResourceParameters).To(HaveKeyWithValue("cpu", &files.ResourceParam{
			PricedAs: "cpu-tenths",
			Schema:   map[string]any{"type": "number", "readOnly": false, "minimum": 100},
		}))
	})

	It("legacy read-only parameter without a default -> error", func() {
		_, err := codesphere.Values(
			withPlans([]files.ServicePlan{{
				ID:          0,
				Name:        "Small",
				Description: "Small Plan",
				Parameters: map[string]files.PlanParam{
					"cpu": legacyParam("cpu-tenths", map[string]any{"type": "number", "readOnly": true}),
				},
			}}),
			defaultTestImages(), defaultTestGateways(), noMetalLB, noRegistry,
		)

		Expect(err).To(MatchError(ContainSubstring(`legacy plan "Small" parameter "cpu" is read-only`)))
	})

	It("legacy read-only parameter with default 0 -> plan pins 0", func() {
		services := managedServices([]files.ServicePlan{{
			ID:          0,
			Name:        "Small",
			Description: "Small Plan",
			Parameters: map[string]files.PlanParam{
				"replicas": legacyParam("replicas", map[string]any{"type": "integer", "default": 0, "readOnly": true}),
			},
		}})

		Expect(services[0].Plans[0].Parameters).To(Equal(map[string]files.PlanParam{"replicas": scalarParam(0)}))
	})

	It("new-format plans -> passed through unchanged", func() {
		plans := []files.ServicePlan{{
			ID:          0,
			Name:        "Small",
			Description: "Small Plan",
			Parameters: map[string]files.PlanParam{
				"storage": scalarParam(512), "cpu": scalarParam(1), "memory": scalarParam(256),
			},
		}}

		services := managedServices(plans)

		Expect(services[0].Plans).To(Equal(plans))
		Expect(services[0].ResourceParameters).To(BeNil())
	})

	It("plans without parameters -> passed through without resource parameters", func() {
		plans := []files.ServicePlan{{ID: 0, Name: "default", Description: "default", Parameters: map[string]files.PlanParam{}}}

		services := managedServices(plans)

		Expect(services[0].Plans).To(Equal(plans))
		Expect(services[0].ResourceParameters).To(BeNil())
	})

	It("no managed service workspace plan ID -> 20", func() {
		values, err := codesphere.Values(
			files.CodesphereConfig{}, defaultTestImages(), defaultTestGateways(), noMetalLB, noRegistry,
		)
		Expect(err).NotTo(HaveOccurred())

		Expect(values.ManagedServiceWorkspacePlanID).To(Equal(20))
	})

	It("managed service resource parameter set to null -> kept as null", func() {
		conf := files.CodesphereConfig{ManagedServices: []files.ManagedServiceConfig{{
			Name:               "postgres",
			SchemaVersion:      "v1",
			Scope:              "team",
			ResourceParameters: map[string]*files.ResourceParam{"storage": nil},
		}}}

		values, err := codesphere.Values(conf, defaultTestImages(), defaultTestGateways(), noMetalLB, noRegistry)
		Expect(err).NotTo(HaveOccurred())

		service := values.ManagedServices[0]
		Expect(service.Scope).To(Equal("team"))
		Expect(service.ResourceParameters).To(Equal(map[string]*files.ResourceParam{"storage": nil}))
	})

	It("REST provider with versions -> versions dropped", func() {
		conf := files.CodesphereConfig{ManagedServices: []files.ManagedServiceConfig{{
			Name:          "postgres",
			SchemaVersion: "v1",
			Backend:       map[string]any{"api": map[string]any{"endpoint": "https://postgres.example.com"}},
			Versions:      map[string]any{"1.0.0": map[string]any{"appVersion": "17.6"}},
		}}}

		values, err := codesphere.Values(conf, defaultTestImages(), defaultTestGateways(), noMetalLB, noRegistry)
		Expect(err).NotTo(HaveOccurred())

		Expect(values.ManagedServices[0].Versions).To(BeNil())
	})

	It("landscape provider with versions -> versions kept", func() {
		conf := files.CodesphereConfig{ManagedServices: []files.ManagedServiceConfig{{
			Name:          "valkey",
			SchemaVersion: "v0",
			Backend:       map[string]any{"landscape": map[string]any{"gitUrl": "https://git.example.com/valkey"}},
			Versions:      map[string]any{"1.0.0": map[string]any{"appVersion": "8.1"}},
		}}}

		values, err := codesphere.Values(conf, defaultTestImages(), defaultTestGateways(), noMetalLB, noRegistry)
		Expect(err).NotTo(HaveOccurred())

		Expect(values.ManagedServices[0].Versions).To(
			Equal(map[string]any{"1.0.0": map[string]any{"appVersion": "8.1"}}),
		)
	})
})
