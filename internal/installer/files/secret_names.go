// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package files

// Secret name constants — each constant value equals the vault secret name it represents.
const (
	// Auth tokens
	SecretTokenPrivateKey = "tokenPrivateKey"
	SecretTokenPublicKey  = "tokenPublicKey"

	// Codesphere core
	SecretDomainAuthPrivateKey = "domainAuthPrivateKey"
	SecretDomainAuthPublicKey  = "domainAuthPublicKey"

	// OIDC
	SecretOidcClientId     = "oidcClientId"
	SecretOidcClientSecret = "oidcClientSecret"

	// GitHub
	SecretGithubAppsClientId     = "githubAppsClientId"
	SecretGithubAppsClientSecret = "githubAppsClientSecret"

	// GitLab
	SecretGitlabAppClientId     = "gitlabAppClientId"
	SecretGitlabAppClientSecret = "gitlabAppClientSecret"

	// Bitbucket
	SecretBitbucketAppsClientId     = "bitbucketAppsClientId"
	SecretBitbucketAppsClientSecret = "bitbucketAppsClientSecret"

	// Azure DevOps
	SecretAzureDevOpsAppClientId     = "azureDevOpsAppClientId"
	SecretAzureDevOpsAppClientSecret = "azureDevOpsAppClientSecret"

	// Registry
	SecretRegistryUsername = "registryUsername"
	SecretRegistryPassword = "registryPassword"

	// ACME
	SecretAcmeEabMacKey = "acmeEabMacKey"

	// OpenBao
	SecretOpenBaoPassword = "openBaoPassword"

	// Monitoring
	SecretLokiGatewayBasicAuthPassword = "lokiGatewayBasicAuthPassword"

	// Postgres
	SecretPostgresCaKeyPem            = "postgresCaKeyPem"
	SecretPostgresPassword            = "postgresPassword"
	SecretPostgresPrimaryServerKeyPem = "postgresPrimaryServerKeyPem"
	SecretPostgresReplicaPassword     = "postgresReplicaPassword"
	SecretPostgresReplicaServerKeyPem = "postgresReplicaServerKeyPem"

	// Ceph
	SecretCephSshPrivateKey = "cephSshPrivateKey"

	// Cluster / TLS
	SecretSelfSignedCaKeyPem = "selfSignedCaKeyPem"

	// Mounter
	SecretMounterHmacSecret = "mounterHmacSecret"

	// Nix
	SecretPrivNixSigningKey = "privNixSigningKey"
	SecretPubNixSigningKey  = "pubNixSigningKey"

	// Generated-only (not consumed by merge)
	SecretManagedServiceSecrets = "managedServiceSecrets"
	SecretKubeConfig            = "kubeConfig"

	// Default/optional
	SecretDigitalOceanApiToken          = "digitalOceanApiToken"
	SecretMongoDbPasswordEncryptionKey  = "mongoDbPasswordEncryptionKey"
	SecretGoogleCloudAvatarPrivateKey   = "googleCloudAvatarPrivateKey"
	SecretGoogleCloudVmImagesPrivateKey = "googleCloudVmImagesPrivateKey"
	SecretGoogleClientId                = "googleClientId"
	SecretGoogleClientSecret            = "googleClientSecret"
	SecretGoogleCloudAvatarBucket       = "googleCloudAvatarBucket"
	SecretGoogleCloudAvatarClientEmail  = "googleCloudAvatarClientEmail"
	SecretGoogleCloudAvatarProjectId    = "googleCloudAvatarProjectId"
	SecretGitHubClientId                = "gitHubClientId"
	SecretGitHubClientSecret            = "gitHubClientSecret"
	SecretGitlabClientId                = "gitlabClientId"
	SecretGitlabClientSecret            = "gitlabClientSecret"
	SecretBitbucketClientId             = "bitbucketClientId"
	SecretBitbucketClientSecret         = "bitbucketClientSecret"
	SecretRecaptchaKey                  = "recaptchaKey"
	SecretRecaptchaSecret               = "recaptchaSecret"
	SecretRecaptchaKeyV3                = "recaptchaKeyV3"
	SecretRecaptchaSecretV3             = "recaptchaSecretV3"
	SecretRecaptchaClientEmailV3        = "recaptchaClientEmailV3"
	SecretRecaptchaProjectIdV3          = "recaptchaProjectIdV3"
	SecretStripeWebhookEndpointSecret   = "stripeWebhookEndpointSecret"
	SecretStripePublishableKey          = "stripePublishableKey"
	SecretStripeSecretKey               = "stripeSecretKey"
	SecretSendGridApiKey                = "sendGridApiKey"
)
