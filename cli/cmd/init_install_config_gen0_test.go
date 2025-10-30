// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestGenerateSSHKeyPair(t *testing.T) {
	privKey, pubKey, err := generateSSHKeyPair()
	if err != nil {
		t.Fatalf("generateSSHKeyPair failed: %v", err)
	}

	if !strings.HasPrefix(privKey, "-----BEGIN RSA PRIVATE KEY-----") {
		t.Error("Private key should be in PEM format")
	}

	_, _, _, _, err = ssh.ParseAuthorizedKey([]byte(pubKey))
	if err != nil {
		t.Errorf("Failed to parse SSH public key: %v", err)
	}

	block, _ := pem.Decode([]byte(privKey))
	if block == nil {
		t.Fatal("Failed to decode private key PEM")
	}
	if block.Type != "RSA PRIVATE KEY" {
		t.Errorf("Expected RSA PRIVATE KEY, got %s", block.Type)
	}
}

func TestGenerateCA(t *testing.T) {
	keyPEM, certPEM, err := generateCA("Test CA", "DE", "Berlin", "TestOrg")
	if err != nil {
		t.Fatalf("generateCA failed: %v", err)
	}

	if !strings.HasPrefix(keyPEM, "-----BEGIN RSA PRIVATE KEY-----") {
		t.Error("CA key should be in PEM format")
	}

	if !strings.HasPrefix(certPEM, "-----BEGIN CERTIFICATE-----") {
		t.Error("CA cert should be in PEM format")
	}

	certBlock, _ := pem.Decode([]byte(certPEM))
	if certBlock == nil {
		t.Fatal("Failed to decode certificate PEM")
	}

	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		t.Fatalf("Failed to parse certificate: %v", err)
	}

	if !cert.IsCA {
		t.Error("Certificate should be a CA")
	}
	if cert.Subject.CommonName != "Test CA" {
		t.Errorf("Expected CN 'Test CA', got '%s'", cert.Subject.CommonName)
	}
	if len(cert.Subject.Country) == 0 || cert.Subject.Country[0] != "DE" {
		t.Error("Expected country DE")
	}
	if len(cert.Subject.Locality) == 0 || cert.Subject.Locality[0] != "Berlin" {
		t.Error("Expected locality Berlin")
	}
	if len(cert.Subject.Organization) == 0 || cert.Subject.Organization[0] != "TestOrg" {
		t.Error("Expected organization TestOrg")
	}
}

func TestGenerateServerCertificate(t *testing.T) {
	caKeyPEM, caCertPEM, err := generateCA("Test CA", "DE", "Berlin", "TestOrg")
	if err != nil {
		t.Fatalf("Failed to generate CA: %v", err)
	}

	serverKeyPEM, serverCertPEM, err := generateServerCertificate(
		caKeyPEM,
		caCertPEM,
		"test-server",
		[]string{"192.168.1.1", "10.0.0.1"},
	)
	if err != nil {
		t.Fatalf("generateServerCertificate failed: %v", err)
	}

	if !strings.HasPrefix(serverKeyPEM, "-----BEGIN RSA PRIVATE KEY-----") {
		t.Error("Server key should be in PEM format")
	}

	if !strings.HasPrefix(serverCertPEM, "-----BEGIN CERTIFICATE-----") {
		t.Error("Server cert should be in PEM format")
	}

	certBlock, _ := pem.Decode([]byte(serverCertPEM))
	if certBlock == nil {
		t.Fatal("Failed to decode server certificate PEM")
	}

	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		t.Fatalf("Failed to parse server certificate: %v", err)
	}

	if cert.Subject.CommonName != "test-server" {
		t.Errorf("Expected CN 'test-server', got '%s'", cert.Subject.CommonName)
	}

	if len(cert.IPAddresses) != 2 {
		t.Errorf("Expected 2 IP addresses, got %d", len(cert.IPAddresses))
	}
}

func TestGenerateECDSAKeyPair(t *testing.T) {
	privKey, pubKey, err := generateECDSAKeyPair()
	if err != nil {
		t.Fatalf("generateECDSAKeyPair failed: %v", err)
	}

	if !strings.HasPrefix(privKey, "-----BEGIN EC PRIVATE KEY-----") {
		t.Error("Private key should be in EC PEM format")
	}

	if !strings.HasPrefix(pubKey, "-----BEGIN PUBLIC KEY-----") {
		t.Error("Public key should be in PEM format")
	}

	privBlock, _ := pem.Decode([]byte(privKey))
	if privBlock == nil {
		t.Fatal("Failed to decode private key PEM")
	}
	if privBlock.Type != "EC PRIVATE KEY" {
		t.Errorf("Expected EC PRIVATE KEY, got %s", privBlock.Type)
	}

	pubBlock, _ := pem.Decode([]byte(pubKey))
	if pubBlock == nil {
		t.Fatal("Failed to decode public key PEM")
	}
	if pubBlock.Type != "PUBLIC KEY" {
		t.Errorf("Expected PUBLIC KEY, got %s", pubBlock.Type)
	}
}

func TestGeneratePassword(t *testing.T) {
	password := generatePassword(20)

	if len(password) != 20 {
		t.Errorf("Expected password length 20, got %d", len(password))
	}

	password2 := generatePassword(20)
	if password == password2 {
		t.Error("Generated passwords should be different")
	}
}

func TestParseIP(t *testing.T) {
	tests := []struct {
		name    string
		ip      string
		wantNil bool
	}{
		{"valid IPv4", "192.168.1.1", false},
		{"valid IPv6", "2001:db8::1", false},
		{"invalid IP", "not-an-ip", true},
		{"empty string", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseIP(tt.ip)
			if tt.wantNil && result != nil {
				t.Errorf("parseIP(%s) should return nil, got %v", tt.ip, result)
			}
			if !tt.wantNil && result == nil {
				t.Errorf("parseIP(%s) should not return nil", tt.ip)
			}
		})
	}
}

func TestParseCAKeyAndCert(t *testing.T) {
	caKeyPEM, caCertPEM, err := generateCA("Test CA", "DE", "Berlin", "TestOrg")
	if err != nil {
		t.Fatalf("Failed to generate CA: %v", err)
	}

	caKey, caCert, err := parseCAKeyAndCert(caKeyPEM, caCertPEM)
	if err != nil {
		t.Fatalf("parseCAKeyAndCert failed: %v", err)
	}
	if caKey == nil {
		t.Error("CA key should not be nil")
	}
	if caCert == nil {
		t.Error("CA cert should not be nil")
	}

	_, _, err = parseCAKeyAndCert("invalid-pem", caCertPEM)
	if err == nil {
		t.Error("Expected error for invalid key PEM")
	}

	_, _, err = parseCAKeyAndCert(caKeyPEM, "invalid-pem")
	if err == nil {
		t.Error("Expected error for invalid cert PEM")
	}
}

func TestEncodePEMKey(t *testing.T) {
	tests := []struct {
		name    string
		keyType string
		wantErr bool
	}{
		{"RSA key", "RSA", false},
		{"EC key", "EC", false},
		{"invalid type", "INVALID", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var key interface{}
			var err error

			switch tt.keyType {
			case "RSA":
				key, err = generateTestRSAKey()
			case "EC":
				key, err = generateTestECKey()
			default:
				key = "invalid-key"
			}

			if err != nil && !tt.wantErr {
				t.Fatalf("Failed to generate test key: %v", err)
			}

			result, err := encodePEMKey(key, tt.keyType)
			if (err != nil) != tt.wantErr {
				t.Errorf("encodePEMKey() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && !strings.Contains(result, "-----BEGIN") {
				t.Error("Result should be in PEM format")
			}
		})
	}
}

func TestEncodePEMCert(t *testing.T) {
	_, certPEM, err := generateCA("Test CA", "DE", "Berlin", "TestOrg")
	if err != nil {
		t.Fatalf("Failed to generate CA: %v", err)
	}

	certBlock, _ := pem.Decode([]byte(certPEM))
	if certBlock == nil {
		t.Fatal("Failed to decode certificate PEM")
	}

	result := encodePEMCert(certBlock.Bytes)

	if !strings.HasPrefix(result, "-----BEGIN CERTIFICATE-----") {
		t.Error("Result should be in PEM format")
	}
}

func TestGenerateSecrets(t *testing.T) {
	cmd := &InitInstallConfigCmd{
		Opts: &InitInstallConfigOpts{
			PostgresMode:        "install",
			PostgresPrimaryHost: "pg-primary",
			PostgresPrimaryIP:   "10.0.0.1",
			PostgresReplicaIP:   "10.0.0.2",
			PostgresReplicaName: "replica1",
		},
	}

	secrets, err := cmd.generateSecrets()
	if err != nil {
		t.Fatalf("generateSecrets failed: %v", err)
	}

	if secrets.CephSSHPrivateKey == "" {
		t.Error("Ceph SSH private key should not be empty")
	}
	if secrets.CephSSHPublicKey == "" {
		t.Error("Ceph SSH public key should not be empty")
	}

	if secrets.IngressCAKey == "" {
		t.Error("Ingress CA key should not be empty")
	}
	if secrets.IngressCACert == "" {
		t.Error("Ingress CA cert should not be empty")
	}

	if secrets.DomainAuthPrivateKey == "" {
		t.Error("Domain auth private key should not be empty")
	}
	if secrets.DomainAuthPublicKey == "" {
		t.Error("Domain auth public key should not be empty")
	}

	if secrets.PostgresCACert == "" {
		t.Error("PostgreSQL CA cert should not be empty")
	}
	if secrets.PostgresPrimaryCert == "" {
		t.Error("PostgreSQL primary cert should not be empty")
	}
	if secrets.PostgresReplicaCert == "" {
		t.Error("PostgreSQL replica cert should not be empty")
	}

	if secrets.PostgresAdminPassword == "" {
		t.Error("PostgreSQL admin password should not be empty")
	}
	if secrets.PostgresReplicaPassword == "" {
		t.Error("PostgreSQL replica password should not be empty")
	}

	expectedServices := []string{"auth", "deployment", "ide", "marketplace", "payment", "public_api", "team", "workspace"}
	for _, service := range expectedServices {
		if _, ok := secrets.PostgresUserPasswords[service]; !ok {
			t.Errorf("Missing password for service: %s", service)
		}
	}
}

func generateTestRSAKey() (interface{}, error) {
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	return rsaKey, nil
}

func generateTestECKey() (interface{}, error) {
	return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
}
