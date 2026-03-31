// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// OMS Project Label Keys
const (
	OMSManagedLabel     = "oms-managed"
	DeleteAfterLabel    = "delete-after"
	InstallVersionLabel = "install-version"
	InstallHashLabel    = "install-hash"
)

// EnsureProject creates or updates an existing GCP project with labels
func (b *GCPBootstrapper) EnsureProject() error {
	parent := ""
	if b.Env.FolderID != "" {
		parent = fmt.Sprintf("folders/%s", b.Env.FolderID)
	}

	labels, err := b.generateProjectLabels()
	if err != nil {
		return fmt.Errorf("failed to generate project labels: %w", err)
	}

	existingProject, err := b.GCPClient.GetProjectByName(b.Env.FolderID, b.Env.ProjectName)
	if err == nil {
		b.Env.ProjectID = existingProject.ProjectId
		b.Env.ProjectName = existingProject.Name

		err := b.GCPClient.UpdateProject(existingProject.ProjectId, labels)
		if err != nil {
			return fmt.Errorf("failed to update project: %w", err)
		}

		return nil
	}

	if err.Error() == fmt.Sprintf("project not found: %s", b.Env.ProjectName) {
		projectId := b.GCPClient.CreateProjectID(b.Env.ProjectName)

		_, err = b.GCPClient.CreateProject(parent, projectId, b.Env.ProjectName, labels)
		if err != nil {
			return fmt.Errorf("failed to create project: %w", err)
		}

		b.Env.ProjectID = projectId

		return nil
	}

	return fmt.Errorf("failed to get project: %w", err)
}

// generateProjectLabels creates a map of GCP project labels
// returns an error if "delete-after" label can not be generated
func (b *GCPBootstrapper) generateProjectLabels() (map[string]string, error) {
	labels := make(map[string]string)
	labels[OMSManagedLabel] = "true"

	installVersionLabel, err := createLabel(b.Env.InstallVersion)
	if err == nil {
		labels[InstallVersionLabel] = installVersionLabel
	}

	installHashLabel, err := createLabel(b.Env.InstallHash)
	if err == nil {
		labels[InstallHashLabel] = installHashLabel
	}

	deleteProjectAfter, err := calculateProjectExpiryLabel(b.Env.ProjectTTL)
	if err != nil {
		return labels, fmt.Errorf("failed to calculate project expiry label: %w", err)
	}

	deleteProjectAfterLabel, err := createLabel(deleteProjectAfter)
	if err != nil {
		return nil, fmt.Errorf("failed to create '%s' label: %w", DeleteAfterLabel, err)
	}

	labels[DeleteAfterLabel] = deleteProjectAfterLabel

	return labels, nil
}

// createLabel replaces invalid label characters to create a valid GCP label
// returns an error if value is empty
func createLabel(value string) (string, error) {
	if len(value) < 1 {
		return "", fmt.Errorf("value is empty")
	}

	label := value
	if len(label) > 64 {
		label = label[:64]
	}

	invalidChars := []string{"/", "."}
	for _, char := range invalidChars {
		label = strings.ReplaceAll(label, char, "_")
	}

	label = strings.ToLower(label)

	labelRegexFormat := `^[a-z0-9_-]{0,64}$`
	if !regexp.MustCompile(labelRegexFormat).MatchString(label) {
		return "", fmt.Errorf("label '%s' does not match regex '%s'", label, labelRegexFormat)
	}

	return label, nil
}

// calculateProjectExpiryLabel takes a TTL string (e.g. "24h") and
// returns a formatted UTC timestamp string that is usable as a GCP project label for automatic deletion.
func calculateProjectExpiryLabel(projectTTLStr string) (string, error) {
	projectTTL, err := time.ParseDuration(projectTTLStr)
	if err != nil {
		return "", fmt.Errorf("invalid project TTL format: %w", err)
	}

	// prepare label for gcp project deletion in custom UTC time format.
	// GCP Labels are very limited. This is an easy way to add date and TZ info in one label.
	gcpExpiryLabelLayout := "2006-01-02_15-04-05"
	deleteProjectAfter := time.Now().UTC().Add(projectTTL).Format(gcpExpiryLabelLayout)
	deleteProjectAfter = fmt.Sprintf("%s_utc", deleteProjectAfter)

	return deleteProjectAfter, nil
}

// EnsureBilling connects the GCP project with an existing billing account.
// Doesn't change anything if billing account is already connected.
func (b *GCPBootstrapper) EnsureBilling() error {
	bi, err := b.GCPClient.GetBillingInfo(b.Env.ProjectID)
	if err != nil {
		return fmt.Errorf("failed to get billing info: %w", err)
	}
	if bi.BillingEnabled && bi.BillingAccountName == b.Env.BillingAccount {
		return nil
	}

	err = b.GCPClient.EnableBilling(b.Env.ProjectID, b.Env.BillingAccount)
	if err != nil {
		return fmt.Errorf("failed to enable billing: %w", err)
	}

	return nil
}

// EnsureAPIsEnabled enables all required GCP APIs.
// Doesn't change anything if API's already enabled in the project.
func (b *GCPBootstrapper) EnsureAPIsEnabled() error {
	apis := []string{
		"compute.googleapis.com",
		"serviceusage.googleapis.com",
		"artifactregistry.googleapis.com",
		"dns.googleapis.com",
	}

	err := b.GCPClient.EnableAPIs(b.Env.ProjectID, apis)
	if err != nil {
		return fmt.Errorf("failed to enable APIs: %w", err)
	}

	return nil
}

// EnsureServiceAccounts creates the required service account and keys.
func (b *GCPBootstrapper) EnsureServiceAccounts() error {
	_, _, err := b.GCPClient.CreateServiceAccount(b.Env.ProjectID, "cloud-controller", "cloud-controller")
	if err != nil {
		return err
	}

	if b.Env.RegistryType == RegistryTypeArtifactRegistry {
		sa, newSa, err := b.GCPClient.CreateServiceAccount(b.Env.ProjectID, "artifact-registry-writer", "artifact-registry-writer")
		if err != nil {
			return err
		}

		if !newSa && b.Env.InstallConfig.Registry.Password != "" {
			return nil
		}

		for retries := range 5 {
			privateKey, err := b.GCPClient.CreateServiceAccountKey(b.Env.ProjectID, sa)

			if err != nil && status.Code(err) != codes.AlreadyExists {
				if retries > 3 {
					return fmt.Errorf("failed to create service account key: %w", err)
				}
				b.stlog.LogRetry()
				b.Time.Sleep(5 * time.Second)
				continue
			}

			b.Env.InstallConfig.Registry.Password = string(privateKey)
			b.Env.InstallConfig.Registry.Username = "_json_key_base64"

			break
		}
	}

	return nil
}

// EnsureIAMRoles assigns all required IAM roles to the previously created SA's
func (b *GCPBootstrapper) EnsureIAMRoles() error {
	err := b.ensureIAMRoleWithRetry(b.Env.ProjectID, "cloud-controller", b.Env.ProjectID, []string{"roles/compute.admin"})
	if err != nil {
		return fmt.Errorf("failed to ensure cloud-controller role bindings: %w", err)
	}

	err = b.ensureDnsPermissions()
	if err != nil {
		return fmt.Errorf("failed to ensure DNS permissions: %w", err)
	}

	if b.Env.RegistryType != RegistryTypeArtifactRegistry {
		return nil
	}

	err = b.ensureIAMRoleWithRetry(b.Env.ProjectID, "artifact-registry-writer", b.Env.ProjectID, []string{"roles/artifactregistry.writer"})
	if err != nil {
		return fmt.Errorf("failed to ensure artifact-registry-writer role bindings: %w", err)
	}

	return nil
}

// ensureIAMRoleWithRetry assigns a list of roles to an existing service account.
// Will try to assign the role up to 5 times before failing to cover expected Google API delays.
func (b *GCPBootstrapper) ensureIAMRoleWithRetry(projectID string, serviceAccount string, serviceAccountProjectID string, roles []string) error {
	var err error
	for retries := range 5 {
		err = b.GCPClient.AssignIAMRole(projectID, serviceAccount, serviceAccountProjectID, roles)
		if err == nil {
			return nil
		}

		if retries < 4 {
			b.stlog.LogRetry()
			b.Time.Sleep(5 * time.Second)
		}
	}

	return fmt.Errorf("failed to assign roles %v to service account %s: %w", roles, serviceAccount, err)
}
