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
	"strings"
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
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
)

//go:embed manifests/openbao/vault-cr.yaml
var vaultCRTemplate []byte

const (
	openBaoUnsealSecretName = "openbao-unseal-keys"
	openBaoHeadlessService  = "openbao"
	DefaultOpenBaoNamespace = "vault"
	openBaoImage            = "quay.io/openbao/openbao:2.1.0"
	bankVaultsImage         = "ghcr.io/bank-vaults/bank-vaults:v1.31.3"
	bankVaultsChartRepo     = "oci://ghcr.io/bank-vaults/helm-charts"
	bankVaultsChartName     = "vault-operator"
	bankVaultsChartVersion  = "1.22.5"
	defaultPasswordLength   = 32
	pollInterval            = 5 * time.Second
	maxPollInterval         = 30 * time.Second

	// vaultCRLabelKey/Value identify resources (pods, PVCs) managed by the
	// bank-vaults operator for our "openbao" Vault CR.
	vaultCRLabelKey   = "vault_cr"
	vaultCRLabelValue = "openbao"
)

// vaultCRLabelSelector selects all resources belonging to the "openbao" Vault CR.
var vaultCRLabelSelector = labels.SelectorFromSet(labels.Set{vaultCRLabelKey: vaultCRLabelValue}).String()

// OpenBaoInstallerConfig holds all configurable parameters for the OpenBao bootstrap.
type OpenBaoInstallerConfig struct {
	Namespace         string
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

	// ConfirmFunc is called when the destructive fresh-install path is about
	// to proceed (no DR backup found). If it returns an error the install is
	// aborted. When nil the install proceeds without confirmation.
	ConfirmFunc func() error

	// Intermediate state populated during the install pipeline
	ctx              context.Context
	password         string
	drBackupExists   bool
	unsealSecret     *corev1.Secret
	backupUnsealKeys map[string][]byte // unseal keys from DR backup, used during WaitForInitialization
}

// NewOpenBaoInstaller constructs an OpenBaoInstaller with real Kubernetes and Helm clients.
func NewOpenBaoInstaller(cfg OpenBaoInstallerConfig) (*OpenBaoInstaller, error) {
	// Apply namespace default here so the Helm client is always initialised with
	// the correct namespace, even when --namespace was not supplied by the caller.
	if cfg.Namespace == "" {
		cfg.Namespace = DefaultOpenBaoNamespace
	}

	helm, err := NewHelmClient(cfg.Namespace)
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
	if o.Config.Namespace == "" {
		o.Config.Namespace = DefaultOpenBaoNamespace
	}
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

	// Only warn when an existing deployment is detected but no DR backup was
	// found — the user likely supplied the wrong backup path. A genuine first
	// install (no existing deployment) proceeds without prompting.
	if !o.drBackupExists && o.ConfirmFunc != nil {
		exists, checkErr := o.hasExistingDeployment()
		if checkErr != nil {
			return fmt.Errorf("checking for existing deployment: %w", checkErr)
		}
		if exists {
			if err := o.ConfirmFunc(); err != nil {
				return err
			}
		}
	}

	// Only generate a new password for fresh installs; on DR restore the
	// password was already extracted from the backup in PreFlightDRCheck.
	if !o.drBackupExists {
		err = o.Logger.Step("Generating secure password", o.GeneratePassword)
		if err != nil {
			return fmt.Errorf("failed to generate secure password: %w", err)
		}
	}

	// Ensure the namespace exists before anything that writes into it (the Helm
	// release metadata, the Vault CR, cleanup of stale resources). This is the
	// single namespace-creation path — DeployBankVaultsOperator deploys with
	// CreateNamespace:false and relies on the namespace already being present.
	err = o.Logger.Step("Ensuring namespace exists", func() error {
		return o.ensureNamespace(o.ctx)
	})
	if err != nil {
		return fmt.Errorf("failed to ensure namespace: %w", err)
	}

	err = o.Logger.Step("Deploying Bank-Vaults Operator", o.DeployBankVaultsOperator)
	if err != nil {
		return fmt.Errorf("failed to deploy Bank-Vaults Operator: %w", err)
	}

	// If a previous install left behind an unseal-keys Secret (e.g. Raft storage
	// was wiped or the cluster was rebuilt), those keys belong to the old master
	// key and will cause bank-vaults to permanently fail unsealing the new instance.
	// We clean the full prior install state: Vault CR, pods, PVCs, and the secret.
	if !o.drBackupExists {
		err = o.Logger.Step("Cleaning stale install state", o.CleanStaleInstallState)
		if err != nil {
			return fmt.Errorf("failed to clean stale install state: %w", err)
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

	err = o.Logger.Step("Waiting for all OpenBao pods to be ready", o.WaitForPodsReady)
	if err != nil {
		return fmt.Errorf("failed waiting for pods to be ready: %w", err)
	}

	err = o.Logger.Step("Extracting and encrypting DR backup", o.ExtractAndEncrypt)
	if err != nil {
		return fmt.Errorf("failed to extract and encrypt DR backup: %w", err)
	}

	o.Logger.Logf("OpenBao bootstrap complete. DR backup saved to: %s", o.Config.DRBackupPath)
	return nil
}

// PreFlightDRCheck checks if a SOPS-encrypted DR backup exists.
// If it does, the backup is decrypted and the unseal keys are stored in memory
// (backupUnsealKeys) for later use by WaitForInitialization, which handles
// creating/updating the Kubernetes Secret with retry logic.
// Sets o.drBackupExists to true if a DR backup was found and processed.
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

	o.Logger.Logf("Found existing DR backup at %s", o.Config.DRBackupPath)

	decrypted, err := DecryptFileWithSOPS(o.Config.DRBackupPath, o.Config.AgeKeyPath)
	if err != nil {
		return err
	}

	var backup drBackup
	if err := json.Unmarshal(decrypted, &backup); err != nil {
		return fmt.Errorf("parsing DR backup: %w", err)
	}

	// Store backup unseal keys for later use in WaitForInitialization.
	// We do NOT write them to Kubernetes yet — the operator may delete or
	// recreate the secret during Vault CR reconciliation, so we defer
	// secret creation to the initialization wait loop where we can retry.
	o.backupUnsealKeys = make(map[string][]byte)
	for k, v := range backup.UnsealKeys {
		o.backupUnsealKeys[k] = []byte(v)
	}

	// Reuse the password and username from the DR backup so the Vault CR is
	// rendered with the same credentials that OpenBao already has configured.
	if o.Config.Username != backup.Username {
		o.Logger.Logf("Warning: --bao-user=%q differs from DR backup username %q — using backup value", o.Config.Username, backup.Username)
	}
	o.password = backup.Password
	o.Config.Username = backup.Username

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
//
// The operator is cluster-scoped (it creates ClusterRoles, ClusterRoleBindings)
// and watches Vault CRs across all namespaces. If the operator is already
// installed in a different namespace, we skip re-deployment — one instance
// is sufficient for the entire cluster.
func (o *OpenBaoInstaller) DeployBankVaultsOperator() error {
	cfg := ChartConfig{
		ReleaseName: "vault-operator",
		ChartName:   bankVaultsChartRepo + "/" + bankVaultsChartName,
		Version:     bankVaultsChartVersion,
		Namespace:   o.Config.Namespace,
		// Namespace creation is handled exclusively by ensureNamespace, which
		// runs earlier in the install pipeline — keep a single creation path.
		CreateNamespace: false,
		Values:          map[string]interface{}{},
	}

	// Upgrade in place when a release already exists in the target namespace.
	exists, err := o.releaseExistsInTargetNamespace(cfg.ReleaseName)
	if err != nil {
		return err
	}
	if exists {
		return o.Helm.UpgradeChart(o.ctx, cfg, UpgradeChartOptions{})
	}

	// Release not found in target namespace. Skip when the operator is already
	// deployed cluster-wide (in another namespace) — one instance suffices.
	clusterWide, err := o.operatorInstalledClusterWide()
	if err != nil {
		return err
	}
	if clusterWide {
		o.Logger.Logf("Bank-Vaults Operator already installed in the cluster, skipping deployment")
		return nil
	}

	// Operator does not exist — perform fresh install.
	return o.Helm.InstallChart(o.ctx, cfg, InstallChartOptions{})
}

// releaseExistsInTargetNamespace reports whether the named Helm release exists
// in the configured namespace. If the namespace does not exist yet there can be
// no release in it, so it returns false without querying Helm (which would fail
// on a non-existent namespace).
func (o *OpenBaoInstaller) releaseExistsInTargetNamespace(releaseName string) (bool, error) {
	_, nsErr := o.Clientset.CoreV1().Namespaces().Get(o.ctx, o.Config.Namespace, metav1.GetOptions{})
	if nsErr != nil {
		if k8serrors.IsNotFound(nsErr) {
			return false, nil
		}
		return false, fmt.Errorf("checking namespace %s: %w", o.Config.Namespace, nsErr)
	}

	rel, err := o.Helm.FindRelease(o.Config.Namespace, releaseName)
	if err != nil {
		return false, fmt.Errorf("finding release %s in namespace %s: %w", releaseName, o.Config.Namespace, err)
	}
	return rel != nil, nil
}

// operatorInstalledClusterWide reports whether the bank-vaults operator is
// already installed anywhere in the cluster, detected via its cluster-scoped
// "vault-operator" ClusterRole.
func (o *OpenBaoInstaller) operatorInstalledClusterWide() (bool, error) {
	_, err := o.Clientset.RbacV1().ClusterRoles().Get(o.ctx, "vault-operator", metav1.GetOptions{})
	if err == nil {
		return true, nil
	}
	if !k8serrors.IsNotFound(err) {
		return false, fmt.Errorf("checking for existing vault-operator ClusterRole: %w", err)
	}
	return false, nil
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
		addr := fmt.Sprintf("http://openbao-%d.%s.%s.svc.cluster.local:8200", i, openBaoHeadlessService, o.Config.Namespace)
		retryJoinAddrs = append(retryJoinAddrs, addr)
	}

	data := vaultCRTemplateData{
		Namespace:         o.Config.Namespace,
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
//
// When a DR backup was loaded (backupUnsealKeys is set), the function ensures
// the secret exists with the backup's unseal keys on every poll iteration. This
// handles the case where the bank-vaults operator deletes or recreates the
// secret during Vault CR reconciliation — we simply re-apply it until the
// operator settles and the sidecar can successfully unseal.
func (o *OpenBaoInstaller) WaitForInitialization() error {
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

// ensureUnsealSecret creates or updates the unseal keys secret from the DR backup.
// It preserves existing metadata (labels, annotations, ownerReferences) when updating.
func (o *OpenBaoInstaller) ensureUnsealSecret(secretsClient corev1client.SecretInterface) error {
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

// WaitForPodsReady polls until the expected number of vault pods (matching the
// configured replica count) are in Running phase with all containers Ready.
// This ensures scaling operations have fully completed before reporting success.
func (o *OpenBaoInstaller) WaitForPodsReady() error {
	selector := vaultCRLabelSelector
	expected := o.Config.Replicas

	return o.pollUntil("waiting for all OpenBao pods to be ready", func() (bool, error) {
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

// CleanStaleInstallState removes all state left by a prior installation whose
// Raft storage no longer exists (e.g. cluster rebuild, PVC deletion). Without
// cleanup, bank-vaults would attempt to unseal with the old master key's shares
// and fail permanently — the new instance needs to run a fresh init.
//
// The cleanup sequence is:
//  1. Delete the Vault CR (stops the bank-vaults sidecar retry loop)
//  2. Wait for all vault pods to terminate
//  3. Delete PVCs (removes stale Raft data that would confuse initialization)
//  4. Delete the unseal-keys Secret
func (o *OpenBaoInstaller) CleanStaleInstallState() error {
	vaultGVR := k8s.VaultGVR()
	var cleaned []string

	// Tolerates NotFound — this may be a first-time install with no prior Vault CR.
	delErr := o.DynClient.Resource(vaultGVR).Namespace(o.Config.Namespace).Delete(
		o.ctx, "openbao", metav1.DeleteOptions{},
	)
	if delErr != nil && !k8serrors.IsNotFound(delErr) {
		return fmt.Errorf("deleting Vault CR: %w", delErr)
	}
	if delErr == nil {
		cleaned = append(cleaned, "Vault CR")
		// Only wait for pods to terminate when we actually deleted a Vault CR —
		// without a CR there are no operator-managed pods to wait on.
		if err := o.waitForVaultPodsGone(); err != nil {
			return err
		}
	}

	// Delete PVCs associated with the prior StatefulSet so that stale Raft
	// data does not cause OpenBao to report as "initialized" on a fresh install.
	pvcList, err := o.Clientset.CoreV1().PersistentVolumeClaims(o.Config.Namespace).List(
		o.ctx, metav1.ListOptions{LabelSelector: vaultCRLabelSelector},
	)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return fmt.Errorf("listing stale PVCs: %w", err)
		}
		// Namespace doesn't exist yet — no stale PVCs to clean.
		pvcList = &corev1.PersistentVolumeClaimList{}
	}
	for i := range pvcList.Items {
		delErr = o.Clientset.CoreV1().PersistentVolumeClaims(o.Config.Namespace).Delete(
			o.ctx, pvcList.Items[i].Name, metav1.DeleteOptions{},
		)
		if delErr != nil && !k8serrors.IsNotFound(delErr) {
			return fmt.Errorf("deleting PVC %s: %w", pvcList.Items[i].Name, delErr)
		}
	}
	if len(pvcList.Items) > 0 {
		cleaned = append(cleaned, fmt.Sprintf("%d PVC(s)", len(pvcList.Items)))
		if err := o.waitForPVCsGone(); err != nil {
			return err
		}
	}

	// Now it is safe to delete the stale secret.
	delErr = o.Clientset.CoreV1().Secrets(o.Config.Namespace).Delete(
		o.ctx, openBaoUnsealSecretName, metav1.DeleteOptions{},
	)
	if delErr != nil && !k8serrors.IsNotFound(delErr) {
		return fmt.Errorf("deleting stale unseal secret: %w", delErr)
	}
	if delErr == nil {
		cleaned = append(cleaned, "unseal secret")
	}

	if len(cleaned) > 0 {
		o.Logger.Logf("Cleaned stale resources: %s", strings.Join(cleaned, ", "))
	} else {
		o.Logger.Logf("No stale install state found in namespace %q", o.Config.Namespace)
	}
	return nil
}

// hasExistingDeployment checks whether an OpenBao deployment already exists
// in the cluster by looking for the Vault CR or PVCs with vault_cr=openbao.
// This is used to distinguish a genuine first install (nothing exists) from a
// re-install where the user may have supplied the wrong DR backup path.
func (o *OpenBaoInstaller) hasExistingDeployment() (bool, error) {
	vaultGVR := k8s.VaultGVR()
	_, err := o.DynClient.Resource(vaultGVR).Namespace(o.Config.Namespace).Get(
		o.ctx, "openbao", metav1.GetOptions{},
	)
	if err == nil {
		return true, nil
	}
	if !k8serrors.IsNotFound(err) {
		return false, fmt.Errorf("checking Vault CR: %w", err)
	}

	// Vault CR gone but PVCs may linger (e.g. CR was manually deleted).
	pvcList, err := o.Clientset.CoreV1().PersistentVolumeClaims(o.Config.Namespace).List(
		o.ctx, metav1.ListOptions{LabelSelector: vaultCRLabelSelector},
	)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return false, nil // Namespace doesn't exist — no prior deployment.
		}
		return false, fmt.Errorf("listing PVCs: %w", err)
	}
	return len(pvcList.Items) > 0, nil
}

// waitForVaultPodsGone polls until no pods with label vault_cr=openbao remain
// in the target namespace, or until the context deadline is exceeded.
func (o *OpenBaoInstaller) waitForVaultPodsGone() error {
	selector := vaultCRLabelSelector

	return o.pollUntil("waiting for vault pods to terminate", func() (bool, error) {
		list, err := o.Clientset.CoreV1().Pods(o.Config.Namespace).List(o.ctx, metav1.ListOptions{
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

// waitForPVCsGone polls until no PVCs with label vault_cr=openbao remain in the
// target namespace. This ensures asynchronous PVC deletion has fully completed
// before the install pipeline creates new resources, avoiding conflicts.
func (o *OpenBaoInstaller) waitForPVCsGone() error {
	selector := vaultCRLabelSelector

	return o.pollUntil("waiting for stale PVCs to be deleted", func() (bool, error) {
		list, err := o.Clientset.CoreV1().PersistentVolumeClaims(o.Config.Namespace).List(
			o.ctx, metav1.ListOptions{LabelSelector: selector},
		)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				return true, nil
			}
			return false, fmt.Errorf("listing PVCs: %w", err)
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
			Name: o.Config.Namespace,
		},
	}

	_, err := o.Clientset.CoreV1().Namespaces().Get(ctx, o.Config.Namespace, metav1.GetOptions{})
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return fmt.Errorf("checking namespace %s: %w", o.Config.Namespace, err)
		}
		_, err = o.Clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
		if err != nil && !k8serrors.IsAlreadyExists(err) {
			return fmt.Errorf("creating namespace %s: %w", o.Config.Namespace, err)
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
