// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"time"

	"github.com/codesphere-cloud/oms/internal/bootstrap"
	"github.com/codesphere-cloud/oms/internal/installer/vault/sops"
	k8s "github.com/codesphere-cloud/oms/internal/util"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
)

const (
	openBaoUnsealSecretName = "openbao-unseal-keys"
	openBaoVaultCRName      = "openbao"
	DefaultOpenBaoNamespace = "vault"
	pollInterval            = 5 * time.Second
	maxPollInterval         = 30 * time.Second

	// defaultReadinessTimeoutPerReplica is added to the base timeout for each
	// replica when waiting for all OpenBao pods to become ready. Pods come up
	// sequentially and each must join Raft before the next, so the total wait
	// scales with the deployment size.
	defaultReadinessTimeoutPerReplica = 3 * time.Minute

	// vaultCRLabelKey/Value identify pods managed by the bank-vaults operator
	// for our "openbao" Vault CR.
	vaultCRLabelKey   = "vault_cr"
	vaultCRLabelValue = openBaoVaultCRName

	// appNameLabelKey/Value distinguish the OpenBao server pods from the
	// operator's configurer pod — both carry vault_cr=openbao, but only the
	// server pods are app.kubernetes.io/name=vault. Pod counting must exclude
	// the configurer, otherwise readiness never matches the replica count.
	appNameLabelKey        = "app.kubernetes.io/name"
	appNameLabelValueVault = "vault"
)

// vaultPodLabelSelector selects only the OpenBao server pods, excluding the
// configurer pod (which also carries vault_cr=openbao). Used for readiness
// checks that must count exactly the StatefulSet replicas.
var vaultPodLabelSelector = labels.SelectorFromSet(labels.Set{
	vaultCRLabelKey: vaultCRLabelValue,
	appNameLabelKey: appNameLabelValueVault,
}).String()

// OpenBaoBootstrapConfig holds all configurable parameters for the OpenBao bootstrap.
type OpenBaoBootstrapConfig struct {
	Namespace    string
	DRBackupPath string
	// Restore seeds the unseal keys from DRBackupPath instead of exporting them.
	Restore bool
	Timeout time.Duration
	// ReadinessTimeoutPerReplica is added to Timeout per replica when waiting
	// for all pods to become ready. Defaulted in validateConfig when unset.
	ReadinessTimeoutPerReplica time.Duration
	AgeRecipient               string
	AgeKeyPath                 string
	StepTimer                  bool
}

// OpenBaoBootstrapper covers what ArgoCD cannot do for an OpenBao instance
// deployed by the openbao chart: exporting the bank-vaults unseal keys to a
// SOPS-encrypted DR backup, and seeding them from that backup on restore.
type OpenBaoBootstrapper struct {
	Clientset kubernetes.Interface
	DynClient dynamic.Interface
	Logger    *bootstrap.StepLogger
	Config    OpenBaoBootstrapConfig

	// Intermediate state populated during the bootstrap pipeline
	ctx              context.Context
	replicas         int
	unsealSecret     *corev1.Secret
	backupUnsealKeys map[string][]byte // unseal keys from DR backup, used during WaitForInitialization
}

// NewOpenBaoBootstrapper constructs an OpenBaoBootstrapper with real Kubernetes clients.
func NewOpenBaoBootstrapper(cfg OpenBaoBootstrapConfig) (*OpenBaoBootstrapper, error) {
	clientset, dynClient, err := k8s.NewClients()
	if err != nil {
		return nil, fmt.Errorf("creating kubernetes clients: %w", err)
	}

	return &OpenBaoBootstrapper{
		Clientset: clientset,
		DynClient: dynClient,
		Logger:    bootstrap.NewStepLogger(false, bootstrap.WithTimer(cfg.StepTimer)),
		Config:    cfg,
	}, nil
}

const defaultTimeout = 5 * time.Minute

func (o *OpenBaoBootstrapper) validateConfig() error {
	if o.Config.Namespace == "" {
		o.Config.Namespace = DefaultOpenBaoNamespace
	}
	if o.Config.DRBackupPath == "" {
		return fmt.Errorf("DRBackupPath must be set")
	}
	if o.Config.Timeout <= 0 {
		o.Config.Timeout = defaultTimeout
	}
	if o.Config.ReadinessTimeoutPerReplica <= 0 {
		o.Config.ReadinessTimeoutPerReplica = defaultReadinessTimeoutPerReplica
	}
	return nil
}

// Bootstrap exports the unseal keys of the ArgoCD-deployed instance to the DR
// backup, or, with Config.Restore, seeds them from it. Safe to re-run.
func (o *OpenBaoBootstrapper) Bootstrap(ctx context.Context) error {
	if err := o.validateConfig(); err != nil {
		return err
	}
	o.ctx = ctx

	if o.Config.Restore {
		return o.restore()
	}
	return o.export()
}

func (o *OpenBaoBootstrapper) export() error {
	if _, err := os.Stat(o.Config.DRBackupPath); err == nil {
		return fmt.Errorf("DR backup %s already exists: use --restore to restore from it, or choose another --dr-backup-path", o.Config.DRBackupPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("checking DR backup file %s: %w", o.Config.DRBackupPath, err)
	}

	err := o.Logger.Step("Checking for Vault CR", func() error {
		found, err := o.loadVaultCR()
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("vault CR %q not found in namespace %q: sync the OpenBao ArgoCD app first", openBaoVaultCRName, o.Config.Namespace)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed checking for Vault CR: %w", err)
	}

	if err := o.waitUntilReady(); err != nil {
		return err
	}

	err = o.Logger.Step("Extracting and encrypting DR backup", o.ExtractAndEncrypt)
	if err != nil {
		return fmt.Errorf("failed to extract and encrypt DR backup: %w", err)
	}

	o.Logger.Logf("OpenBao bootstrap complete. DR backup saved to: %s", o.Config.DRBackupPath)
	return nil
}

func (o *OpenBaoBootstrapper) restore() error {
	err := o.Logger.Step("Loading DR backup", o.PreFlightDRCheck)
	if err != nil {
		return fmt.Errorf("failed to load DR backup: %w", err)
	}

	// The secret must exist before the chart's Vault CR comes up, otherwise
	// bank-vaults initializes a new instance instead of unsealing the restored one.
	err = o.Logger.Step("Seeding unseal keys from DR backup", func() error {
		if err := o.ensureNamespace(); err != nil {
			return err
		}
		secretsClient := o.Clientset.CoreV1().Secrets(o.Config.Namespace)
		if err := o.checkNoForeignUnsealKeys(secretsClient); err != nil {
			return err
		}
		return o.ensureUnsealSecret(secretsClient)
	})
	if err != nil {
		return fmt.Errorf("failed to seed unseal keys: %w", err)
	}

	err = o.Logger.Step("Waiting for Vault CR (sync the OpenBao ArgoCD app now if not done yet)", o.WaitForVaultCR)
	if err != nil {
		return fmt.Errorf("failed waiting for Vault CR: %w", err)
	}

	if err := o.waitUntilReady(); err != nil {
		return err
	}

	o.Logger.Logf("OpenBao restore complete.")
	return nil
}

func (o *OpenBaoBootstrapper) waitUntilReady() error {
	err := o.Logger.Step("Waiting for initialization", o.WaitForInitialization)
	if err != nil {
		return fmt.Errorf("failed waiting for initialization: %w", err)
	}

	err = o.Logger.Step("Waiting for all OpenBao pods to be ready", o.WaitForPodsReady)
	if err != nil {
		return fmt.Errorf("failed waiting for pods to be ready: %w", err)
	}
	return nil
}

// PreFlightDRCheck decrypts the DR backup and stores the unseal keys in memory
// (backupUnsealKeys) for seeding the unseal Secret.
func (o *OpenBaoBootstrapper) PreFlightDRCheck() error {
	if o.Config.DRBackupPath == "" {
		return fmt.Errorf("DRBackupPath must be set")
	}

	if _, err := os.Stat(o.Config.DRBackupPath); err != nil {
		return fmt.Errorf("reading DR backup file %s: %w", o.Config.DRBackupPath, err)
	}

	decrypted, err := sops.DecryptFile(o.Config.DRBackupPath, o.Config.AgeKeyPath)
	if err != nil {
		return err
	}

	var backup drBackup
	if err := json.Unmarshal(decrypted, &backup); err != nil {
		return fmt.Errorf("parsing DR backup: %w", err)
	}
	if len(backup.UnsealKeys) == 0 {
		return fmt.Errorf("DR backup %s contains no unseal keys", o.Config.DRBackupPath)
	}

	o.backupUnsealKeys = make(map[string][]byte)
	for k, v := range backup.UnsealKeys {
		o.backupUnsealKeys[k] = []byte(v)
	}
	return nil
}

// loadVaultCR fetches the Vault CR and records its replica count (spec.size).
// It reports false when the CR does not exist.
func (o *OpenBaoBootstrapper) loadVaultCR() (bool, error) {
	cr, err := o.DynClient.Resource(k8s.VaultGVR()).Namespace(o.Config.Namespace).Get(
		o.ctx, openBaoVaultCRName, metav1.GetOptions{},
	)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("fetching Vault CR: %w", err)
	}

	size, found, err := unstructured.NestedInt64(cr.Object, "spec", "size")
	if err != nil {
		return false, fmt.Errorf("reading spec.size of Vault CR: %w", err)
	}
	// bank-vaults defaults an unset size to a single replica.
	o.replicas = 1
	if found && size > 0 {
		o.replicas = int(size)
	}
	return true, nil
}

// WaitForVaultCR polls until the Vault CR exists and records its replica count.
func (o *OpenBaoBootstrapper) WaitForVaultCR() error {
	return o.pollUntil(fmt.Sprintf("waiting for Vault CR %q", openBaoVaultCRName), o.loadVaultCR)
}

// WaitForInitialization polls the openbao-unseal-keys Secret until it contains
// unseal key data, indicating that Bank-Vaults has completed initialization.
//
// When a DR backup was loaded (backupUnsealKeys is set), the function ensures
// the secret exists with the backup's unseal keys on every poll iteration. This
// handles the case where the bank-vaults operator deletes or recreates the
// secret during Vault CR reconciliation — we simply re-apply it until the
// operator settles and the sidecar can successfully unseal.
func (o *OpenBaoBootstrapper) WaitForInitialization() error {
	secretsClient := o.Clientset.CoreV1().Secrets(o.Config.Namespace)

	return o.pollUntil("waiting for openbao-unseal-keys to be populated", func() (bool, error) {
		secret, err := secretsClient.Get(o.ctx, openBaoUnsealSecretName, metav1.GetOptions{})
		if err != nil {
			if !k8serrors.IsNotFound(err) {
				return false, fmt.Errorf("fetching unseal secret: %w", err)
			}
			// Secret doesn't exist yet.
			if o.backupUnsealKeys != nil {
				// DR restore: create the secret from backup so the sidecar can unseal.
				if createErr := o.ensureUnsealSecret(secretsClient); createErr != nil {
					return false, createErr
				}
			}
			return false, nil // Keep polling — sidecar hasn't confirmed unseal yet
		}

		// Check if the secret has meaningful data: at least one key must be
		// present, indicating bank-vaults has completed initialization and
		// written the unseal keys.
		if len(secret.Data) > 0 {
			if o.backupUnsealKeys != nil && !unsealKeysEqual(secret.Data, o.backupUnsealKeys) {
				return false, fmt.Errorf("secret %s holds unseal keys that differ from the DR backup: bank-vaults initialized a new instance instead of restoring", openBaoUnsealSecretName)
			}
			o.unsealSecret = secret
			return true, nil
		}

		// Secret exists but is empty — restore from backup if available.
		if o.backupUnsealKeys != nil {
			if updateErr := o.ensureUnsealSecret(secretsClient); updateErr != nil {
				return false, updateErr
			}
		}
		return false, nil
	})
}

// checkNoForeignUnsealKeys refuses to restore over an initialized instance:
// replacing its keys would make its Raft data unsealable after the next restart.
func (o *OpenBaoBootstrapper) checkNoForeignUnsealKeys(secretsClient corev1client.SecretInterface) error {
	existing, err := secretsClient.Get(o.ctx, openBaoUnsealSecretName, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("checking unseal secret: %w", err)
	}
	if len(existing.Data) > 0 && !unsealKeysEqual(existing.Data, o.backupUnsealKeys) {
		return fmt.Errorf("secret %s/%s already holds unseal keys that differ from the DR backup: refusing to overwrite an initialized instance", o.Config.Namespace, openBaoUnsealSecretName)
	}
	return nil
}

func unsealKeysEqual(a, b map[string][]byte) bool {
	return maps.EqualFunc(a, b, bytes.Equal)
}

// ensureUnsealSecret creates or updates the unseal keys secret from the DR backup.
// It preserves existing metadata (labels, annotations, ownerReferences) when updating.
func (o *OpenBaoBootstrapper) ensureUnsealSecret(secretsClient corev1client.SecretInterface) error {
	existing, err := secretsClient.Get(o.ctx, openBaoUnsealSecretName, metav1.GetOptions{})
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return fmt.Errorf("checking unseal secret: %w", err)
		}
		// Create new secret
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      openBaoUnsealSecretName,
				Namespace: o.Config.Namespace,
			},
			Data: o.backupUnsealKeys,
		}
		_, err = secretsClient.Create(o.ctx, secret, metav1.CreateOptions{})
		if err == nil {
			return nil
		}
		if !k8serrors.IsAlreadyExists(err) {
			return fmt.Errorf("creating unseal secret from backup: %w", err)
		}
		// Secret was created concurrently (e.g. by the operator) between our Get
		// and Create. Re-fetch so we can update its data rather than leaving
		// whatever (possibly empty/stale) data the racing writer set.
		existing, err = secretsClient.Get(o.ctx, openBaoUnsealSecretName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("re-fetching unseal secret after create conflict: %w", err)
		}
	}

	// Update existing secret — preserve metadata, only set Data
	existing.Data = o.backupUnsealKeys
	_, err = secretsClient.Update(o.ctx, existing, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("updating unseal secret from backup: %w", err)
	}
	return nil
}

// readinessTimeout returns how long to wait for all pods to become ready.
// StatefulSet pods start sequentially and each Raft member must initialize and
// join before the next comes up, so the wait grows with replica count: the
// configured base timeout plus a per-replica allowance.
func (o *OpenBaoBootstrapper) readinessTimeout() time.Duration {
	return o.Config.Timeout + o.Config.ReadinessTimeoutPerReplica*time.Duration(o.replicas)
}

// WaitForPodsReady polls until the expected number of vault pods (spec.size of
// the Vault CR) are in Running phase with all containers Ready.
func (o *OpenBaoBootstrapper) WaitForPodsReady() error {
	selector := vaultPodLabelSelector
	expected := o.replicas

	return o.pollUntilTimeout(o.readinessTimeout(), "waiting for all OpenBao pods to be ready", func() (bool, error) {
		list, err := o.Clientset.CoreV1().Pods(o.Config.Namespace).List(o.ctx, metav1.ListOptions{
			LabelSelector: selector,
		})
		if err != nil {
			return false, fmt.Errorf("listing vault pods: %w", err)
		}

		var activePods int
		var readyCount int
		for i := range list.Items {
			if list.Items[i].DeletionTimestamp != nil {
				continue // Skip terminating pods
			}
			activePods++
			if isPodReady(&list.Items[i]) {
				readyCount++
			}
		}
		return activePods == expected && readyCount == expected, nil
	})
}

// isPodReady returns true if the pod is in Running phase and has the Ready condition.
func isPodReady(pod *corev1.Pod) bool {
	if pod.Status.Phase != corev1.PodRunning {
		return false
	}
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// ExtractAndEncrypt reads the unseal keys Secret and writes it to a
// SOPS-encrypted backup file.
func (o *OpenBaoBootstrapper) ExtractAndEncrypt() error {
	backup := drBackup{
		UnsealKeys: make(map[string]string),
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

	if err := sops.EncryptFile(tmpPath, o.Config.DRBackupPath, o.Config.AgeRecipient); err != nil {
		return fmt.Errorf("encrypting DR backup: %w", err)
	}

	o.Logger.Logf("DR backup encrypted and saved to: %s", o.Config.DRBackupPath)
	return nil
}

// pollUntil runs check in a loop with exponential backoff until it returns
// true or the configured timeout expires. timeoutMsg describes the operation
// for the timeout error message.
func (o *OpenBaoBootstrapper) pollUntil(timeoutMsg string, check func() (bool, error)) error {
	return o.pollUntilTimeout(o.Config.Timeout, timeoutMsg, check)
}

// pollUntilTimeout is pollUntil with an explicit timeout, used by steps (e.g.
// readiness) whose duration scales with the deployment size rather than the
// fixed per-step timeout.
func (o *OpenBaoBootstrapper) pollUntilTimeout(timeout time.Duration, timeoutMsg string, check func() (bool, error)) error {
	deadline := time.Now().Add(timeout)
	interval := pollInterval

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out %s (timeout: %s)", timeoutMsg, timeout)
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
func (o *OpenBaoBootstrapper) ensureNamespace() error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: o.Config.Namespace,
		},
	}

	_, err := o.Clientset.CoreV1().Namespaces().Get(o.ctx, o.Config.Namespace, metav1.GetOptions{})
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return fmt.Errorf("checking namespace %s: %w", o.Config.Namespace, err)
		}
		_, err = o.Clientset.CoreV1().Namespaces().Create(o.ctx, ns, metav1.CreateOptions{})
		if err != nil && !k8serrors.IsAlreadyExists(err) {
			return fmt.Errorf("creating namespace %s: %w", o.Config.Namespace, err)
		}
	}
	return nil
}

// drBackup represents the structure of the SOPS-encrypted DR backup file.
type drBackup struct {
	UnsealKeys map[string]string `json:"unseal_keys"`
	Timestamp  string            `json:"timestamp"`
}
