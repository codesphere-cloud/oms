// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

// Secrets is the canonical registry of all vault secret names used across the
// Codesphere private-cloud installer. The key is the vault secret name; access
// a name via Secrets["openBaoPassword"] to get the string "openBaoPassword".
var Secrets = map[string]string{
	// Auth tokens
	"tokenPrivateKey": "tokenPrivateKey",
	"tokenPublicKey":  "tokenPublicKey",

	// Codesphere core
	"domainAuthPrivateKey": "domainAuthPrivateKey",
	"domainAuthPublicKey":  "domainAuthPublicKey",

	// OIDC
	"oidcClientId":     "oidcClientId",
	"oidcClientSecret": "oidcClientSecret",

	// GitHub
	"githubAppsClientId":     "githubAppsClientId",
	"githubAppsClientSecret": "githubAppsClientSecret",

	// GitLab
	"gitlabAppClientId":     "gitlabAppClientId",
	"gitlabAppClientSecret": "gitlabAppClientSecret",

	// Bitbucket
	"bitbucketAppsClientId":     "bitbucketAppsClientId",
	"bitbucketAppsClientSecret": "bitbucketAppsClientSecret",

	// Azure DevOps
	"azureDevOpsAppClientId":     "azureDevOpsAppClientId",
	"azureDevOpsAppClientSecret": "azureDevOpsAppClientSecret",

	// Registry
	"registryUsername": "registryUsername",
	"registryPassword": "registryPassword",

	// ACME
	"acmeEabMacKey": "acmeEabMacKey",

	// OpenBao
	"openBaoPassword": "openBaoPassword",

	// Monitoring
	"lokiGatewayBasicAuthPassword": "lokiGatewayBasicAuthPassword",

	// Postgres
	"postgresCaKeyPem":            "postgresCaKeyPem",
	"postgresPassword":            "postgresPassword",
	"postgresPrimaryServerKeyPem": "postgresPrimaryServerKeyPem",
	"postgresReplicaPassword":     "postgresReplicaPassword",
	"postgresReplicaServerKeyPem": "postgresReplicaServerKeyPem",

	// Ceph
	"cephSshPrivateKey": "cephSshPrivateKey",

	// Cluster / TLS
	"selfSignedCaKeyPem": "selfSignedCaKeyPem",

	// Mounter
	"mounterHmacSecret": "mounterHmacSecret",

	// Nix
	"privNixSigningKey": "privNixSigningKey",
	"pubNixSigningKey":  "pubNixSigningKey",

	// Generated-only (not consumed by merge)
	"managedServiceSecrets": "managedServiceSecrets",
	"kubeConfig":            "kubeConfig",

	// Default/optional — required by the Helm chart but not managed by installer config
	"digitalOceanApiToken":          "digitalOceanApiToken",
	"mongoDbPasswordEncryptionKey":  "mongoDbPasswordEncryptionKey",
	"googleCloudAvatarPrivateKey":   "googleCloudAvatarPrivateKey",
	"googleCloudVmImagesPrivateKey": "googleCloudVmImagesPrivateKey",
	"googleClientId":                "googleClientId",
	"googleClientSecret":            "googleClientSecret",
	"googleCloudAvatarBucket":       "googleCloudAvatarBucket",
	"googleCloudAvatarClientEmail":  "googleCloudAvatarClientEmail",
	"googleCloudAvatarProjectId":    "googleCloudAvatarProjectId",
	"gitHubClientId":                "gitHubClientId",
	"gitHubClientSecret":            "gitHubClientSecret",
	"gitlabClientId":                "gitlabClientId",
	"gitlabClientSecret":            "gitlabClientSecret",
	"bitbucketClientId":             "bitbucketClientId",
	"bitbucketClientSecret":         "bitbucketClientSecret",
	"recaptchaKey":                  "recaptchaKey",
	"recaptchaSecret":               "recaptchaSecret",
	"recaptchaKeyV3":                "recaptchaKeyV3",
	"recaptchaSecretV3":             "recaptchaSecretV3",
	"recaptchaClientEmailV3":        "recaptchaClientEmailV3",
	"recaptchaProjectIdV3":          "recaptchaProjectIdV3",
	"stripeWebhookEndpointSecret":   "stripeWebhookEndpointSecret",
	"stripePublishableKey":          "stripePublishableKey",
	"stripeSecretKey":               "stripeSecretKey",
	"sendGridApiKey":                "sendGridApiKey",
}

// MergedSecretNames lists every vault secret name that MergeVaultIntoConfig reads into
// a specific config field. Used to identify "extra" secrets for round-trip preservation.
// Postgres per-service password names are added dynamically at merge time.
var MergedSecretNames = []string{
	Secrets["postgresCaKeyPem"],
	Secrets["postgresPassword"],
	Secrets["postgresPrimaryServerKeyPem"],
	Secrets["postgresReplicaPassword"],
	Secrets["postgresReplicaServerKeyPem"],
	Secrets["cephSshPrivateKey"],
	Secrets["selfSignedCaKeyPem"],
	Secrets["domainAuthPrivateKey"],
	Secrets["domainAuthPublicKey"],
	Secrets["registryUsername"],
	Secrets["registryPassword"],
	Secrets["githubAppsClientId"],
	Secrets["githubAppsClientSecret"],
	Secrets["acmeEabMacKey"],
	Secrets["openBaoPassword"],
	Secrets["lokiGatewayBasicAuthPassword"],
}
