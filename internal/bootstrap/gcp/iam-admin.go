package gcp

import (
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (b *GCPBootstrapper) EnsureProject() error {
	parent := ""
	if b.Env.FolderID != "" {
		parent = fmt.Sprintf("folders/%s", b.Env.FolderID)
	}

	deleteProjectAfter, err := calculateProjectExpiryLabel(b.Env.ProjectTTL)
	if err != nil {
		return fmt.Errorf("failed to calculate project expiry label: %w", err)
	}

	labels := map[string]string{
		OMSManagedLabel:  "true",
		DeleteAfterLabel: deleteProjectAfter,
		InstallVersion:   b.Env.InstallVersion,
		InstallHash:      b.Env.InstallHash,
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

func (b *GCPBootstrapper) EnsureIAMRoles() error {
	err := b.ensureIAMRoleWithRetry(b.Env.ProjectID, "cloud-controller", b.Env.ProjectID, []string{"roles/compute.admin"})
	if err != nil {
		return err
	}

	err = b.ensureDnsPermissions()
	if err != nil {
		return err
	}

	if b.Env.RegistryType != RegistryTypeArtifactRegistry {
		return nil
	}

	err = b.ensureIAMRoleWithRetry(b.Env.ProjectID, "artifact-registry-writer", b.Env.ProjectID, []string{"roles/artifactregistry.writer"})
	return err
}

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
