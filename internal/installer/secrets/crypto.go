// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package secrets

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"time"

	"golang.org/x/crypto/ssh"
)

func GenerateSSHKeyPair() (privateKey string, publicKey string, err error) {
	rsaKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return "", "", err
	}

	privKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(rsaKey),
	})

	sshPubKey, err := ssh.NewPublicKey(&rsaKey.PublicKey)
	if err != nil {
		return "", "", err
	}
	pubKeySSH := string(ssh.MarshalAuthorizedKey(sshPubKey))

	return string(privKeyPEM), pubKeySSH, nil
}

func GenerateECDSAKeyPair() (privateKey string, publicKey string, err error) {
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}

	privKeyPEM, err := encodePEMKey(ecKey, "EC")
	if err != nil {
		return "", "", err
	}

	pubBytes, err := x509.MarshalPKIXPublicKey(&ecKey.PublicKey)
	if err != nil {
		return "", "", err
	}
	pubKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubBytes,
	})

	return privKeyPEM, string(pubKeyPEM), nil
}

// GenerateCA generates a self-signed RSA-2048 CA certificate.
func GenerateCA(cn, country, locality, org string) (keyPEM, certPEM string, err error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", err
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", "", err
	}

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   cn,
			Country:      []string{country},
			Locality:     []string{locality},
			Organization: []string{org},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(3, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return "", "", err
	}

	keyPEM, err = encodePEMKey(key, "RSA")
	if err != nil {
		return "", "", err
	}
	certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}))
	return keyPEM, certPEM, nil
}

// GenerateServerCertificate generates an RSA-4096 server certificate signed by the given CA.
// The CA private key may be in either PKCS8 ("PRIVATE KEY") or PKCS1 ("RSA PRIVATE KEY") PEM
// format to support legacy vaults that were created before the PKCS8 migration.
func GenerateServerCertificate(caKeyPEM, caCertPEM, cn string, ipAddresses []string) (keyPEM, certPEM string, err error) {
	caKey, err := ParseRSAPrivateKey(caKeyPEM)
	if err != nil {
		return "", "", fmt.Errorf("parse CA key: %w", err)
	}

	caCertBlock, _ := pem.Decode([]byte(caCertPEM))
	if caCertBlock == nil {
		return "", "", fmt.Errorf("decode CA cert PEM: empty block")
	}
	caCert, err := x509.ParseCertificate(caCertBlock.Bytes)
	if err != nil {
		return "", "", fmt.Errorf("parse CA cert: %w", err)
	}

	serverKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return "", "", err
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", "", err
	}

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   cn,
			Organization: []string{"Codesphere"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(2, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	for _, ip := range ipAddresses {
		if parsed := net.ParseIP(ip); parsed != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, parsed)
		}
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		return "", "", err
	}

	keyPEM, err = encodePEMKey(serverKey, "RSA")
	if err != nil {
		return "", "", err
	}
	certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}))
	return keyPEM, certPEM, nil
}

func encodePEMKey(key interface{}, keyType string) (string, error) {
	var pemBytes []byte

	switch keyType {
	case "RSA":
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return "", fmt.Errorf("invalid RSA key type")
		}
		pemBytes = pem.EncodeToMemory(&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(rsaKey),
		})
	case "EC":
		ecKey, ok := key.(*ecdsa.PrivateKey)
		if !ok {
			return "", fmt.Errorf("invalid EC key type")
		}
		ecBytes, err := x509.MarshalECPrivateKey(ecKey)
		if err != nil {
			return "", err
		}
		pemBytes = pem.EncodeToMemory(&pem.Block{
			Type:  "EC PRIVATE KEY",
			Bytes: ecBytes,
		})
	default:
		return "", fmt.Errorf("unsupported key type: %s", keyType)
	}

	return string(pemBytes), nil
}

// ParseRSAPrivateKey decodes a PEM block and parses an RSA private key in either
// PKCS8 or legacy PKCS1 format.
func ParseRSAPrivateKey(keyPEM string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(keyPEM))
	if block == nil {
		return nil, fmt.Errorf("empty PEM block")
	}
	switch block.Type {
	case "PRIVATE KEY":
		raw, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse PKCS8: %w", err)
		}
		key, ok := raw.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("PKCS8 key is not RSA")
		}
		return key, nil
	case "RSA PRIVATE KEY":
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	default:
		return nil, fmt.Errorf("unsupported PEM type %q", block.Type)
	}
}

func GeneratePassword(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("read random bytes for password: %w", err)
	}
	return base64.StdEncoding.EncodeToString(b)[:length], nil
}

// ValidateCertKeyPair verifies that a PEM-encoded certificate's public key matches a PEM-encoded private key.
func ValidateCertKeyPair(certPEM, keyPEM string) error {
	_, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	return err
}
