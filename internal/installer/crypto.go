// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
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

func GenerateCA(cn, country, locality, org string) (keyPEM string, certPEM string, err error) {
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", err
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", "", err
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
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

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &caKey.PublicKey, caKey)
	if err != nil {
		return "", "", err
	}

	keyPEM, err = encodePEMKey(caKey, "RSA")
	if err != nil {
		return "", "", err
	}

	return keyPEM, encodePEMCert(certDER), nil
}

func GenerateServerCertificate(caKeyPEM, caCertPEM, cn string, ipAddresses []string) (keyPEM string, certPEM string, err error) {
	caKey, caCert, err := parseCAKeyAndCert(caKeyPEM, caCertPEM)
	if err != nil {
		return "", "", err
	}

	serverKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return "", "", err
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", "", err
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
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
			template.IPAddresses = append(template.IPAddresses, parsed)
		}
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		return "", "", err
	}

	keyPEM, err = encodePEMKey(serverKey, "RSA")
	if err != nil {
		return "", "", err
	}

	return keyPEM, encodePEMCert(certDER), nil
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

func GeneratePassword(length int) string {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
	}
	return base64.StdEncoding.EncodeToString(bytes)[:length]
}

func parseCAKeyAndCert(caKeyPEM, caCertPEM string) (*rsa.PrivateKey, *x509.Certificate, error) {
	caKeyBlock, _ := pem.Decode([]byte(caKeyPEM))
	if caKeyBlock == nil {
		return nil, nil, fmt.Errorf("failed to decode CA key PEM")
	}
	caKey, err := x509.ParsePKCS1PrivateKey(caKeyBlock.Bytes)
	if err != nil {
		return nil, nil, err
	}

	caCertBlock, _ := pem.Decode([]byte(caCertPEM))
	if caCertBlock == nil {
		return nil, nil, fmt.Errorf("failed to decode CA cert PEM")
	}
	caCert, err := x509.ParseCertificate(caCertBlock.Bytes)
	if err != nil {
		return nil, nil, err
	}

	return caKey, caCert, nil
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

func encodePEMCert(certDER []byte) string {
	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	}))
}
