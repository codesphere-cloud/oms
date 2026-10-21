// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Managed services", func() {
	It("managed service with legacy plans -> parameters moved to the provider", func() {
		values := input{conf: `
			managedServices:
			  - name: postgres
			    schemaVersion: v1
			    plans:
			      - id: 1
			        name: small
			        description: Small
			        parameters:
			          storage: {pricedAs: storage, schema: {type: integer, default: 10}}
		`}.mustValues()

		Expect(values["managedServices"]).To(Equal([]any{map[string]any{
			"name":          "postgres",
			"schemaVersion": "v1",
			"scope":         "global",
			"resourceParameters": map[string]any{
				"storage": map[string]any{
					"pricedAs": "storage",
					"schema":   map[string]any{"type": "integer"},
				},
			},
			"plans": []any{map[string]any{
				"id":          1,
				"name":        "small",
				"description": "Small",
				"parameters":  map[string]any{"storage": 10},
			}},
		}}))
	})

	It("managed service resource parameter set to null -> kept as null", func() {
		values := input{conf: `
			managedServices:
			  - name: postgres
			    schemaVersion: v1
			    scope: team
			    resourceParameters: {storage: null}
		`}.mustValues()

		service := values["managedServices"].([]any)[0].(map[string]any)
		Expect(service["scope"]).To(Equal("team"))
		Expect(service["resourceParameters"]).To(Equal(map[string]any{"storage": nil}))
	})

	It("REST provider with versions -> versions dropped", func() {
		values := input{conf: `
			managedServices:
			  - name: postgres
			    schemaVersion: v1
			    backend: {api: {endpoint: "https://postgres.example.com"}}
			    versions: {1.0.0: {appVersion: "17.6"}}
		`}.mustValues()

		Expect(values["managedServices"].([]any)[0]).NotTo(HaveKey("versions"))
	})

	It("landscape provider with versions -> versions kept", func() {
		values := input{conf: `
			managedServices:
			  - name: valkey
			    schemaVersion: v0
			    backend: {landscape: {gitUrl: "https://git.example.com/valkey"}}
			    versions: {1.0.0: {appVersion: "8.1"}}
		`}.mustValues()

		Expect(values["managedServices"].([]any)[0]).To(
			HaveKeyWithValue("versions", map[string]any{"1.0.0": map[string]any{"appVersion": "8.1"}}),
		)
	})
})
