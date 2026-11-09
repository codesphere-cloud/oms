// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/argocd/pcapps"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("OpenFgaPcAppsValues", func() {
	configWith := func(fga *files.OpenFgaConfig, issuer files.CertIssuerType) *files.RootConfig {
		config := &files.RootConfig{}
		config.Codesphere.OpenFga = fga
		config.Codesphere.CertIssuer = &files.CertIssuerConfig{Type: issuer}

		return config
	}

	// A vault of an installation that has no preshared key, i.e. everything written
	// before oms started generating one.
	emptyVault := func() *files.InstallVault {
		return &files.InstallVault{}
	}

	vaultWithPresharedKey := func() *files.InstallVault {
		return &files.InstallVault{Secrets: []files.SecretEntry{{
			Name:   files.SecretOpenFgaPresharedKey,
			Fields: &files.SecretFields{Password: "0123456789abcdef"},
		}}}
	}

	// The application entry of the rendered values, or nil if there is none.
	openfgaValues := func(values files.ChartValues) files.ChartValues {
		apps, ok := values["applications"].(files.ChartValues)
		Expect(ok).To(BeTrue(), "expected an applications map")

		return apps["openfga"].(files.ChartValues)
	}

	// The authn block the openfga chart is configured with, or nil if there is none.
	authnValues := func(values files.ChartValues) files.ChartValues {
		chartValues, ok := openfgaValues(values)["valuesObject"].(files.ChartValues)
		if !ok {
			return nil
		}

		openfga, ok := chartValues["openfga"].(files.ChartValues)
		if !ok {
			return nil
		}

		return openfga["authn"].(files.ChartValues)
	}

	It("leaves the chart defaults alone when the config says nothing", func() {
		Expect(pcapps.OpenFgaPcAppsValues(configWith(nil, ""), emptyVault())).To(BeNil())
	})

	It("disables the application for a data center that uses a remote OpenFGA", func() {
		deploy := false
		values := pcapps.OpenFgaPcAppsValues(configWith(&files.OpenFgaConfig{
			Deploy: &deploy,
			APIURL: "https://openfga.1.cs.example.com",
		}, files.CertIssuerTypeACME), emptyVault())

		fga := openfgaValues(values)
		Expect(fga["enabled"]).To(BeFalse())
		// Nothing to expose, so the gateway is not configured at all.
		Expect(fga).NotTo(HaveKey("valuesObject"))
	})

	It("defaults to deploying when only the exposure is configured", func() {
		values := pcapps.OpenFgaPcAppsValues(configWith(&files.OpenFgaConfig{
			Expose: &files.OpenFgaExposeConfig{Enabled: true, Host: "openfga.1.cs.example.com"},
		}, files.CertIssuerTypeACME), emptyVault())

		fga := openfgaValues(values)
		Expect(fga["enabled"]).To(BeTrue())

		gateway := fga["valuesObject"].(files.ChartValues)["gateway"].(files.ChartValues)
		Expect(gateway["enabled"]).To(BeTrue())
		Expect(gateway["host"]).To(Equal("openfga.1.cs.example.com"))
	})

	It("issues the gateway certificate with the installation's cert issuer", func() {
		values := pcapps.OpenFgaPcAppsValues(configWith(&files.OpenFgaConfig{
			Expose: &files.OpenFgaExposeConfig{Enabled: true, Host: "openfga.1.cs.example.com"},
		}, files.CertIssuerTypeACME), emptyVault())

		gateway := openfgaValues(values)["valuesObject"].(files.ChartValues)["gateway"].(files.ChartValues)
		issuerRef := gateway["tls"].(files.ChartValues)["certificate"].(files.ChartValues)["issuerRef"].(files.ChartValues)
		Expect(issuerRef["name"]).To(Equal("acme"))
	})

	It("falls back to the self-signed issuer when no cert issuer is configured", func() {
		values := pcapps.OpenFgaPcAppsValues(configWith(&files.OpenFgaConfig{
			Expose: &files.OpenFgaExposeConfig{Enabled: true, Host: "openfga.1.cs.example.com"},
		}, ""), emptyVault())

		gateway := openfgaValues(values)["valuesObject"].(files.ChartValues)["gateway"].(files.ChartValues)
		issuerRef := gateway["tls"].(files.ChartValues)["certificate"].(files.ChartValues)["issuerRef"].(files.ChartValues)
		Expect(issuerRef["name"]).To(Equal("self-signed"))
	})

	It("falls back to the self-signed issuer when the cert issuer block is absent", func() {
		config := configWith(&files.OpenFgaConfig{
			Expose: &files.OpenFgaExposeConfig{Enabled: true, Host: "openfga.1.cs.example.com"},
		}, "")
		config.Codesphere.CertIssuer = nil

		values := pcapps.OpenFgaPcAppsValues(config, emptyVault())

		gateway := openfgaValues(values)["valuesObject"].(files.ChartValues)["gateway"].(files.ChartValues)
		issuerRef := gateway["tls"].(files.ChartValues)["certificate"].(files.ChartValues)["issuerRef"].(files.ChartValues)
		Expect(issuerRef["name"]).To(Equal("self-signed"))
	})

	It("leaves OpenFGA unauthenticated while the vault has no preshared key", func() {
		values := pcapps.OpenFgaPcAppsValues(configWith(&files.OpenFgaConfig{}, ""), emptyVault())

		Expect(authnValues(values)).To(BeNil())
	})

	It("requires the preshared key once the vault holds one", func() {
		values := pcapps.OpenFgaPcAppsValues(configWith(nil, ""), vaultWithPresharedKey())

		authn := authnValues(values)
		Expect(authn["method"]).To(Equal("preshared"))
		Expect(authn["preshared"].(files.ChartValues)["keysSecret"]).To(Equal("openfga-preshared-keys"))
	})

	It("keeps the gateway configuration when authentication is derived as well", func() {
		values := pcapps.OpenFgaPcAppsValues(configWith(&files.OpenFgaConfig{
			Expose: &files.OpenFgaExposeConfig{Enabled: true, Host: "openfga.1.cs.example.com"},
		}, files.CertIssuerTypeACME), vaultWithPresharedKey())

		Expect(authnValues(values)["method"]).To(Equal("preshared"))
		gateway := openfgaValues(values)["valuesObject"].(files.ChartValues)["gateway"].(files.ChartValues)
		Expect(gateway["host"]).To(Equal("openfga.1.cs.example.com"))
	})

	It("ignores an entry that is present but empty", func() {
		vault := &files.InstallVault{Secrets: []files.SecretEntry{{
			Name:   files.SecretOpenFgaPresharedKey,
			Fields: &files.SecretFields{Password: ""},
		}}}

		Expect(pcapps.OpenFgaPcAppsValues(configWith(nil, ""), vault)).To(BeNil())
	})

	Context("with openfgaBackups", func() {
		configWithBackups := func(fga *files.OpenFgaConfig, backups *files.OpenfgaBackupsConfig) *files.RootConfig {
			config := configWith(fga, "")
			config.Codesphere.OpenfgaBackups = backups

			return config
		}

		backupValues := func(values files.ChartValues) files.ChartValues {
			postgres := openfgaValues(values)["valuesObject"].(files.ChartValues)["postgres"].(files.ChartValues)

			return postgres["backup"].(files.ChartValues)
		}

		It("configures the database backup from a hand-written block", func() {
			values := pcapps.OpenFgaPcAppsValues(configWithBackups(nil, &files.OpenfgaBackupsConfig{
				Enabled:         true,
				Schedule:        "0 */30 * * * *",
				DestinationPath: "s3://backup-openfga-dev",
				EndpointURL:     "https://s3.de.io.cloud.ovh.net",
				RetentionPolicy: "14d",
			}), emptyVault())

			Expect(backupValues(values)).To(Equal(files.ChartValues{
				"enabled":         true,
				"schedule":        "0 */30 * * * *",
				"destinationPath": "s3://backup-openfga-dev",
				"endpointURL":     "https://s3.de.io.cloud.ovh.net",
				"retentionPolicy": "14d",
			}))
		})

		It("leaves schedule and retention to the chart when they are unset", func() {
			values := pcapps.OpenFgaPcAppsValues(configWithBackups(nil, &files.OpenfgaBackupsConfig{
				Enabled:         true,
				DestinationPath: "s3://backup-openfga-dev",
				EndpointURL:     "https://s3.de.io.cloud.ovh.net",
			}), emptyVault())

			Expect(backupValues(values)).To(Equal(files.ChartValues{
				"enabled":         true,
				"destinationPath": "s3://backup-openfga-dev",
				"endpointURL":     "https://s3.de.io.cloud.ovh.net",
			}))
		})

		It("turns backups off explicitly when the block disables them", func() {
			values := pcapps.OpenFgaPcAppsValues(configWithBackups(nil, &files.OpenfgaBackupsConfig{
				Enabled: false,
			}), emptyVault())

			Expect(backupValues(values)).To(Equal(files.ChartValues{"enabled": false}))
		})

		It("leaves enabling the application to the chart when only backups are configured", func() {
			values := pcapps.OpenFgaPcAppsValues(configWithBackups(nil, &files.OpenfgaBackupsConfig{
				Enabled:         true,
				DestinationPath: "s3://backup-openfga-dev",
				EndpointURL:     "https://s3.de.io.cloud.ovh.net",
			}), emptyVault())

			Expect(openfgaValues(values)).NotTo(HaveKey("enabled"))
		})

		It("keeps the application disabled for a data center that uses a remote OpenFGA", func() {
			deploy := false
			values := pcapps.OpenFgaPcAppsValues(configWithBackups(&files.OpenFgaConfig{
				Deploy: &deploy,
				APIURL: "https://openfga.1.cs.example.com",
			}, &files.OpenfgaBackupsConfig{
				Enabled:         true,
				DestinationPath: "s3://backup-openfga-dev",
				EndpointURL:     "https://s3.de.io.cloud.ovh.net",
			}), emptyVault())

			Expect(openfgaValues(values)["enabled"]).To(BeFalse())
		})

		It("keeps the gateway configuration next to the backup", func() {
			values := pcapps.OpenFgaPcAppsValues(configWithBackups(&files.OpenFgaConfig{
				Expose: &files.OpenFgaExposeConfig{Enabled: true, Host: "openfga.1.cs.example.com"},
			}, &files.OpenfgaBackupsConfig{
				Enabled:         true,
				DestinationPath: "s3://backup-openfga-dev",
				EndpointURL:     "https://s3.de.io.cloud.ovh.net",
			}), emptyVault())

			Expect(backupValues(values)["enabled"]).To(BeTrue())
			gateway := openfgaValues(values)["valuesObject"].(files.ChartValues)["gateway"].(files.ChartValues)
			Expect(gateway["host"]).To(Equal("openfga.1.cs.example.com"))
		})
	})
})
