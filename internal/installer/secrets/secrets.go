// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package secrets generates and defaults all secrets required by the private-cloud
// Helm chart that are not derived from the installer configuration.
package secrets

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// postgresServices is the ordered list of services that each get a dedicated postgres user and password.
var postgresServices = []string{
	"auth", "deployment", "ide", "marketplace", "payment", "public_api", "team", "workspace",
	"usageAggregationRefresher", "usageAggregationReader",
}

// EnsureSecrets generates all secrets required by the Helm chart that are not derived from
// the installer configuration. Each sub-function is idempotent; the whole call is safe to
// repeat on an already-populated vault.
func EnsureSecrets(vault *files.InstallVault, config *files.RootConfig) error {
	if err := EnsureAuthKeys(vault); err != nil {
		return fmt.Errorf("ensure auth keys: %w", err)
	}
	if err := EnsureServiceAccountTokens(vault); err != nil {
		return fmt.Errorf("ensure service account tokens: %w", err)
	}
	if err := EnsureIngressCA(vault, &config.Cluster); err != nil {
		return fmt.Errorf("ensure ingress CA: %w", err)
	}
	if err := EnsureCephSSHKeys(vault, &config.Ceph); err != nil {
		return fmt.Errorf("ensure ceph SSH keys: %w", err)
	}
	if config.Postgres.Primary != nil {
		if err := EnsurePostgresSecrets(vault, &config.Postgres); err != nil {
			return fmt.Errorf("ensure postgres secrets: %w", err)
		}
	}
	if err := EnsureMounterHmacSecret(vault); err != nil {
		return fmt.Errorf("ensure hmac secret: %w", err)
	}
	if err := EnsureDefaultSecrets(vault); err != nil {
		return fmt.Errorf("ensure default secrets: %w", err)
	}
	return nil
}

// serviceUser holds the fixed claims for a Codesphere internal service account JWT.
type serviceUser struct {
	tokenName string
	serviceID string
	email     string
}

// serviceAccountTokenExpiry is the lifetime of generated service account JWTs.
const serviceAccountTokenExpiry = 10 * 365 * 24 * time.Hour

// codesphereServiceUsers lists all internal service accounts and the vault key for their token.
var codesphereServiceUsers = []serviceUser{
	{tokenName: "authServiceUserToken", serviceID: "auth-service", email: "auth.service@codesphere.com"},
	{tokenName: "paymentServiceUserToken", serviceID: "payment-service", email: "payment.service@codesphere.com"},
	{tokenName: "publicApiServiceUserToken", serviceID: "public-api-service", email: "public.api.service@codesphere.com"},
	{tokenName: "deploymentServiceUserToken", serviceID: "deployment-service", email: "deployment.service@codesphere.com"},
	{tokenName: "marketplaceServiceUserToken", serviceID: "marketplace-service", email: "marketplace.service@codesphere.com"},
	{tokenName: "errorPageServerUserToken", serviceID: "error-page-server", email: "error.page.servere@codesphere.com"},
	{tokenName: "userDeletionCronJobUserToken", serviceID: "userdeletion-cronjob", email: "userDeletion.service@codesphere.com"},
	{tokenName: "workspaceServiceUserToken", serviceID: "workspace-service", email: "workspace.service@codesphere.com"},
	{tokenName: "workspaceProxyUserToken", serviceID: "workspace-proxy", email: "workspace.proxy@codesphere.com"},
	{tokenName: "ideServiceUserToken", serviceID: "ide-service", email: "ide.service@codesphere.com"},
}

// EnsureServiceAccountTokens signs RS512 JWTs for all Codesphere internal service accounts
// and stores them in vault. Requires tokenPrivateKey to already be present (call EnsureAuthKeys
// first). Idempotent: skips if authServiceUserToken already exists.
func EnsureServiceAccountTokens(vault *files.InstallVault) error {
	if vault.GetSecret("authServiceUserToken") != nil {
		return nil
	}

	privKeyEntry := vault.GetSecret("tokenPrivateKey")
	if privKeyEntry == nil || privKeyEntry.File == nil {
		return fmt.Errorf("tokenPrivateKey not found in vault; call EnsureAuthKeys first")
	}

	block, _ := pem.Decode([]byte(privKeyEntry.File.Content))
	if block == nil {
		return fmt.Errorf("failed to PEM-decode tokenPrivateKey")
	}
	rawKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("parse tokenPrivateKey: %w", err)
	}
	rsaKey, ok := rawKey.(*rsa.PrivateKey)
	if !ok {
		return fmt.Errorf("tokenPrivateKey is not an RSA key")
	}

	expiresAt := time.Now().Add(serviceAccountTokenExpiry)

	for _, su := range codesphereServiceUsers {
		claims := jwt.MapClaims{
			"userId":               -1,
			"firstName":            su.serviceID,
			"lastName":             "",
			"avatarId":             "",
			"serviceId":            su.serviceID,
			"authenticationMethod": "service",
			"email":                su.email,
			"exp":                  expiresAt.Unix(),
			"iat":                  time.Now().Unix(),
		}
		token, err := jwt.NewWithClaims(jwt.SigningMethodRS512, claims).SignedString(rsaKey)
		if err != nil {
			return fmt.Errorf("sign token for %s: %w", su.tokenName, err)
		}
		vault.SetSecret(files.SecretEntry{
			Name:   su.tokenName,
			Fields: &files.SecretFields{Password: token},
		})
	}
	return nil
}

// EnsureAuthKeys generates RSA-4096 token keys and EC P-256 domain-auth keys in
// PKCS8/SPKI PEM format if not already present. Each key pair is checked independently.
func EnsureAuthKeys(vault *files.InstallVault) error {
	if vault.GetSecret("tokenPrivateKey") == nil {
		tokenPriv, tokenPub, err := generateRSAPKCS8KeyPair(4096)
		if err != nil {
			return fmt.Errorf("generate token key pair: %w", err)
		}
		vault.SetSecret(files.SecretEntry{Name: "tokenPrivateKey", File: &files.SecretFile{Name: "key.pem", Content: tokenPriv}})
		vault.SetSecret(files.SecretEntry{Name: "tokenPublicKey", File: &files.SecretFile{Name: "key.pub", Content: tokenPub}})
	}

	if vault.GetSecret("domainAuthPrivateKey") == nil {
		domainPriv, domainPub, err := generateECP256PKCS8KeyPair()
		if err != nil {
			return fmt.Errorf("generate domain auth key pair: %w", err)
		}
		vault.SetSecret(files.SecretEntry{Name: "domainAuthPrivateKey", File: &files.SecretFile{Name: "key.pem", Content: domainPriv}})
		vault.SetSecret(files.SecretEntry{Name: "domainAuthPublicKey", File: &files.SecretFile{Name: "key.pub", Content: domainPub}})
	}

	return nil
}

// EnsureMounterHmacSecret migrates the legacy 'hmac-secret' to 'mounterHmacSecret'
// or creates a new 64-character hex secret if neither exists. Idempotent.
func EnsureMounterHmacSecret(vault *files.InstallVault) error {
	if vault.GetSecret("mounterHmacSecret") != nil {
		return nil
	}

	// Migrate from legacy name if present.
	if old := vault.GetSecret("hmac-secret"); old != nil && old.Fields != nil {
		vault.SetSecret(files.SecretEntry{
			Name:   "mounterHmacSecret",
			Fields: &files.SecretFields{Password: old.Fields.Password},
		})
		return nil
	}

	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return fmt.Errorf("read random bytes: %w", err)
	}
	vault.SetSecret(files.SecretEntry{
		Name:   "mounterHmacSecret",
		Fields: &files.SecretFields{Password: hex.EncodeToString(b)},
	})
	return nil
}

// EnsureNixSigningKeys generates an Ed25519 signing key pair for nix-cache in the
// format "host:hexKey" if not already present. Idempotent.
func EnsureNixSigningKeys(vault *files.InstallVault, host string) error {
	if vault.GetSecret("privNixSigningKey") != nil {
		return nil
	}

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("generate ed25519 key pair: %w", err)
	}
	vault.SetSecret(files.SecretEntry{
		Name:   "privNixSigningKey",
		Fields: &files.SecretFields{Password: fmt.Sprintf("%s:%s", host, hex.EncodeToString(priv.Seed()))},
	})
	vault.SetSecret(files.SecretEntry{
		Name:   "pubNixSigningKey",
		Fields: &files.SecretFields{Password: fmt.Sprintf("%s:%s", host, hex.EncodeToString(pub))},
	})
	return nil
}

// EnsureDefaultSecrets sets dummy defaults for all Helm chart secrets not managed by
// the installer config. Always overwrites digitalOceanApiToken; all others are only
// set when absent.
func EnsureDefaultSecrets(vault *files.InstallVault) error {
	// Always overwrite — not used in private cloud but must not be empty.
	setPassword(vault, "digitalOceanApiToken", "dummy")

	for _, name := range optionalPasswordSecrets {
		setPasswordIfAbsent(vault, name, "dummy")
	}

	// Requires a valid AES key: base64(hex(16 random bytes)).
	if vault.GetSecret("mongoDbPasswordEncryptionKey") == nil {
		b := make([]byte, 16)
		if _, err := rand.Read(b); err != nil {
			return fmt.Errorf("generate mongodb encryption key: %w", err)
		}
		setPassword(vault, "mongoDbPasswordEncryptionKey", base64.StdEncoding.EncodeToString([]byte(hex.EncodeToString(b))))
	}

	// Requires a valid JSON array string.
	setPasswordIfAbsent(vault, "managedServiceSecrets", "[]")

	if vault.GetSecret("googleCloudAvatarPrivateKey") == nil {
		vault.SetSecret(files.SecretEntry{
			Name: "googleCloudAvatarPrivateKey",
			File: &files.SecretFile{Name: "dummy", Content: "dummy"},
		})
	}

	return nil
}

// optionalPasswordSecrets are set to "dummy" only when absent. They are not required for
// private cloud but must be present for the Helm chart to render.
var optionalPasswordSecrets = []string{
	"githubAppsClientId",
	"githubAppsClientSecret",
	"gitlabAppClientId",
	"gitlabAppClientSecret",
	"bitbucketAppsClientId",
	"bitbucketAppsClientSecret",
	"azureDevOpsAppClientId",
	"azureDevOpsAppClientSecret",
	"googleCloudVmImagesPrivateKey",
	"googleClientId",
	"googleClientSecret",
	"googleCloudAvatarBucket",
	"googleCloudAvatarClientEmail",
	"googleCloudAvatarProjectId",
	"gitHubClientId",
	"gitHubClientSecret",
	"gitlabClientId",
	"gitlabClientSecret",
	"bitbucketClientId",
	"bitbucketClientSecret",
	"recaptchaKey",
	"recaptchaSecret",
	"recaptchaKeyV3",
	"recaptchaSecretV3",
	"recaptchaClientEmailV3",
	"recaptchaProjectIdV3",
	"stripeWebhookEndpointSecret",
	"stripePublishableKey",
	"stripeSecretKey",
	"sendGridApiKey",
	"openBaoPassword",
}

func setPassword(vault *files.InstallVault, name, password string) {
	vault.SetSecret(files.SecretEntry{
		Name:   name,
		Fields: &files.SecretFields{Password: password},
	})
}

func setPasswordIfAbsent(vault *files.InstallVault, name, password string) {
	if vault.GetSecret(name) != nil {
		return
	}
	setPassword(vault, name, password)
}

func generateRSAPKCS8KeyPair(bits int) (privatePEM, publicPEM string, err error) {
	key, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return "", "", err
	}
	pkcs8Bytes, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", "", err
	}
	privatePEM = string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8Bytes}))
	spkiBytes, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return "", "", err
	}
	publicPEM = string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: spkiBytes}))
	return privatePEM, publicPEM, nil
}

func generateECP256PKCS8KeyPair() (privatePEM, publicPEM string, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}
	pkcs8Bytes, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", "", err
	}
	privatePEM = string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8Bytes}))
	spkiBytes, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return "", "", err
	}
	publicPEM = string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: spkiBytes}))
	return privatePEM, publicPEM, nil
}

// EnsureIngressCA generates the cluster ingress CA if not already present in vault.
// The CA private key is written to vault; the cert PEM is set on cluster.Certificates.CA.CertPem.
func EnsureIngressCA(vault *files.InstallVault, cluster *files.ClusterConfig) error {
	if vault.GetSecret("selfSignedCaKeyPem") != nil {
		return nil
	}
	keyPEM, certPEM, err := generateCA("Cluster Ingress CA", "DE", "Karlsruhe", "Codesphere")
	if err != nil {
		return fmt.Errorf("generate ingress CA: %w", err)
	}
	vault.SetSecret(files.SecretEntry{
		Name: "selfSignedCaKeyPem",
		File: &files.SecretFile{Name: "key.pem", Content: keyPEM},
	})
	cluster.Certificates.CA.CertPem = certPEM
	return nil
}

// EnsureCephSSHKeys generates the Ceph SSH key pair if not already present in vault.
// The private key is written to vault; the public key is set on ceph.CephAdmSSHKey.PublicKey.
func EnsureCephSSHKeys(vault *files.InstallVault, ceph *files.CephConfig) error {
	if vault.GetSecret("cephSshPrivateKey") != nil {
		return nil
	}
	privKey, pubKey, err := generateSSHKeyPair()
	if err != nil {
		return fmt.Errorf("generate ceph SSH keys: %w", err)
	}
	vault.SetSecret(files.SecretEntry{
		Name: "cephSshPrivateKey",
		File: &files.SecretFile{Name: "id_rsa", Content: privKey},
	})
	ceph.CephAdmSSHKey.PublicKey = pubKey
	return nil
}

// EnsurePostgresSecrets generates all postgres certificates and passwords if not already present
// in vault (sentinel: postgresPassword). Private keys and passwords are written to vault;
// cert PEMs are set on the postgres config struct for inclusion in the config YAML.
func EnsurePostgresSecrets(vault *files.InstallVault, postgres *files.PostgresConfig) error {
	if vault.GetSecret("postgresPassword") != nil {
		return nil
	}

	caKeyPEM, caCertPEM, err := generateCA("PostgreSQL CA", "DE", "Karlsruhe", "Codesphere")
	if err != nil {
		return fmt.Errorf("generate postgres CA: %w", err)
	}

	primaryKeyPEM, primaryCertPEM, err := generateServerCertificate(
		caKeyPEM, caCertPEM,
		postgres.Primary.Hostname,
		[]string{postgres.Primary.IP},
	)
	if err != nil {
		return fmt.Errorf("generate postgres primary cert: %w", err)
	}
	if err := validateCertKeyPair(primaryCertPEM, primaryKeyPEM); err != nil {
		return fmt.Errorf("validate postgres primary cert/key: %w", err)
	}

	vault.SetSecret(files.SecretEntry{Name: "postgresCaKeyPem", File: &files.SecretFile{Name: "ca.key", Content: caKeyPEM}})
	vault.SetSecret(files.SecretEntry{Name: "postgresPassword", Fields: &files.SecretFields{Password: generatePassword(32)}})
	vault.SetSecret(files.SecretEntry{Name: "postgresReplicaPassword", Fields: &files.SecretFields{Password: generatePassword(32)}})
	vault.SetSecret(files.SecretEntry{Name: "postgresPrimaryServerKeyPem", File: &files.SecretFile{Name: "primary.key", Content: primaryKeyPEM}})

	postgres.CACertPem = caCertPEM
	postgres.Primary.SSLConfig.ServerCertPem = primaryCertPEM

	if postgres.Replica != nil {
		replicaKeyPEM, replicaCertPEM, err := generateServerCertificate(
			caKeyPEM, caCertPEM,
			postgres.Replica.Name,
			[]string{postgres.Replica.IP},
		)
		if err != nil {
			return fmt.Errorf("generate postgres replica cert: %w", err)
		}
		if err := validateCertKeyPair(replicaCertPEM, replicaKeyPEM); err != nil {
			return fmt.Errorf("validate postgres replica cert/key: %w", err)
		}
		vault.SetSecret(files.SecretEntry{Name: "postgresReplicaServerKeyPem", File: &files.SecretFile{Name: "replica.key", Content: replicaKeyPEM}})
		postgres.Replica.SSLConfig.ServerCertPem = replicaCertPEM
	}

	for _, service := range postgresServices {
		vault.SetSecret(files.SecretEntry{
			Name:   fmt.Sprintf("postgresPassword%s", files.Capitalize(service)),
			Fields: &files.SecretFields{Password: generatePassword(32)},
		})
	}

	return nil
}
