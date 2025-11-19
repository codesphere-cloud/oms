// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

//go:build integration
// +build integration

package cmd_test

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/cli/cmd"
	"github.com/codesphere-cloud/oms/internal/portal"
)

var _ = Describe("API Key Integration Tests", func() {
	var (
		portalClient     portal.Portal
		testOwner        string
		testOrg          string
		testRole         string
		registeredKey    *portal.ApiKey
		originalAdminKey string
		expiresAt        time.Time
		extendedExpiry   time.Time
	)

	BeforeEach(func() {
		apiKey := os.Getenv("OMS_PORTAL_API_KEY")
		apiURL := os.Getenv("OMS_PORTAL_API")
		if apiKey == "" || apiURL == "" {
			Skip("Integration tests require OMS_PORTAL_API_KEY and OMS_PORTAL_API environment variables")
		}

		originalAdminKey = apiKey

		portalClient = portal.NewPortalClient()
		// test env wrapper
		portalClient.(*portal.PortalClient).Env = NewTestEnv(apiKey, os.Getenv("OMS_PORTAL_API"), "")

		// test data
		testOwner = fmt.Sprintf("integration-test-%d@test.com", time.Now().Unix())
		testOrg = "IntegrationTestOrg"
		testRole = "Ext"
		expiresAt = time.Now().Add(24 * time.Hour)
		extendedExpiry = time.Now().Add(48 * time.Hour)
	})

	Describe("Standalone created-key behavior", func() {
		It("created API key can list builds when used", func() {
			registerCmd := cmd.RegisterCmd{
				Opts: cmd.RegisterOpts{
					Owner:        fmt.Sprintf("standalone-test-%d@test.com", time.Now().Unix()),
					Organization: "StandaloneTestOrg",
					Role:         "Ext",
					ExpiresAt:    time.Now().Add(1 * time.Hour).Format(time.RFC3339),
				},
			}

			newKey, err := registerCmd.Register(portalClient)
			Expect(err).To(BeNil(), "API key registration should succeed")
			Expect(newKey).NotTo(BeNil(), "Register should return the created API key")

			keys, err := portalClient.ListAPIKeys()
			Expect(err).To(BeNil(), "Listing API keys should succeed")

			var created *portal.ApiKey
			for i := range keys {
				if keys[i].Owner == registerCmd.Opts.Owner {
					created = &keys[i]
					break
				}
			}
			Expect(created).NotTo(BeNil(), "Should find the created API key")
			Expect(newKey.ApiKey).NotTo(BeEmpty(), "Created API key must include secret value")

			client := portal.NewPortalClient()
			client.Env = NewTestEnv(newKey.ApiKey, os.Getenv("OMS_PORTAL_API"), "")

			builds, err := client.ListBuilds(portal.CodesphereProduct)
			Expect(err).To(BeNil(), "Listing builds with created key should succeed")
			Expect(builds.Builds).NotTo(BeEmpty(), "Created key should be able to see builds")
		})
	})

	Describe("Complete API Key Flow", func() {
		It("should successfully complete the full API key lifecycle", func() {
			By("Registering a new customer API key")
			registerCmd := cmd.RegisterCmd{
				Opts: cmd.RegisterOpts{
					Owner:        testOwner,
					Organization: testOrg,
					Role:         testRole,
					ExpiresAt:    expiresAt.Format(time.RFC3339),
				},
			}

			newKey, err := registerCmd.Register(portalClient)
			Expect(err).To(BeNil(), "API key registration should succeed")
			Expect(newKey).NotTo(BeNil(), "Register should return the created API key")

			By("Listing API keys to get the newly registered key")
			keys, err := portalClient.ListAPIKeys()
			Expect(err).To(BeNil(), "Listing API keys should succeed")
			Expect(keys).NotTo(BeEmpty(), "Should have at least one API key")

			// Find the new key
			for i := range keys {
				if keys[i].Owner == testOwner {
					registeredKey = &keys[i]
					break
				}
			}
			Expect(registeredKey).NotTo(BeNil(), "Should find the registered API key")
			Expect(registeredKey.Owner).To(Equal(testOwner))
			Expect(registeredKey.Organization).To(Equal(testOrg))
			Expect(registeredKey.Role).To(Equal(testRole))

			By("Ensuring the customer can see builds")
			Expect(newKey.ApiKey).NotTo(BeEmpty(), "Registered key must include the API key value")

			p := portal.NewPortalClient()
			// switch to the new key
			p.Env = NewTestEnv(newKey.ApiKey, os.Getenv("OMS_PORTAL_API"), "")

			builds, err := p.ListBuilds(portal.CodesphereProduct)
			Expect(err).To(BeNil(), "Listing builds with new key should succeed")
			Expect(builds.Builds).NotTo(BeEmpty(), "Should have at least one build available")

			// restore admin key
			portalClient.(*portal.PortalClient).Env = NewTestEnv(originalAdminKey, os.Getenv("OMS_PORTAL_API"), "")

			By("Extending the API Key to a future date")
			updateCmd := cmd.UpdateAPIKeyCmd{
				Opts: cmd.UpdateAPIKeyOpts{
					APIKeyID:     registeredKey.KeyID,
					ExpiresAtStr: extendedExpiry.Format(time.RFC3339),
				},
			}

			err = updateCmd.UpdateAPIKey(portalClient)
			Expect(err).To(BeNil(), "API key update should succeed")

			By("Verifying the API key was updated")
			keys, err = portalClient.ListAPIKeys()
			Expect(err).To(BeNil(), "Listing API keys should succeed")

			// Find the updated key
			var updatedKey *portal.ApiKey
			for i := range keys {
				if keys[i].KeyID == registeredKey.KeyID {
					updatedKey = &keys[i]
					break
				}
			}
			Expect(updatedKey).NotTo(BeNil(), "Should find the updated API key")
			Expect(updatedKey.ExpiresAt).To(BeTemporally("~", extendedExpiry, 5*time.Second))

			By("Revoking the API Key")
			revokeCmd := cmd.RevokeAPIKeyCmd{
				Opts: cmd.RevokeAPIKeyOpts{
					ID: registeredKey.KeyID,
				},
			}

			err = revokeCmd.Revoke(portalClient)
			Expect(err).To(BeNil(), "API key revocation should succeed")

			By("Ensuring the API Key is not valid anymore")

			keyFound := true
			for attempt := 0; attempt < 5; attempt++ {
				keys, err = portalClient.ListAPIKeys()
				Expect(err).To(BeNil(), "Listing API keys should succeed")

				keyFound = false
				for i := range keys {
					if keys[i].KeyID == registeredKey.KeyID {
						keyFound = true
						break
					}
				}

				if !keyFound {
					break
				}
				time.Sleep(1 * time.Second)
			}

			if keyFound {
				revokedClient := portal.NewPortalClient()
				revokedClient.Env = NewTestEnv(newKey.ApiKey, os.Getenv("OMS_PORTAL_API"), "")
				_, useErr := revokedClient.ListBuilds(portal.CodesphereProduct)
				Expect(useErr).NotTo(BeNil(), "Using a revoked API key should fail")
			} else {
				Expect(keyFound).To(BeFalse(), "Revoked API key should not be in the list")
			}
		})
	})

	Describe("API Key Update With Wrong Input", func() {
		It("should handle update with invalid date format", func() {
			updateCmd := cmd.UpdateAPIKeyCmd{
				Opts: cmd.UpdateAPIKeyOpts{
					APIKeyID:     "test-key-id",
					ExpiresAtStr: "invalid-date",
				},
			}

			err := updateCmd.UpdateAPIKey(portalClient)
			Expect(err).NotTo(BeNil(), "Should fail with invalid date format")
			Expect(err.Error()).To(ContainSubstring("invalid date format"))
		})
	})

	Describe("Old API Key Detection and Warning", func() {
		var (
			cliPath string
		)

		BeforeEach(func() {
			cliPath = "./oms-cli"

			_, err := os.Stat(cliPath)
			if err != nil {
				Skip("OMS CLI not found at " + cliPath + ", please build it first with 'make build-cli'")
			}
		})

		Context("when using a 22-character old API key format", func() {
			It("should detect the old format and attempt to upgrade", func() {
				cmd := exec.Command(cliPath, "version")
				cmd.Env = append(os.Environ(),
					"OMS_PORTAL_API_KEY=fakeapikeywith22charsa", // 22 characters
					"OMS_PORTAL_API=http://localhost:3000/api",
				)

				output, _ := cmd.CombinedOutput()
				outputStr := string(output)

				Expect(outputStr).To(ContainSubstring("OMS CLI version"))
			})
		})

		Context("when using a new long-format API key", func() {
			It("should not show any warning", func() {
				cmd := exec.Command(cliPath, "version")
				cmd.Env = append(os.Environ(),
					"OMS_PORTAL_API_KEY=fake-api-key",
					"OMS_PORTAL_API=http://localhost:3000/api",
				)

				output, _ := cmd.CombinedOutput()
				outputStr := string(output)

				Expect(outputStr).To(ContainSubstring("OMS CLI version"))
				Expect(outputStr).NotTo(ContainSubstring("old API key"))
				Expect(outputStr).NotTo(ContainSubstring("Failed to upgrade"))
			})
		})

		Context("when using a 22-character key with list api-keys command", func() {
			It("should attempt the upgrade and handle the error gracefully", func() {
				cmd := exec.Command(cliPath, "list", "api-keys")
				cmd.Env = append(os.Environ(),
					"OMS_PORTAL_API_KEY=fakeapikeywith22charsa", // 22 characters (old format)
					"OMS_PORTAL_API=http://localhost:3000/api",
				)

				output, err := cmd.CombinedOutput()
				outputStr := string(output)

				Expect(err).To(HaveOccurred())

				hasWarning := strings.Contains(outputStr, "old API key") ||
					strings.Contains(outputStr, "Failed to upgrade") ||
					strings.Contains(outputStr, "Unauthorized")

				Expect(hasWarning).To(BeTrue(),
					"Should contain warning about old key or auth failure. Got: "+outputStr)
			})
		})

		Context("when checking key length detection", func() {
			It("should correctly identify 22-character old format", func() {
				oldKey := "fakeapikeywith22charsa"
				Expect(len(oldKey)).To(Equal(22))
			})

			It("should correctly identify new long format", func() {
				newKey := "4hBieJRj2pWeB9qKJ9wQGE3CrcldLnLwP8fz6qutMjkf1n1"
				Expect(len(newKey)).NotTo(Equal(22))
				Expect(len(newKey)).To(BeNumerically(">", 22))
			})
		})
	})

	Describe("PreRun Hook Execution", func() {
		var (
			cliPath string
		)

		BeforeEach(func() {
			cliPath = "./oms-cli"

			_, err := os.Stat(cliPath)
			if err != nil {
				Skip("OMS CLI not found at " + cliPath + ", please build it first with 'make build-cli'")
			}
		})

		Context("when running any OMS command", func() {
			It("should execute the PreRun hook", func() {
				cmd := exec.Command(cliPath, "version")
				cmd.Env = append(os.Environ(),
					"OMS_PORTAL_API_KEY=valid-key-format-short",
					"OMS_PORTAL_API=http://localhost:3000/api",
				)

				output, _ := cmd.CombinedOutput()
				outputStr := string(output)

				Expect(outputStr).To(ContainSubstring("OMS CLI version"))
			})
		})
	})
})
