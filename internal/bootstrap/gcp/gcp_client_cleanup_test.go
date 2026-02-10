// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/api/googleapi"

	"github.com/codesphere-cloud/oms/internal/bootstrap/gcp"
)

var _ = Describe("GCP Client Cleanup Methods", func() {
	Describe("OMSManagedLabel constant", func() {
		It("should be set to 'oms-managed'", func() {
			Expect(gcp.OMSManagedLabel).To(Equal("oms-managed"))
		})
	})

	Describe("CheckOMSManagedLabel", func() {
		Context("when labels contain oms-managed=true", func() {
			It("should return true", func() {
				labels := map[string]string{
					gcp.OMSManagedLabel: "true",
				}
				Expect(gcp.CheckOMSManagedLabel(labels)).To(BeTrue())
			})
		})

		Context("when labels contain oms-managed=false", func() {
			It("should return false", func() {
				labels := map[string]string{
					gcp.OMSManagedLabel: "false",
				}
				Expect(gcp.CheckOMSManagedLabel(labels)).To(BeFalse())
			})
		})

		Context("when labels do not contain oms-managed", func() {
			It("should return false", func() {
				labels := map[string]string{
					"other-label": "value",
				}
				Expect(gcp.CheckOMSManagedLabel(labels)).To(BeFalse())
			})
		})

		Context("when labels map is nil", func() {
			It("should return false", func() {
				Expect(gcp.CheckOMSManagedLabel(nil)).To(BeFalse())
			})
		})

		Context("when labels map is empty", func() {
			It("should return false", func() {
				labels := map[string]string{}
				Expect(gcp.CheckOMSManagedLabel(labels)).To(BeFalse())
			})
		})

		Context("when checking case sensitivity", func() {
			It("should be case-sensitive for label values", func() {
				testCases := []struct {
					value    string
					expected bool
				}{
					{"true", true},
					{"True", false},
					{"TRUE", false},
					{"1", false},
					{"yes", false},
					{"", false},
				}

				for _, tc := range testCases {
					labels := map[string]string{
						gcp.OMSManagedLabel: tc.value,
					}
					Expect(gcp.CheckOMSManagedLabel(labels)).To(Equal(tc.expected),
						"Label value '%s' should result in %v", tc.value, tc.expected)
				}
			})
		})

		Context("when multiple labels exist", func() {
			It("should correctly identify oms-managed among other labels", func() {
				labels := map[string]string{
					gcp.OMSManagedLabel: "true",
					"environment":       "production",
					"team":              "platform",
					"managed-by":        "terraform",
				}
				Expect(gcp.CheckOMSManagedLabel(labels)).To(BeTrue())
			})
		})
	})

	Describe("GetDNSRecordNames", func() {
		Context("when given a simple base domain", func() {
			It("should generate correct DNS record names", func() {
				baseDomain := "example.com"
				records := gcp.GetDNSRecordNames(baseDomain)

				Expect(records).To(HaveLen(4))
				Expect(records[0].Name).To(Equal("cs.example.com."))
				Expect(records[0].Rtype).To(Equal("A"))
				Expect(records[1].Name).To(Equal("*.cs.example.com."))
				Expect(records[1].Rtype).To(Equal("A"))
				Expect(records[2].Name).To(Equal("ws.example.com."))
				Expect(records[2].Rtype).To(Equal("A"))
				Expect(records[3].Name).To(Equal("*.ws.example.com."))
				Expect(records[3].Rtype).To(Equal("A"))
			})
		})

		Context("when given a subdomain", func() {
			It("should handle domains with subdomains correctly", func() {
				baseDomain := "internal.codesphere.com"
				records := gcp.GetDNSRecordNames(baseDomain)

				Expect(records).To(HaveLen(4))
				for _, record := range records {
					Expect(record.Name).To(ContainSubstring("internal.codesphere.com"))
					Expect(record.Name).To(HaveSuffix("."))
					Expect(record.Rtype).To(Equal("A"))
				}
			})
		})

		Context("when ensuring all records are A type", func() {
			It("should only generate A records", func() {
				records := gcp.GetDNSRecordNames("test.com")
				for _, record := range records {
					Expect(record.Rtype).To(Equal("A"))
				}
			})
		})

		Context("when ensuring trailing dot format", func() {
			It("should append trailing dot for DNS FQDN format", func() {
				records := gcp.GetDNSRecordNames("nodot.com")
				for _, record := range records {
					Expect(record.Name).To(HaveSuffix("."))
				}
			})
		})
	})

	Describe("IsNotFoundError", func() {
		Context("when error is nil", func() {
			It("should return false", func() {
				Expect(gcp.IsNotFoundError(nil)).To(BeFalse())
			})
		})

		Context("when error is a Google API 404 error", func() {
			It("should return true", func() {
				err := &googleapi.Error{
					Code:    404,
					Message: "not found",
				}
				Expect(gcp.IsNotFoundError(err)).To(BeTrue())
			})
		})

		Context("when error is a Google API non-404 error", func() {
			It("should return false for 403 Forbidden", func() {
				err := &googleapi.Error{
					Code:    403,
					Message: "forbidden",
				}
				Expect(gcp.IsNotFoundError(err)).To(BeFalse())
			})

			It("should return false for 500 Internal Server Error", func() {
				err := &googleapi.Error{
					Code:    500,
					Message: "internal error",
				}
				Expect(gcp.IsNotFoundError(err)).To(BeFalse())
			})

			It("should return false for 401 Unauthorized", func() {
				err := &googleapi.Error{
					Code:    401,
					Message: "unauthorized",
				}
				Expect(gcp.IsNotFoundError(err)).To(BeFalse())
			})
		})

		Context("when error is a non-Google API error", func() {
			It("should return false", func() {
				err := fmt.Errorf("some other error")
				Expect(gcp.IsNotFoundError(err)).To(BeFalse())
			})
		})

		Context("when error wraps a Google API 404 error", func() {
			It("should return true for wrapped 404 errors", func() {
				innerErr := &googleapi.Error{
					Code:    404,
					Message: "not found",
				}
				wrappedErr := fmt.Errorf("failed to get record: %w", innerErr)
				Expect(gcp.IsNotFoundError(wrappedErr)).To(BeTrue())
			})

			It("should return false for wrapped non-404 errors", func() {
				innerErr := &googleapi.Error{
					Code:    403,
					Message: "forbidden",
				}
				wrappedErr := fmt.Errorf("failed to get record: %w", innerErr)
				Expect(gcp.IsNotFoundError(wrappedErr)).To(BeFalse())
			})
		})
	})
})
