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
	It("managed service with legacy plans -> parameters moved to the provider", func() {
		legacyStorage := files.ResourceParam{
			PricedAs: "storage",
			Schema:   map[string]any{"type": "integer", "default": 10},
		}
		conf := files.CodesphereConfig{ManagedServices: []files.ManagedServiceConfig{{
			Name:          "postgres",
			SchemaVersion: "v1",
			Plans: []files.ServicePlan{{
				ID:          1,
				Name:        "small",
				Description: "Small",
				Parameters:  map[string]files.PlanParam{"storage": {Legacy: legacyStorage}},
			}},
		}}}

		values, err := codesphere.Values(conf, defaultTestImages(), defaultTestGateways(), noMetalLB, noRegistry)
		Expect(err).NotTo(HaveOccurred())

		Expect(values.ManagedServices).To(HaveLen(1))
		service := values.ManagedServices[0]
		Expect(service.Scope).To(Equal("global"))
		Expect(service.ResourceParameters).To(Equal(map[string]*files.ResourceParam{
			"storage": {PricedAs: "storage", Schema: map[string]any{"type": "integer"}},
		}))
		Expect(service.Plans).To(Equal([]files.ServicePlan{{
			ID:          1,
			Name:        "small",
			Description: "Small",
			Parameters:  map[string]files.PlanParam{"storage": {IsScalar: true, Value: 10}},
		}}))
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
