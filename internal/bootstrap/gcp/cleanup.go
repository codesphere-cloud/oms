// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/codesphere-cloud/oms/internal/bootstrap"
	"github.com/codesphere-cloud/oms/internal/util"
)

// CleanupOpts holds the user-provided options for the cleanup operation.
type CleanupOpts struct {
	ProjectID      string
	Force          bool
	SkipDNSCleanup bool
	BaseDomain     string
	DNSZoneName    string
	DNSProjectID   string
}

// CleanupDeps holds the injectable dependencies for the cleanup operation.
type CleanupDeps struct {
	GCPClient     GCPClientManager
	FileIO        util.FileIO
	StepLogger    *bootstrap.StepLogger
	ConfirmReader io.Reader
	InfraFilePath string
}

// CleanupExecutor manages state and logic for each cleanup step.
type CleanupExecutor struct {
	Opts            *CleanupOpts
	Deps            *CleanupDeps
	ProjectID       string
	InfraEnv        CodesphereEnvironment
	InfraFileLoaded bool
	BaseDomain      string
	DNSZoneName     string
	DNSProjectID    string
}

// NewCleanupExecutor resolves configuration from options and the infra file,
// returning an executor ready to run the cleanup steps.
func NewCleanupExecutor(opts *CleanupOpts, deps *CleanupDeps) (*CleanupExecutor, error) {
	exec := &CleanupExecutor{
		Opts:      opts,
		Deps:      deps,
		ProjectID: opts.ProjectID,
	}
	if err := exec.loadInfraFileIfNeeded(); err != nil {
		return nil, err
	}
	if err := exec.resolveProjectID(); err != nil {
		return nil, err
	}
	exec.resolveDNSSettings()
	return exec, nil
}

// loadInfraFileIfNeeded loads the infra file when the project ID or DNS info is missing.
func (e *CleanupExecutor) loadInfraFileIfNeeded() error {
	missingDNSProjectID := e.Opts.DNSProjectID == ""
	missingDNSInfo := missingDNSProjectID
	if !e.Opts.SkipDNSCleanup {
		missingDNSInfo = missingDNSProjectID || e.Opts.BaseDomain == "" || e.Opts.DNSZoneName == ""
	}
	if e.ProjectID != "" && !missingDNSInfo {
		return nil
	}

	infraEnv, infraFileExists, err := LoadInfraFile(e.Deps.FileIO, e.Deps.InfraFilePath)
	if err != nil {
		if e.ProjectID == "" {
			return fmt.Errorf("failed to load infra file: %w", err)
		}
		log.Printf("Warning: %v", err)
		return nil
	}

	if infraEnv.ProjectID != "" {
		e.InfraEnv = infraEnv
		e.InfraFileLoaded = true
		return nil
	}

	if infraFileExists && e.ProjectID == "" {
		return fmt.Errorf("infra file at %s contains empty project ID", e.Deps.InfraFilePath)
	}

	return nil
}

// resolveProjectID determines the project ID from the flag or the infra file.
func (e *CleanupExecutor) resolveProjectID() error {
	if e.ProjectID != "" {
		if e.InfraFileLoaded && e.InfraEnv.ProjectID != e.ProjectID {
			log.Printf("Warning: infra file contains project ID '%s' but deleting '%s'; ignoring infra file for DNS cleanup", e.InfraEnv.ProjectID, e.ProjectID)
			e.InfraEnv = CodesphereEnvironment{}
			e.InfraFileLoaded = false
		}
		return nil
	}

	if e.InfraEnv.ProjectID == "" {
		return fmt.Errorf("no project ID provided and no infra file found at %s", e.Deps.InfraFilePath)
	}

	e.ProjectID = e.InfraEnv.ProjectID
	log.Printf("Using project ID from infra file: %s", e.ProjectID)
	return nil
}

// resolveDNSSettings resolves DNS configuration from flags with infra file fallback.
func (e *CleanupExecutor) resolveDNSSettings() {
	e.BaseDomain = e.Opts.BaseDomain
	if e.BaseDomain == "" {
		e.BaseDomain = e.InfraEnv.BaseDomain
	}
	e.DNSZoneName = e.Opts.DNSZoneName
	if e.DNSZoneName == "" {
		e.DNSZoneName = e.InfraEnv.DNSZoneName
	}
	e.DNSProjectID = e.Opts.DNSProjectID
	if e.DNSProjectID == "" {
		e.DNSProjectID = e.InfraEnv.DNSProjectID
	}
	if e.DNSProjectID == "" {
		e.DNSProjectID = e.ProjectID
	}
}

// VerifyAndConfirm checks that the project is OMS-managed and prompts the user
// for deletion confirmation, unless Force is set.
func (e *CleanupExecutor) VerifyAndConfirm() error {
	if e.Opts.Force {
		log.Printf("Skipping OMS-managed verification and deletion confirmation (--force flag used)")
		return nil
	}

	isOMSManaged, err := e.Deps.GCPClient.IsOMSManagedProject(e.ProjectID)
	if err != nil {
		return fmt.Errorf("failed to verify project: %w", err)
	}
	if !isOMSManaged {
		return fmt.Errorf("project %s was not bootstrapped by OMS (missing 'oms-managed' label). Use --force to override this check", e.ProjectID)
	}

	return e.confirmDeletion()
}

func (e *CleanupExecutor) confirmDeletion() error {
	log.Printf("WARNING: This will permanently delete the GCP project '%s' and all its resources.", e.ProjectID)
	log.Printf("This action cannot be undone.\n")
	log.Println("Type the project ID to confirm deletion: ")

	reader := bufio.NewReader(e.Deps.ConfirmReader)
	confirmation, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read confirmation: %w", err)
	}
	if strings.TrimSpace(confirmation) != e.ProjectID {
		return fmt.Errorf("confirmation did not match project ID, aborting cleanup")
	}
	return nil
}

// CleanupDNSRecords deletes OMS-created DNS records if DNS cleanup is enabled
// and the required DNS information is available.
func (e *CleanupExecutor) CleanupDNSRecords() error {
	if e.Opts.SkipDNSCleanup {
		return nil
	}
	if e.BaseDomain == "" || e.DNSZoneName == "" {
		log.Printf("Skipping DNS cleanup: missing base domain or DNS zone name (provide --base-domain/--dns-zone-name or use --skip-dns-cleanup)")
		return nil
	}
	return e.Deps.GCPClient.DeleteDNSRecordSets(e.DNSProjectID, e.DNSZoneName, e.BaseDomain)
}

// RemoveDNSIAMBinding removes the cloud-controller service account's IAM binding
// from the DNS project. This is independent of SkipDNSCleanup.
func (e *CleanupExecutor) RemoveDNSIAMBinding() error {
	if e.DNSProjectID == "" || e.DNSProjectID == e.ProjectID {
		return nil
	}
	return e.Deps.GCPClient.RemoveIAMRoleBinding(e.DNSProjectID, "cloud-controller", e.ProjectID, []string{"roles/dns.admin"})
}

// DeleteProject deletes the GCP project.
func (e *CleanupExecutor) DeleteProject() error {
	return e.Deps.GCPClient.DeleteProject(e.ProjectID)
}

// RemoveLocalInfraFile removes the local infra file if it matches the deleted project.
func (e *CleanupExecutor) RemoveLocalInfraFile() {
	if !e.InfraFileLoaded || e.InfraEnv.ProjectID != e.ProjectID {
		return
	}
	if err := e.Deps.FileIO.Remove(e.Deps.InfraFilePath); err != nil {
		log.Printf("Warning: failed to remove local infra file: %v", err)
		return
	}
	log.Printf("Removed local infra file: %s", e.Deps.InfraFilePath)
}
