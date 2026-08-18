// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer"
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
		Expect(installer.OpenFgaPcAppsValues(configWith(nil, ""), emptyVault())).To(BeNil())
	})

	It("disables the application for a data center that uses a remote OpenFGA", func() {
		deploy := false
		values := installer.OpenFgaPcAppsValues(configWith(&files.OpenFgaConfig{
			Deploy: &deploy,
			APIURL: "https://openfga.1.cs.example.com",
		}, files.CertIssuerTypeACME), emptyVault())

		fga := openfgaValues(values)
		Expect(fga["enabled"]).To(BeFalse())
		// Nothing to expose, so the gateway is not configured at all.
		Expect(fga).NotTo(HaveKey("valuesObject"))
	})

	It("defaults to deploying when only the exposure is configured", func() {
		values := installer.OpenFgaPcAppsValues(configWith(&files.OpenFgaConfig{
			Expose: &files.OpenFgaExposeConfig{Enabled: true, Host: "openfga.1.cs.example.com"},
		}, files.CertIssuerTypeACME), emptyVault())

		fga := openfgaValues(values)
		Expect(fga["enabled"]).To(BeTrue())

		gateway := fga["valuesObject"].(files.ChartValues)["gateway"].(files.ChartValues)
		Expect(gateway["enabled"]).To(BeTrue())
		Expect(gateway["host"]).To(Equal("openfga.1.cs.example.com"))
	})

	It("issues the gateway certificate with the installation's cert issuer", func() {
		values := installer.OpenFgaPcAppsValues(configWith(&files.OpenFgaConfig{
			Expose: &files.OpenFgaExposeConfig{Enabled: true, Host: "openfga.1.cs.example.com"},
		}, files.CertIssuerTypeACME), emptyVault())

		gateway := openfgaValues(values)["valuesObject"].(files.ChartValues)["gateway"].(files.ChartValues)
		issuerRef := gateway["tls"].(files.ChartValues)["certificate"].(files.ChartValues)["issuerRef"].(files.ChartValues)
		Expect(issuerRef["name"]).To(Equal("acme"))
	})

	It("falls back to the self-signed issuer when no cert issuer is configured", func() {
		values := installer.OpenFgaPcAppsValues(configWith(&files.OpenFgaConfig{
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

		values := installer.OpenFgaPcAppsValues(config, emptyVault())

		gateway := openfgaValues(values)["valuesObject"].(files.ChartValues)["gateway"].(files.ChartValues)
		issuerRef := gateway["tls"].(files.ChartValues)["certificate"].(files.ChartValues)["issuerRef"].(files.ChartValues)
		Expect(issuerRef["name"]).To(Equal("self-signed"))
	})

	It("leaves OpenFGA unauthenticated while the vault has no preshared key", func() {
		values := installer.OpenFgaPcAppsValues(configWith(&files.OpenFgaConfig{}, ""), emptyVault())

		Expect(authnValues(values)).To(BeNil())
	})

	It("requires the preshared key once the vault holds one", func() {
		values := installer.OpenFgaPcAppsValues(configWith(nil, ""), vaultWithPresharedKey())

		authn := authnValues(values)
		Expect(authn["method"]).To(Equal("preshared"))
		Expect(authn["preshared"].(files.ChartValues)["keysSecret"]).To(Equal("openfga-preshared-keys"))
	})

	It("keeps the gateway configuration when authentication is derived as well", func() {
		values := installer.OpenFgaPcAppsValues(configWith(&files.OpenFgaConfig{
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

		Expect(installer.OpenFgaPcAppsValues(configWith(nil, ""), vault)).To(BeNil())
	})
})
