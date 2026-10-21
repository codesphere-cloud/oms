// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package secrets

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var tokenMethods = map[string]jwt.SigningMethod{
	"ES256": jwt.SigningMethodES256,
	"ES384": jwt.SigningMethodES384,
	"ES512": jwt.SigningMethodES512,
	"RS512": jwt.SigningMethodRS512,
}

func parseTokenPrivateKey(content string) (crypto.PrivateKey, error) {
	block, _ := pem.Decode([]byte(content))
	if block == nil {
		return nil, fmt.Errorf("invalid private key PEM")
	}

	switch block.Type {
	case "PRIVATE KEY":
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse PKCS8 token private key: %w", err)
		}

		return key, nil
	case "RSA PRIVATE KEY":
		key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse RSA token private key: %w", err)
		}

		return key, nil
	case "EC PRIVATE KEY":
		key, err := x509.ParseECPrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse EC token private key: %w", err)
		}

		return key, nil
	default:
		return nil, fmt.Errorf("unsupported private key PEM type %q", block.Type)
	}
}

func algorithmForKey(key crypto.PrivateKey) (string, error) {
	switch k := key.(type) {
	case *rsa.PrivateKey:
		return "RS512", nil
	case *ecdsa.PrivateKey:
		switch k.Curve {
		case elliptic.P256():
			return "ES256", nil
		case elliptic.P384():
			return "ES384", nil
		case elliptic.P521():
			return "ES512", nil
		}
	}

	return "", fmt.Errorf("unsupported token private key type or curve")
}

func tokenAlgorithm(vault *files.InstallVault, configured []string) (string, error) {
	if len(configured) > 0 && configured[0] != "" {
		if tokenMethods[configured[0]] == nil {
			return "", fmt.Errorf("unsupported token algorithm %q", configured[0])
		}

		return configured[0], nil
	}

	entry := vault.GetSecret(files.SecretTokenPrivateKey)
	if entry == nil {
		return "ES256", nil
	}

	if entry.File == nil {
		return "", fmt.Errorf("tokenPrivateKey has no file content")
	}

	key, err := parseTokenPrivateKey(entry.File.Content)
	if err != nil {
		return "", fmt.Errorf("parse tokenPrivateKey: %w", err)
	}

	return algorithmForKey(key)
}

func generateTokenKeyPair(algorithm string) (string, string, error) {
	if algorithm == "RS512" {
		return generateRSAPKCS8KeyPair(4096)
	}

	var curve elliptic.Curve

	switch algorithm {
	case "ES256":
		curve = elliptic.P256()
	case "ES384":
		curve = elliptic.P384()
	case "ES512":
		curve = elliptic.P521()
	default:
		return "", "", fmt.Errorf("unsupported token algorithm %q", algorithm)
	}

	key, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("generate EC token key: %w", err)
	}

	privateDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", "", fmt.Errorf("marshal token private key: %w", err)
	}

	publicDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return "", "", fmt.Errorf("marshal token public key: %w", err)
	}

	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER})),
		string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER})), nil
}

func validateTokenKeys(vault *files.InstallVault, algorithm string) error {
	private := vault.GetSecret(files.SecretTokenPrivateKey)
	if private == nil || private.File == nil {
		return fmt.Errorf("tokenPrivateKey not found in vault; call EnsureAuthKeys first")
	}

	key, err := parseTokenPrivateKey(private.File.Content)
	if err != nil {
		return fmt.Errorf("parse tokenPrivateKey: %w", err)
	}

	keyAlgorithm, err := algorithmForKey(key)
	if err != nil {
		return err
	}

	if keyAlgorithm != algorithm {
		return fmt.Errorf("configured token algorithm %s does not match existing %s token key", algorithm, keyAlgorithm)
	}

	public := vault.GetSecret(files.SecretTokenPublicKey)
	if public == nil || public.File == nil {
		return fmt.Errorf("tokenPublicKey not found in vault")
	}

	block, _ := pem.Decode([]byte(public.File.Content))
	if block == nil || block.Type != "PUBLIC KEY" {
		return fmt.Errorf("invalid tokenPublicKey PEM")
	}

	publicKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("parse tokenPublicKey: %w", err)
	}

	derived, err := x509.MarshalPKIXPublicKey(key.(crypto.Signer).Public())
	if err != nil {
		return fmt.Errorf("marshal derived token public key: %w", err)
	}

	actual, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil || string(actual) != string(derived) {
		return fmt.Errorf("tokenPublicKey does not match tokenPrivateKey")
	}

	return nil
}

// EnsureServiceAccountTokens signs a fresh set of service tokens on every call.
func EnsureServiceAccountTokens(vault *files.InstallVault, configured ...string) error {
	algorithm, err := tokenAlgorithm(vault, configured)
	if err != nil {
		return err
	}

	if err := validateTokenKeys(vault, algorithm); err != nil {
		return err
	}

	private, err := parseTokenPrivateKey(vault.GetSecret(files.SecretTokenPrivateKey).File.Content)
	if err != nil {
		return err
	}

	now := time.Now()
	for _, su := range codesphereServiceUsers {
		claims := jwt.MapClaims{
			"userId": -1, "firstName": su.serviceID, "lastName": "", "avatarId": "",
			"serviceId": su.serviceID, "authenticationMethod": "service", "email": su.email,
			"exp": now.Add(serviceAccountTokenExpiry).Unix(), "iat": now.Unix(),
		}

		token, err := jwt.NewWithClaims(tokenMethods[algorithm], claims).SignedString(private)
		if err != nil {
			return fmt.Errorf("sign token for %s: %w", su.tokenName, err)
		}

		vault.SetSecret(files.SecretEntry{Name: su.tokenName, Fields: &files.SecretFields{Password: token}})
	}

	return nil
}
