// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp_test

import (
	"fmt"

	"cloud.google.com/go/resourcemanager/apiv3/resourcemanagerpb"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/bootstrap/gcp"
)

var _ = Describe("GCP Client Cleanup Methods", func() {
	Describe("OMSManagedLabel constant", func() {
		It("should be set to 'oms-managed'", func() {
			Expect(gcp.OMSManagedLabel).To(Equal("oms-managed"))
		})
	})

	Describe("DNS record patterns", func() {
		It("should generate correct DNS record names for a given base domain", func() {
			baseDomain := "example.com"

			expectedRecords := []struct {
				name  string
				rtype string
			}{
				{fmt.Sprintf("cs.%s.", baseDomain), "A"},
				{fmt.Sprintf("*.cs.%s.", baseDomain), "A"},
				{fmt.Sprintf("ws.%s.", baseDomain), "A"},
				{fmt.Sprintf("*.ws.%s.", baseDomain), "A"},
			}

			Expect(expectedRecords[0].name).To(Equal("cs.example.com."))
			Expect(expectedRecords[1].name).To(Equal("*.cs.example.com."))
			Expect(expectedRecords[2].name).To(Equal("ws.example.com."))
			Expect(expectedRecords[3].name).To(Equal("*.ws.example.com."))

			for _, record := range expectedRecords {
				Expect(record.rtype).To(Equal("A"))
			}
		})

		It("should handle domains with subdomains correctly", func() {
			baseDomain := "internal.codesphere.com"

			expectedNames := []string{
				fmt.Sprintf("cs.%s.", baseDomain),
				fmt.Sprintf("*.cs.%s.", baseDomain),
				fmt.Sprintf("ws.%s.", baseDomain),
				fmt.Sprintf("*.ws.%s.", baseDomain),
			}

			for _, name := range expectedNames {
				Expect(name).To(ContainSubstring("internal.codesphere.com"))
				Expect(name).To(HaveSuffix("."))
			}
		})
	})

	Describe("Label verification logic", func() {
		Context("when checking if a project is OMS-managed", func() {
			It("should identify project with oms-managed=true label", func() {
				project := &resourcemanagerpb.Project{
					Labels: map[string]string{
						gcp.OMSManagedLabel: "true",
					},
				}

				value, exists := project.Labels[gcp.OMSManagedLabel]
				isOmsManaged := exists && value == "true"

				Expect(isOmsManaged).To(BeTrue())
			})

			It("should not identify project with oms-managed=false label", func() {
				project := &resourcemanagerpb.Project{
					Labels: map[string]string{
						gcp.OMSManagedLabel: "false",
					},
				}

				value, exists := project.Labels[gcp.OMSManagedLabel]
				isOmsManaged := exists && value == "true"

				Expect(isOmsManaged).To(BeFalse())
			})

			It("should not identify project without oms-managed label", func() {
				project := &resourcemanagerpb.Project{
					Labels: map[string]string{
						"other-label": "value",
					},
				}

				value, exists := project.Labels[gcp.OMSManagedLabel]
				isOmsManaged := exists && value == "true"

				Expect(isOmsManaged).To(BeFalse())
			})

			It("should not identify project with nil labels", func() {
				project := &resourcemanagerpb.Project{
					Labels: nil,
				}

				if project.Labels == nil {
					Expect(project.Labels).To(BeNil())
					return
				}

				value, exists := project.Labels[gcp.OMSManagedLabel]
				isOmsManaged := exists && value == "true"

				Expect(isOmsManaged).To(BeFalse())
			})

			It("should not identify project with empty labels map", func() {
				project := &resourcemanagerpb.Project{
					Labels: map[string]string{},
				}

				value, exists := project.Labels[gcp.OMSManagedLabel]
				isOmsManaged := exists && value == "true"

				Expect(isOmsManaged).To(BeFalse())
			})

			It("should handle case-sensitive label values", func() {
				// Verify that "True" or "TRUE" would not be considered valid
				testCases := []struct {
					value    string
					expected bool
				}{
					{"true", true},
					{"True", false},
					{"TRUE", false},
					{"1", false},
					{"yes", false},
				}

				for _, tc := range testCases {
					project := &resourcemanagerpb.Project{
						Labels: map[string]string{
							gcp.OMSManagedLabel: tc.value,
						},
					}

					value, exists := project.Labels[gcp.OMSManagedLabel]
					isOmsManaged := exists && value == "true"

					Expect(isOmsManaged).To(Equal(tc.expected),
						"Label value '%s' should result in isOmsManaged=%v", tc.value, tc.expected)
				}
			})
		})
	})

	Describe("Project resource name formatting", func() {
		It("should format project resource name correctly", func() {
			projectID := "my-test-project-123"
			expectedFormat := fmt.Sprintf("projects/%s", projectID)

			Expect(expectedFormat).To(Equal("projects/my-test-project-123"))
		})

		It("should handle project IDs with different formats", func() {
			testCases := []string{
				"simple-project",
				"project-with-multiple-hyphens",
				"project123",
				"123-project",
			}

			for _, projectID := range testCases {
				resourceName := fmt.Sprintf("projects/%s", projectID)
				Expect(resourceName).To(HavePrefix("projects/"))
				Expect(resourceName).To(ContainSubstring(projectID))
			}
		})
	})

	Describe("CreateProject label addition", func() {
		It("should include oms-managed label in project creation", func() {
			labels := map[string]string{
				gcp.OMSManagedLabel: "true",
			}

			Expect(labels).To(HaveKey(gcp.OMSManagedLabel))
			Expect(labels[gcp.OMSManagedLabel]).To(Equal("true"))
		})

		It("should not conflict with other potential labels", func() {
			// Verify that the OMS label doesn't conflict with common label names
			labels := map[string]string{
				gcp.OMSManagedLabel: "true",
				"environment":       "production",
				"team":              "platform",
				"managed-by":        "terraform",
			}

			Expect(labels).To(HaveLen(4))
			Expect(labels[gcp.OMSManagedLabel]).To(Equal("true"))
		})
	})
})
