// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ConfigManagerAnsible", func() {
	var (
		manager           installer.InstallConfigManager
		tempDir           string
		inventoryFilePath string
	)

	BeforeEach(func() {
		manager = installer.NewInstallConfigManager()

		tempDir = GinkgoT().TempDir()
		inventoryFilePath = filepath.Join(tempDir, "inventory.yaml")
	})

	Describe("FetchFromAnsibleInventory", func() {
		Context("inventory file does not exist", func() {
			It("returns an error", func() {
				err := manager.FetchFromAnsibleInventory(inventoryFilePath)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to read Ansible inventory file"))
			})
		})

		Context("inventory file is empty", func() {
			It("returns an error", func() {
				file, err := os.Create(inventoryFilePath)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = os.Remove(inventoryFilePath) }()

				_, err = file.Write([]byte(""))
				Expect(err).ToNot(HaveOccurred())

				err = manager.FetchFromAnsibleInventory(inventoryFilePath)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("empty Ansible inventory file"))
			})
		})

		Context("inventory file is invalid", func() {
			It("returns an error", func() {
				file, err := os.Create(inventoryFilePath)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = os.Remove(inventoryFilePath) }()

				_, err = file.Write([]byte("{"))
				Expect(err).ToNot(HaveOccurred())

				err = manager.FetchFromAnsibleInventory(inventoryFilePath)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to unmarshal Ansible inventory file"))
			})
		})

		Context("inventory has invalid ceph config", func() {
			It("returns an error for missing host variables", func() {
				file, err := os.Create(inventoryFilePath)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = os.Remove(inventoryFilePath) }()

				inputInventoryYaml := `ceph:
				hosts:
					cs-ceph-1:`
				inputInventory := strings.ReplaceAll(inputInventoryYaml, "\t", "  ")

				_, err = file.Write([]byte(inputInventory))
				Expect(err).ToNot(HaveOccurred())

				err = manager.FetchFromAnsibleInventory(inventoryFilePath)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to fetch ceph hosts from inventory"))
				Expect(err.Error()).To(ContainSubstring("missing private_ip for ceph host 'cs-ceph-1'"))
			})

			It("returns an error for typo in private_ip", func() {
				file, err := os.Create(inventoryFilePath)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = os.Remove(inventoryFilePath) }()

				inputInventoryYaml := `ceph:
				hosts:
					cs-ceph-1:
						private_up: 1`
				inputInventory := strings.ReplaceAll(inputInventoryYaml, "\t", "  ")

				_, err = file.Write([]byte(inputInventory))
				Expect(err).ToNot(HaveOccurred())

				err = manager.FetchFromAnsibleInventory(inventoryFilePath)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to fetch ceph hosts from inventory"))
				Expect(err.Error()).To(ContainSubstring("missing private_ip for ceph host 'cs-ceph-1'"))
			})
		})

		Context("inventory has ceph config", func() {
			It("creates a host list in the config", func() {
				file, err := os.Create(inventoryFilePath)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = os.Remove(inventoryFilePath) }()

				inputInventory := `ceph:
  hosts:
    cs-ceph-1:
      private_ip: 1.2.3.4
    cs-ceph-2:
      private_ip: 1.2.3.5
    cs-ceph-3:
      private_ip: 1.2.3.6`

				_, err = file.Write([]byte(inputInventory))
				Expect(err).ToNot(HaveOccurred())

				err = manager.FetchFromAnsibleInventory(inventoryFilePath)
				Expect(err).ToNot(HaveOccurred())

				expectedCephHosts := []files.CephHost{
					{
						Hostname:  "cs-ceph-1",
						IPAddress: "1.2.3.4",
						IsMaster:  true,
					},
					{
						Hostname:  "cs-ceph-2",
						IPAddress: "1.2.3.5",
						IsMaster:  false,
					},
					{
						Hostname:  "cs-ceph-3",
						IPAddress: "1.2.3.6",
						IsMaster:  false,
					},
				}

				actualCephHosts := manager.GetInstallConfig().Ceph.Hosts
				Expect(actualCephHosts).To(Equal(expectedCephHosts))

				actualK8sCPHosts := manager.GetInstallConfig().Kubernetes.ControlPlanes
				Expect(actualK8sCPHosts).To(BeEmpty())
				actualK8sWorkers := manager.GetInstallConfig().Kubernetes.Workers
				Expect(actualK8sWorkers).To(BeEmpty())
			})

			It("converts any value into string", func() {
				file, err := os.Create(inventoryFilePath)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = os.Remove(inventoryFilePath) }()

				inputInventoryYaml := `ceph:
				hosts:
					cs-ceph-1:
						private_ip: true`
				inputInventory := strings.ReplaceAll(inputInventoryYaml, "\t", "  ")

				_, err = file.Write([]byte(inputInventory))
				Expect(err).ToNot(HaveOccurred())

				err = manager.FetchFromAnsibleInventory(inventoryFilePath)
				Expect(err).ToNot(HaveOccurred())
			})

			It("keeps pre-existing control plan config, if inventory has none", func() {
				file, err := os.Create(inventoryFilePath)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = os.Remove(inventoryFilePath) }()

				inputInventory := `ceph:
  hosts:
    cs-ceph-1:
      private_ip: 1.2.3.4
    cs-ceph-2:
      private_ip: 1.2.3.5
    cs-ceph-3:
      private_ip: 1.2.3.6`

				_, err = file.Write([]byte(inputInventory))
				Expect(err).ToNot(HaveOccurred())

				manager.GetInstallConfig().Kubernetes.ControlPlanes = []files.K8sNode{
					{
						IPAddress: "1.1.1.1",
					},
				}
				manager.GetInstallConfig().Kubernetes.Workers = []files.K8sNode{
					{
						IPAddress: "1.1.1.2",
					},
				}

				err = manager.FetchFromAnsibleInventory(inventoryFilePath)
				Expect(err).ToNot(HaveOccurred())

				expectedCephHosts := []files.CephHost{
					{
						Hostname:  "cs-ceph-1",
						IPAddress: "1.2.3.4",
						IsMaster:  true,
					},
					{
						Hostname:  "cs-ceph-2",
						IPAddress: "1.2.3.5",
						IsMaster:  false,
					},
					{
						Hostname:  "cs-ceph-3",
						IPAddress: "1.2.3.6",
						IsMaster:  false,
					},
				}

				actualCephHosts := manager.GetInstallConfig().Ceph.Hosts
				Expect(actualCephHosts).To(Equal(expectedCephHosts))

				expectedCPHosts := []files.K8sNode{
					{
						IPAddress: "1.1.1.1",
					},
				}

				expectedWorkerHosts := []files.K8sNode{
					{
						IPAddress: "1.1.1.2",
					},
				}

				actualK8sCPHosts := manager.GetInstallConfig().Kubernetes.ControlPlanes
				Expect(actualK8sCPHosts).To(Equal(expectedCPHosts))

				actualK8sWorkerHosts := manager.GetInstallConfig().Kubernetes.Workers
				Expect(actualK8sWorkerHosts).To(Equal(expectedWorkerHosts))
			})
		})

		Context("inventory has k8s config", func() {
			It("creates a host list in the config", func() {
				file, err := os.Create(inventoryFilePath)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = os.Remove(inventoryFilePath) }()

				inputInventory := `k8s-cp:
  hosts:
    cs-k8s-cp-1:
      private_ip: 1.2.3.4
    cs-k8s-cp-2:
      private_ip: 1.2.3.5
    cs-k8s-cp-3:
      private_ip: 1.2.3.6
k8s-workers:
  hosts:
    cs-k8s-worker-1:
      private_ip: 1.2.3.7
    cs-k8s-worker-2:
      private_ip: 1.2.3.8
    cs-k8s-worker-3:
      private_ip: 1.2.3.9`

				_, err = file.Write([]byte(inputInventory))
				Expect(err).ToNot(HaveOccurred())

				err = manager.FetchFromAnsibleInventory(inventoryFilePath)
				Expect(err).ToNot(HaveOccurred())

				expectedCPHosts := []files.K8sNode{
					{
						IPAddress: "1.2.3.4",
					}, {
						IPAddress: "1.2.3.5",
					}, {
						IPAddress: "1.2.3.6",
					},
				}

				expectedWorkerHosts := []files.K8sNode{
					{
						IPAddress: "1.2.3.7",
					}, {
						IPAddress: "1.2.3.8",
					}, {
						IPAddress: "1.2.3.9",
					},
				}

				actualK8sCPHosts := manager.GetInstallConfig().Kubernetes.ControlPlanes
				Expect(actualK8sCPHosts).To(Equal(expectedCPHosts))

				actualK8sWorkerHosts := manager.GetInstallConfig().Kubernetes.Workers
				Expect(actualK8sWorkerHosts).To(Equal(expectedWorkerHosts))
			})

			It("returns an error if control plane nodes are already set", func() {
				file, err := os.Create(inventoryFilePath)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = os.Remove(inventoryFilePath) }()

				inputInventory := `k8s-cp:
  hosts:
    cs-k8s-cp-1:
      private_ip: 1.2.3.4
    cs-k8s-cp-2:
      private_ip: 1.2.3.5
    cs-k8s-cp-3:
      private_ip: 1.2.3.6
k8s-workers:
  hosts:
    cs-k8s-worker-1:
      private_ip: 1.2.3.7
    cs-k8s-worker-2:
      private_ip: 1.2.3.8
    cs-k8s-worker-3:
      private_ip: 1.2.3.9`

				_, err = file.Write([]byte(inputInventory))
				Expect(err).ToNot(HaveOccurred())

				manager.GetInstallConfig().Kubernetes.ControlPlanes = []files.K8sNode{
					{
						IPAddress: "1.1.1.1",
					},
				}

				err = manager.FetchFromAnsibleInventory(inventoryFilePath)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("k8s control plane nodes are already set. Adjust flags or inventory"))
			})

			It("returns an error if worker nodes are already set", func() {
				file, err := os.Create(inventoryFilePath)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = os.Remove(inventoryFilePath) }()

				inputInventory := `k8s-cp:
  hosts:
    cs-k8s-cp-1:
      private_ip: 1.2.3.4
    cs-k8s-cp-2:
      private_ip: 1.2.3.5
    cs-k8s-cp-3:
      private_ip: 1.2.3.6
k8s-workers:
  hosts:
    cs-k8s-worker-1:
      private_ip: 1.2.3.7
    cs-k8s-worker-2:
      private_ip: 1.2.3.8
    cs-k8s-worker-3:
      private_ip: 1.2.3.9`

				_, err = file.Write([]byte(inputInventory))
				Expect(err).ToNot(HaveOccurred())

				manager.GetInstallConfig().Kubernetes.Workers = []files.K8sNode{
					{
						IPAddress: "1.1.1.1",
					},
				}

				err = manager.FetchFromAnsibleInventory(inventoryFilePath)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("k8s worker nodes are already set. Adjust flags or inventory"))
			})
		})

		Context("inventory has invalid k8s config", func() {
			It("returns an error for missing host variables", func() {
				file, err := os.Create(inventoryFilePath)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = os.Remove(inventoryFilePath) }()

				inputInventoryYaml := `k8s-cp:
	hosts:
	  cs-k8s-cp-1:`
				inputInventory := strings.ReplaceAll(inputInventoryYaml, "\t", "  ")

				_, err = file.Write([]byte(inputInventory))
				Expect(err).ToNot(HaveOccurred())

				err = manager.FetchFromAnsibleInventory(inventoryFilePath)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to fetch k8s control plane hosts from inventory"))
				Expect(err.Error()).To(ContainSubstring("missing private_ip for k8s host 'cs-k8s-cp-1'"))
			})

			It("returns an error for typo in private_ip", func() {
				file, err := os.Create(inventoryFilePath)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = os.Remove(inventoryFilePath) }()

				inputInventoryYaml := `k8s-cp:
	hosts:
	  cs-k8s-cp-1:
	    private_up: 1`
				inputInventory := strings.ReplaceAll(inputInventoryYaml, "\t", "  ")

				_, err = file.Write([]byte(inputInventory))
				Expect(err).ToNot(HaveOccurred())

				err = manager.FetchFromAnsibleInventory(inventoryFilePath)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to fetch k8s control plane hosts from inventory"))
				Expect(err.Error()).To(ContainSubstring("missing private_ip for k8s host 'cs-k8s-cp-1'"))
			})
		})
	})
})
