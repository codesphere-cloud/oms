// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"fmt"

	"github.com/codesphere-cloud/oms/internal/bootstrap/datacenter"
	"google.golang.org/api/dns/v1"
)

// DNSRecordName identifies a DNS record set that OMS manages.
type DNSRecordName struct {
	Name  string `json:"name"`
	Rtype string `json:"rtype"`
}

// GetDNSRecordNames returns the DNS record names a single-data-center bootstrap creates for a
// given base domain. It is the fallback for infra files written before multi-DC support, which
// do not record the created records.
func GetDNSRecordNames(baseDomain string) []DNSRecordName {
	return []DNSRecordName{
		{fmt.Sprintf("cs.%s.", baseDomain), "A"},
		{fmt.Sprintf("*.cs.%s.", baseDomain), "A"},
		{fmt.Sprintf("ws.%s.", baseDomain), "A"},
		{fmt.Sprintf("*.ws.%s.", baseDomain), "A"},
		{fmt.Sprintf("*.ssh.cs.%s.", baseDomain), "A"},
	}
}

// DataCenterDNSRecordNames returns every DNS record OMS creates for the given data center
// layout: the shared platform gateway names plus each data center's workspace and SSH names.
func DataCenterDNSRecordNames(baseDomain string, dcs []*datacenter.DataCenter) []DNSRecordName {
	records := []DNSRecordName{
		{fmt.Sprintf("cs.%s.", baseDomain), "A"},
		{fmt.Sprintf("*.cs.%s.", baseDomain), "A"},
	}
	for _, dc := range dcs {
		records = append(records,
			DNSRecordName{fmt.Sprintf("%s.", dc.WorkspaceHostingBaseDomain), "A"},
			DNSRecordName{fmt.Sprintf("*.%s.", dc.WorkspaceHostingBaseDomain), "A"},
			DNSRecordName{fmt.Sprintf("*.%s.", dc.SSHBaseDomain), "A"},
		)
		if len(dcs) > 1 {
			records = append(records,
				DNSRecordName{fmt.Sprintf("%s.", dc.PlatformDomain(baseDomain)), "A"},
				DNSRecordName{fmt.Sprintf("*.%s.", dc.PlatformDomain(baseDomain)), "A"},
			)
		}
	}

	return records
}

func (b *GCPBootstrapper) ensureDnsPermissions() error {
	dnsProject := b.Env.DNSProjectID
	if b.Env.DNSProjectID == "" {
		dnsProject = b.Env.ProjectID
	}

	err := b.ensureIAMRoleWithRetry(dnsProject, "cloud-controller", b.Env.ProjectID, []string{"roles/dns.admin"})
	if err != nil {
		return err
	}

	return nil
}

func (b *GCPBootstrapper) EnsureDNSRecords() error {
	if err := b.ensureDataCenters(); err != nil {
		return err
	}

	gcpProject := b.Env.DNSProjectID
	if b.Env.DNSProjectID == "" {
		gcpProject = b.Env.ProjectID
	}

	zoneName := b.Env.DNSZoneName

	err := b.GCPClient.EnsureDNSManagedZone(gcpProject, zoneName, b.Env.BaseDomain+".", "Codesphere DNS zone")
	if err != nil {
		return fmt.Errorf("failed to ensure DNS managed zone: %w", err)
	}

	// The platform is served from one domain shared by all data centers, pointing at the
	// primary data center's gateway.
	records := []*dns.ResourceRecordSet{
		dnsARecord(fmt.Sprintf("cs.%s.", b.Env.BaseDomain), b.primaryDC().GatewayIP),
		dnsARecord(fmt.Sprintf("*.cs.%s.", b.Env.BaseDomain), b.primaryDC().GatewayIP),
	}
	// Workspaces and their SSH endpoints resolve per data center, so each one gets its own
	// names pointing at its own public gateway and SSH proxy.
	for _, dc := range b.Env.DataCenters {
		records = append(records,
			dnsARecord(fmt.Sprintf("%s.", dc.WorkspaceHostingBaseDomain), dc.PublicGatewayIP),
			dnsARecord(fmt.Sprintf("*.%s.", dc.WorkspaceHostingBaseDomain), dc.PublicGatewayIP),
			dnsARecord(fmt.Sprintf("*.%s.", dc.SSHBaseDomain), dc.SSHProxyIP),
		)
		// The platform calls each data center's own services at <dc-id>.cs.<base-domain>, which
		// the wildcard above would send to the primary data center's gateway. A single data
		// center is that primary, so it needs no record of its own.
		if len(b.Env.DataCenters) > 1 {
			platformDomain := dc.PlatformDomain(b.Env.BaseDomain)
			records = append(records,
				dnsARecord(fmt.Sprintf("%s.", platformDomain), dc.GatewayIP),
				dnsARecord(fmt.Sprintf("*.%s.", platformDomain), dc.GatewayIP),
			)
		}
	}

	err = b.GCPClient.EnsureDNSRecordSets(gcpProject, zoneName, records)
	if err != nil {
		return fmt.Errorf("failed to ensure DNS record sets: %w", err)
	}

	// Record what was created so cleanup deletes exactly these records instead of recomputing
	// the list from the base domain.
	b.Env.DNSRecords = DataCenterDNSRecordNames(b.Env.BaseDomain, b.Env.DataCenters)

	return nil
}

// dnsARecord builds a short-TTL A record set, as used during initial setup.
func dnsARecord(name, ip string) *dns.ResourceRecordSet {
	return &dns.ResourceRecordSet{
		Name:    name,
		Type:    "A",
		Ttl:     300,
		Rrdatas: []string{ip},
	}
}
