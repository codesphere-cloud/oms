// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"crypto/x509"
	"encoding/pem"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/crypto/ssh"
)

func TestInstaller(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Installer Suite")
}

var _ = Describe("GenerateSSHKeyPair", func() {
	It("generates a valid SSH key pair", func() {
		privKey, pubKey, err := GenerateSSHKeyPair()
		Expect(err).NotTo(HaveOccurred())

		Expect(privKey).To(HavePrefix("-----BEGIN RSA PRIVATE KEY-----"))

		_, _, _, _, err = ssh.ParseAuthorizedKey([]byte(pubKey))
		Expect(err).NotTo(HaveOccurred())

		block, _ := pem.Decode([]byte(privKey))
		Expect(block).NotTo(BeNil())
		Expect(block.Type).To(Equal("RSA PRIVATE KEY"))
	})
})

var _ = Describe("GenerateCA", func() {
	It("generates a valid CA certificate", func() {
		keyPEM, certPEM, err := GenerateCA("Test CA", "DE", "Berlin", "TestOrg")
		Expect(err).NotTo(HaveOccurred())

		Expect(keyPEM).To(HavePrefix("-----BEGIN RSA PRIVATE KEY-----"))
		Expect(certPEM).To(HavePrefix("-----BEGIN CERTIFICATE-----"))

		certBlock, _ := pem.Decode([]byte(certPEM))
		Expect(certBlock).NotTo(BeNil())

		cert, err := x509.ParseCertificate(certBlock.Bytes)
		Expect(err).NotTo(HaveOccurred())

		Expect(cert.IsCA).To(BeTrue())
		Expect(cert.Subject.CommonName).To(Equal("Test CA"))
		Expect(cert.Subject.Country).To(ContainElement("DE"))
		Expect(cert.Subject.Locality).To(ContainElement("Berlin"))
		Expect(cert.Subject.Organization).To(ContainElement("TestOrg"))
	})
})

var _ = Describe("GenerateServerCertificate", func() {
	It("generates a valid server certificate", func() {
		caKeyPEM, caCertPEM, err := GenerateCA("Test CA", "DE", "Berlin", "TestOrg")
		Expect(err).NotTo(HaveOccurred())

		serverKeyPEM, serverCertPEM, err := GenerateServerCertificate(
			caKeyPEM,
			caCertPEM,
			"test-server",
			[]string{"192.168.1.1", "10.0.0.1"},
		)
		Expect(err).NotTo(HaveOccurred())

		Expect(serverKeyPEM).To(HavePrefix("-----BEGIN RSA PRIVATE KEY-----"))
		Expect(serverCertPEM).To(HavePrefix("-----BEGIN CERTIFICATE-----"))

		certBlock, _ := pem.Decode([]byte(serverCertPEM))
		Expect(certBlock).NotTo(BeNil())

		cert, err := x509.ParseCertificate(certBlock.Bytes)
		Expect(err).NotTo(HaveOccurred())

		Expect(cert.Subject.CommonName).To(Equal("test-server"))
		Expect(cert.IPAddresses).To(HaveLen(2))
	})
})

var _ = Describe("GenerateECDSAKeyPair", func() {
	It("generates a valid ECDSA key pair", func() {
		privKey, pubKey, err := GenerateECDSAKeyPair()
		Expect(err).NotTo(HaveOccurred())

		Expect(privKey).To(HavePrefix("-----BEGIN EC PRIVATE KEY-----"))
		Expect(pubKey).To(HavePrefix("-----BEGIN PUBLIC KEY-----"))

		privBlock, _ := pem.Decode([]byte(privKey))
		Expect(privBlock).NotTo(BeNil())
		Expect(privBlock.Type).To(Equal("EC PRIVATE KEY"))

		pubBlock, _ := pem.Decode([]byte(pubKey))
		Expect(pubBlock).NotTo(BeNil())
		Expect(pubBlock.Type).To(Equal("PUBLIC KEY"))
	})
})

var _ = Describe("GeneratePassword", func() {
	It("generates passwords of the correct length", func() {
		password := GeneratePassword(20)
		Expect(password).To(HaveLen(20))
	})

	It("generates different passwords", func() {
		password1 := GeneratePassword(20)
		password2 := GeneratePassword(20)
		Expect(password1).NotTo(Equal(password2))
	})
})
