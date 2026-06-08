// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"bytes"
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
	"time"

	"github.com/codesphere-cloud/oms/internal/bootstrap"
	k8s "github.com/codesphere-cloud/oms/internal/util"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

//go:embed manifests/openbao/vault-cr.yaml
var vaultCRTemplate []byte

const (
	openBaoUnsealSecretName = "openbao-unseal-keys"
	openBaoNamespace        = "vault"
	openBaoImage            = "quay.io/openbao/openbao:2.1.0"
	bankVaultsImage         = "ghcr.io/bank-vaults/bank-vaults:v1.31.3"
	bankVaultsChartRepo     = "oci://ghcr.io/bank-vaults/helm-charts"
	bankVaultsChartName     = "vault-operator"
	bankVaultsChartVersion  = "1.22.5"
	defaultPasswordLength   = 32
	pollInterval            = 5 * time.Second
	maxPollInterval         = 30 * time.Second
)

// OpenBaoInstallerConfig holds all configurable parameters for the OpenBao bootstrap.
type OpenBaoInstallerConfig struct {
	SecretsEngineName string
	Username          string
	DRBackupPath      string
	Replicas          int
	StorageSize       string
	Timeout           time.Duration
	AgeRecipient      string
	AgeKeyPath        string
}

// OpenBaoInstaller orchestrates the Day-0 bootstrap, configuration, and DR
// workflow for OpenBao using the Bank-Vaults Operator.
type OpenBaoInstaller struct {
	Helm      HelmClient
	Clientset kubernetes.Interface
	DynClient dynamic.Interface
	Logger    *bootstrap.StepLogger
	Config    OpenBaoInstallerConfig

	// Intermediate state populated during the install pipeline
	ctx            context.Context
	password       string
	drBackupExists bool
	unsealSecret   *corev1.Secret
}

// NewOpenBaoInstaller constructs an OpenBaoInstaller with real Kubernetes and Helm clients.
func NewOpenBaoInstaller(cfg OpenBaoInstallerConfig) (*OpenBaoInstaller, error) {
	helm, err := NewHelmClient(openBaoNamespace)
	if err != nil {
		return nil, fmt.Errorf("creating helm client: %w", err)
	}

	clientset, dynClient, err := k8s.NewClients()
	if err != nil {
		return nil, fmt.Errorf("creating kubernetes clients: %w", err)
	}

	return &OpenBaoInstaller{
		Helm:      helm,
		Clientset: clientset,
		DynClient: dynClient,
		Logger:    bootstrap.NewStepLogger(false),
		Config:    cfg,
	}, nil
}

const defaultTimeout = 5 * time.Minute

func (o *OpenBaoInstaller) validateConfig() error {
	r := o.Config.Replicas
	if r < 1 {
		return fmt.Errorf("--replicas must be >= 1, got %d", r)
	}
	if r > 1 && r%2 == 0 {
		return fmt.Errorf("--replicas=%d is invalid: Raft requires 1 (single-node) or an odd number >= 3 for HA", r)
	}
	if o.Config.Timeout <= 0 {
		o.Config.Timeout = defaultTimeout
	}
	return nil
}

// Install is the top-level orchestrator for the full OpenBao bootstrap pipeline.
// It is idempotent — safe to re-run at any point.
func (o *OpenBaoInstaller) Install(ctx context.Context) error {
	if err := o.validateConfig(); err != nil {
		return err
	}

	o.ctx = ctx

	err := o.Logger.Step("Pre-flight DR check", o.PreFlightDRCheck)
	if err != nil {
		return fmt.Errorf("pre-flight DR check failed: %w", err)
	}

	// Only generate a new password for fresh installs; on DR restore the
	// password was already extracted from the backup in PreFlightDRCheck.
	if !o.drBackupExists {
		err = o.Logger.Step("Generating secure password", o.GeneratePassword)
		if err != nil {
			return fmt.Errorf("failed to generate secure password: %w", err)
		}
	}

	err = o.Logger.Step("Deploying Bank-Vaults Operator", o.DeployBankVaultsOperator)
	if err != nil {
		return fmt.Errorf("failed to deploy Bank-Vaults Operator: %w", err)
	}

	// If a previous install left behind an unseal-keys Secret (e.g. Raft storage
	// was wiped or the cluster was rebuilt), those keys belong to the old master
	// key and will cause bank-vaults to permanently fail unsealing the new instance.
	// We delete the Vault CR first and wait for pods to exit, otherwise the old
	// sidecar's retry loop re-creates the secret after we remove it.
	if !o.drBackupExists {
		err = o.Logger.Step("Removing stale unseal keys", o.DeleteStaleUnsealKeys)
		if err != nil {
			return fmt.Errorf("failed to remove stale unseal keys: %w", err)
		}
	}

	err = o.Logger.Step("Applying Vault CR (OpenBao desired state)", o.ApplyVaultCR)
	if err != nil {
		return fmt.Errorf("failed to apply Vault CR: %w", err)
	}

	err = o.Logger.Step("Waiting for initialization", o.WaitForInitialization)
	if err != nil {
		return fmt.Errorf("failed waiting for initialization: %w", err)
	}

	err = o.Logger.Step("Extracting and encrypting DR backup", o.ExtractAndEncrypt)
	if err != nil {
		return fmt.Errorf("failed to extract and encrypt DR backup: %w", err)
	}

	o.Logger.Logf("OpenBao bootstrap complete. DR backup saved to: %s", o.Config.DRBackupPath)
	return nil
}

// PreFlightDRCheck checks if a SOPS-encrypted DR backup exists.
// If it does, the backup is decrypted and the unseal keys Secret is pre-applied
// so that Bank-Vaults can unseal an existing OpenBao instance.
// Sets o.drBackupExists to true if a DR backup was found and restored.
func (o *OpenBaoInstaller) PreFlightDRCheck() error {
	if o.Config.DRBackupPath == "" {
		return fmt.Errorf("DRBackupPath must be set")
	}

	if _, err := os.Stat(o.Config.DRBackupPath); err != nil {
		if os.IsNotExist(err) {
			o.Logger.Logf("No existing DR backup found — proceeding with fresh initialization")
			o.drBackupExists = false
			return nil
		}
		return fmt.Errorf("checking DR backup file %s: %w", o.Config.DRBackupPath, err)
	}

	o.Logger.Logf("Found existing DR backup at %s — restoring unseal keys", o.Config.DRBackupPath)

	decrypted, err := DecryptFileWithSOPS(o.Config.DRBackupPath, o.Config.AgeKeyPath)
	if err != nil {
		return err
	}

	var backup drBackup
	if err := json.Unmarshal(decrypted, &backup); err != nil {
		return fmt.Errorf("parsing DR backup: %w", err)
	}

	secretData := make(map[string][]byte)
	for k, v := range backup.UnsealKeys {
		secretData[k] = []byte(v)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      openBaoUnsealSecretName,
			Namespace: openBaoNamespace,
		},
		Data: secretData,
	}

	if err := o.ensureNamespace(o.ctx); err != nil {
		return err
	}

	secretsClient := o.Clientset.CoreV1().Secrets(openBaoNamespace)
	existing, err := secretsClient.Get(o.ctx, openBaoUnsealSecretName, metav1.GetOptions{})
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return fmt.Errorf("checking for existing secret: %w", err)
		}
		_, err = secretsClient.Create(o.ctx, secret, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("creating unseal secret from DR backup: %w", err)
		}
	} else {
		secret.ResourceVersion = existing.ResourceVersion
		_, err = secretsClient.Update(o.ctx, secret, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("updating unseal secret from DR backup: %w", err)
		}
	}

	// Reuse the password and username from the DR backup so the Vault CR is
	// rendered with the same credentials that OpenBao already has configured.
	if o.Config.Username != backup.Username {
		o.Logger.Logf("Warning: --bao-user=%q differs from DR backup username %q — using backup value", o.Config.Username, backup.Username)
	}
	o.password = backup.Password
	o.Config.Username = backup.Username

	o.Logger.Logf("Unseal keys restored from DR backup successfully")
	o.drBackupExists = true
	return nil
}

// GeneratePassword generates a secure password and stores it on the installer.
func (o *OpenBaoInstaller) GeneratePassword() error {
	var err error
	o.password, err = GenerateSecurePassword(defaultPasswordLength)
	return err
}

// DeployBankVaultsOperator installs or upgrades the Bank-Vaults Operator Helm chart.
// This is idempotent via UpgradeChart with InstallIfNotExist.
func (o *OpenBaoInstaller) DeployBankVaultsOperator() error {
	cfg := ChartConfig{
		ReleaseName:     "vault-operator",
		ChartName:       bankVaultsChartRepo + "/" + bankVaultsChartName,
		Version:         bankVaultsChartVersion,
		Namespace:       openBaoNamespace,
		CreateNamespace: true,
		Values:          map[string]interface{}{},
	}

	return o.Helm.UpgradeChart(o.ctx, cfg, UpgradeChartOptions{InstallIfNotExist: true})
}

// vaultCRTemplateData holds the values injected into the Vault CR template.
type vaultCRTemplateData struct {
	Namespace         string
	OpenBaoImage      string
	BankVaultsImage   string
	SecretsEngineName string
	BaoUsername       string
	BaoPassword       string
	Replicas          int
	StorageSize       string
	RetryJoinAddrs    []string
}

// ApplyVaultCR renders the Bank-Vaults Vault CR template and applies it to the cluster.
func (o *OpenBaoInstaller) ApplyVaultCR() error {
	tmpl, err := template.New("vault-cr").Parse(string(vaultCRTemplate))
	if err != nil {
		return fmt.Errorf("parsing vault CR template: %w", err)
	}

	// Build retry_join addresses for Raft so each node can autonomously
	// find and join the cluster leader. For a single replica this produces
	// one self-referencing address, which is harmless and means scaling up
	// later only requires changing the replica count.
	var retryJoinAddrs []string
	for i := 0; i < o.Config.Replicas; i++ {
		addr := fmt.Sprintf("http://openbao-%d.%s.svc.cluster.local:8200", i, openBaoNamespace)
		retryJoinAddrs = append(retryJoinAddrs, addr)
	}

	data := vaultCRTemplateData{
		Namespace:         openBaoNamespace,
		OpenBaoImage:      openBaoImage,
		BankVaultsImage:   bankVaultsImage,
		SecretsEngineName: o.Config.SecretsEngineName,
		BaoUsername:       o.Config.Username,
		BaoPassword:       o.password,
		Replicas:          o.Config.Replicas,
		StorageSize:       o.Config.StorageSize,
		RetryJoinAddrs:    retryJoinAddrs,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("rendering vault CR template: %w", err)
	}

	objects, err := k8s.DecodeMultiDocYAML(buf.Bytes())
	if err != nil {
		return fmt.Errorf("decoding vault CR: %w", err)
	}

	for _, obj := range objects {
		gvr, err := k8s.GvrForUnstructured(obj)
		if err != nil {
			return fmt.Errorf("resolving GVR for %s: %w", obj.GetKind(), err)
		}
		if err := k8s.ApplyUnstructured(o.ctx, o.DynClient, gvr, obj); err != nil {
			return fmt.Errorf("applying vault CR: %w", err)
		}
	}

	return nil
}

// WaitForInitialization polls the openbao-unseal-keys Secret until it contains
// unseal key data, indicating that Bank-Vaults has completed initialization.
func (o *OpenBaoInstaller) WaitForInitialization() error {
	secretsClient := o.Clientset.CoreV1().Secrets(openBaoNamespace)

	return o.pollUntil("waiting for openbao-unseal-keys to be populated", func() (bool, error) {
		secret, err := secretsClient.Get(o.ctx, openBaoUnsealSecretName, metav1.GetOptions{})
		if err != nil {
			if k8serrors.IsNotFound(err) {
				return false, nil // Secret doesn't exist yet — keep polling
			}
			return false, fmt.Errorf("fetching unseal secret: %w", err)
		}

		// Check if the secret has meaningful data: at least one key must be
		// present, indicating bank-vaults has completed initialization and
		// written the unseal keys. Bank-vaults writes all keys atomically
		// during Init(), so any data means init is done.
		if len(secret.Data) > 0 {
			o.unsealSecret = secret
			return true, nil
		}
		return false, nil
	})
}

// ExtractAndEncrypt reads the unseal keys Secret, combines it with the generated
// password, and creates a SOPS-encrypted backup file.
func (o *OpenBaoInstaller) ExtractAndEncrypt() error {
	backup := drBackup{
		UnsealKeys: make(map[string]string),
		Password:   o.password,
		Username:   o.Config.Username,
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	}

	for key, val := range o.unsealSecret.Data {
		backup.UnsealKeys[key] = string(val)
	}

	plaintext, err := json.MarshalIndent(backup, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling DR backup: %w", err)
	}

	dir := filepath.Dir(o.Config.DRBackupPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating DR backup directory: %w", err)
	}

	// Write plaintext to a temp file (sops encrypts in-place).
	// Use os.CreateTemp to avoid predictable filenames (symlink attacks).
	tmpFile, err := os.CreateTemp(dir, "openbao-dr-*.json")
	if err != nil {
		return fmt.Errorf("creating temp backup file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() { _ = os.Remove(tmpPath) }() // clean up temp file on failure or panic

	if err := tmpFile.Chmod(0o600); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("setting temp file permissions: %w", err)
	}
	if _, err := tmpFile.Write(plaintext); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("writing temp backup file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("closing temp backup file: %w", err)
	}

	if err := EncryptFileWithSOPS(tmpPath, o.Config.DRBackupPath, o.Config.AgeRecipient); err != nil {
		return fmt.Errorf("encrypting DR backup: %w", err)
	}

	o.Logger.Logf("DR backup encrypted and saved to: %s", o.Config.DRBackupPath)
	return nil
}

// DeleteStaleUnsealKeys removes unseal keys left by a prior installation whose
// Raft storage no longer exists (e.g. cluster rebuild, PVC deletion). Without
// removal, bank-vaults would attempt to unseal with the old master key's shares
// and fail permanently — the new instance needs to run a fresh init.
//
// To prevent the old bank-vaults sidecar from re-creating the secret via its
// retry loop, the Vault CR is deleted first and we wait for all pods to exit
// before removing the secret.
func (o *OpenBaoInstaller) DeleteStaleUnsealKeys() error {
	vaultGVR := k8s.VaultGVR()

	// Tolerates NotFound — this may be a first-time install with no prior Vault CR.
	delErr := o.DynClient.Resource(vaultGVR).Namespace(openBaoNamespace).Delete(
		o.ctx, "openbao", metav1.DeleteOptions{},
	)
	if delErr != nil && !k8serrors.IsNotFound(delErr) {
		return fmt.Errorf("deleting Vault CR: %w", delErr)
	}

	if err := o.waitForVaultPodsGone(); err != nil {
		return err
	}

	// Now it is safe to delete the stale secret.
	delErr = o.Clientset.CoreV1().Secrets(openBaoNamespace).Delete(
		o.ctx, openBaoUnsealSecretName, metav1.DeleteOptions{},
	)
	if delErr != nil && !k8serrors.IsNotFound(delErr) {
		return fmt.Errorf("deleting stale unseal secret: %w", delErr)
	}

	o.Logger.Logf("Stale unseal keys removed (vault CR deleted, pods terminated)")
	return nil
}

// waitForVaultPodsGone polls until no pods with label vault_cr=openbao remain
// in the vault namespace, or until the context deadline is exceeded.
func (o *OpenBaoInstaller) waitForVaultPodsGone() error {
	selector := labels.SelectorFromSet(labels.Set{"vault_cr": "openbao"}).String()

	return o.pollUntil("waiting for vault pods to terminate", func() (bool, error) {
		list, err := o.Clientset.CoreV1().Pods(openBaoNamespace).List(o.ctx, metav1.ListOptions{
			LabelSelector: selector,
		})
		if err != nil {
			if k8serrors.IsNotFound(err) {
				return true, nil
			}
			return false, fmt.Errorf("listing vault pods: %w", err)
		}
		return len(list.Items) == 0, nil
	})
}

// pollUntil runs check in a loop with exponential backoff until it returns
// true or the configured timeout expires. timeoutMsg describes the operation
// for the timeout error message.
func (o *OpenBaoInstaller) pollUntil(timeoutMsg string, check func() (bool, error)) error {
	deadline := time.Now().Add(o.Config.Timeout)
	interval := pollInterval

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out %s (timeout: %s)", timeoutMsg, o.Config.Timeout)
		}

		done, err := check()
		if err != nil {
			return err
		}
		if done {
			return nil
		}

		o.Logger.LogRetry()
		select {
		case <-o.ctx.Done():
			return o.ctx.Err()
		case <-time.After(interval):
		}
		interval = min(interval*2, maxPollInterval)
	}
}

// ensureNamespace creates the target namespace if it doesn't exist.
func (o *OpenBaoInstaller) ensureNamespace(ctx context.Context) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: openBaoNamespace,
		},
	}

	_, err := o.Clientset.CoreV1().Namespaces().Get(ctx, openBaoNamespace, metav1.GetOptions{})
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return fmt.Errorf("checking namespace %s: %w", openBaoNamespace, err)
		}
		_, err = o.Clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
		if err != nil && !k8serrors.IsAlreadyExists(err) {
			return fmt.Errorf("creating namespace %s: %w", openBaoNamespace, err)
		}
	}
	return nil
}

// drBackup represents the structure of the SOPS-encrypted DR backup file.
type drBackup struct {
	UnsealKeys map[string]string `json:"unseal_keys"`
	Password   string            `json:"password"`
	Username   string            `json:"username"`
	Timestamp  string            `json:"timestamp"`
}

// GenerateSecurePassword generates a cryptographically secure random password
// of the specified byte length, returned as base64url-encoded string.
func GenerateSecurePassword(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// SetCtx is a test helper.
func (o *OpenBaoInstaller) SetCtx(ctx context.Context) {
	o.ctx = ctx
}

// SetUnsealSecret is a test helper.
func (o *OpenBaoInstaller) SetUnsealSecret(secret *corev1.Secret) {
	o.unsealSecret = secret
}

// SetPassword is a test helper.
func (o *OpenBaoInstaller) SetPassword(password string) {
	o.password = password
}

// GetDRBackupExists returns whether a DR backup was found during pre-flight check.
func (o *OpenBaoInstaller) GetDRBackupExists() bool {
	return o.drBackupExists
}

// GetUnsealSecret returns the unseal secret populated during initialization.
func (o *OpenBaoInstaller) GetUnsealSecret() *corev1.Secret {
	return o.unsealSecret
}
