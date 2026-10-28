// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	"errors"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/util"
)

var _ = Describe("K0sctlConfigGenerator", func() {
	const (
		inventoryPath = "inventory.yaml"
		outputPath    = "/out/k0sctl-config.yaml"
		inventory     = `k8s-cp:
  hosts:
    cs-k8s-cp-1:
      internal_ip: 10.0.0.1
k8s-workers:
  hosts:
    cs-k8s-worker-1:
      internal_ip: 10.0.1.1
    cs-k8s-worker-2:
      internal_ip: 10.0.1.2
`
	)

	var (
		mockFileWriter *util.MockFileIO
		mockPM         *installer.MockPackageManager
		mockK0s        *installer.MockK0sManager
		generator      installer.K0sctlConfigGenerator
		written        installer.K0sctlConfig
	)

	BeforeEach(func() {
		mockFileWriter = util.NewMockFileIO(GinkgoT())
		mockPM = installer.NewMockPackageManager(GinkgoT())
		mockK0s = installer.NewMockK0sManager(GinkgoT())
		written = installer.K0sctlConfig{}

		generator = installer.K0sctlConfigGenerator{
			Opts: installer.K0sctlConfigOpts{
				Version:        "v1.30.0+k0s.0",
				NoDownload:     true,
				DatacenterName: "dc1",
			},
			FileWriter: mockFileWriter,
		}
	})

	expectInventoryRead := func(content string) {
		generator.Opts.AnsibleInventory = inventoryPath
		mockFileWriter.EXPECT().ReadFile(inventoryPath).Return([]byte(content), nil)
	}

	expectConfigWritten := func() {
		mockFileWriter.EXPECT().WriteFile(outputPath, mock.Anything, os.FileMode(0644)).
			Run(func(_ string, data []byte, _ os.FileMode) {
				Expect(written.Unmarshal(data)).To(Succeed())
			}).
			Return(nil)
	}

	// hostAddresses returns the SSH addresses of the written hosts with the given role.
	hostAddresses := func(role string) []string {
		addresses := []string{}

		for _, host := range written.Spec.Hosts {
			if host.Role == role {
				addresses = append(addresses, host.SSH.Address)
			}
		}

		return addresses
	}

	It("generates the config from the Ansible inventory", func() {
		expectInventoryRead(inventory)
		expectConfigWritten()

		Expect(generator.Generate(mockPM, mockK0s, outputPath)).To(Equal(outputPath))
		Expect(hostAddresses("controller")).To(Equal([]string{"10.0.0.1"}))
		Expect(hostAddresses("worker")).To(Equal([]string{"10.0.1.1", "10.0.1.2"}))
		Expect(written.Metadata.Name).To(Equal("codesphere-dc1"))
	})

	It("replaces the hosts of the Ansible inventory with the ones given in the options", func() {
		expectInventoryRead(inventory)

		generator.Opts.Workers = []string{"10.0.9.1"}
		generator.Opts.APIServerHost = "k8s.example.com"

		expectConfigWritten()

		Expect(generator.Generate(mockPM, mockK0s, outputPath)).To(Equal(outputPath))
		Expect(hostAddresses("controller")).To(Equal([]string{"10.0.0.1"}))
		Expect(hostAddresses("worker")).To(Equal([]string{"10.0.9.1"}))
		Expect(written.Spec.K0s.Config.Spec.API.ExternalAddress).To(Equal("k8s.example.com"))
	})

	It("runs both roles on a node given as control plane and worker", func() {
		generator.Opts.ControlPlanes = []string{"10.0.0.1"}
		generator.Opts.Workers = []string{"10.0.0.1", "10.0.0.2"}

		expectConfigWritten()

		Expect(generator.Generate(mockPM, mockK0s, outputPath)).To(Equal(outputPath))
		Expect(hostAddresses("controller")).To(Equal([]string{"10.0.0.1"}))
		Expect(hostAddresses("worker")).To(Equal([]string{"10.0.0.2"}))
		Expect(written.Spec.Hosts[0].InstallFlags).To(Equal([]string{"--enable-worker", "--no-taints=true"}))
	})

	It("recognizes a node given in different IPv6 notations as the same node", func() {
		generator.Opts.ControlPlanes = []string{"fd00::1"}
		generator.Opts.Workers = []string{"fd00:0:0::1"}

		expectConfigWritten()

		Expect(generator.Generate(mockPM, mockK0s, outputPath)).To(Equal(outputPath))
		Expect(written.Spec.Hosts).To(HaveLen(1))
		Expect(written.Spec.Hosts[0].SSH.Address).To(Equal("fd00::1"))
		Expect(written.Spec.Hosts[0].InstallFlags).To(ContainElement("--enable-worker"))
	})

	It("fails without a control plane", func() {
		expectInventoryRead(`k8s-workers:
  hosts:
    cs-k8s-worker-1:
      internal_ip: 10.0.1.1
`)

		_, err := generator.Generate(mockPM, mockK0s, outputPath)
		Expect(err).To(MatchError("no Kubernetes control plane defined"))
	})

	It("fails without a data center name", func() {
		generator.Opts.ControlPlanes = []string{"10.0.0.1"}
		generator.Opts.DatacenterName = ""

		_, err := generator.Generate(mockPM, mockK0s, outputPath)
		Expect(err).To(MatchError("no data center name defined"))
	})

	It("fails for an invalid IP address in the options", func() {
		generator.Opts.ControlPlanes = []string{"10.0.0.1"}
		generator.Opts.Workers = []string{"10.0.0.300"}

		_, err := generator.Generate(mockPM, mockK0s, outputPath)
		Expect(err).To(MatchError(ContainSubstring(`invalid worker: invalid IP address "10.0.0.300"`)))
	})

	It("fails for an invalid IP address in the Ansible inventory", func() {
		expectInventoryRead(`k8s-cp:
  hosts:
    cs-k8s-cp-1:
      internal_ip: cs-k8s-cp-1.local
`)

		_, err := generator.Generate(mockPM, mockK0s, outputPath)
		Expect(err).To(MatchError(ContainSubstring(`invalid k8s-cp host: invalid IP address "cs-k8s-cp-1.local"`)))
	})

	It("fails for an Ansible inventory without Kubernetes hosts", func() {
		expectInventoryRead(`ceph:
  hosts:
    cs-ceph-1:
      internal_ip: 10.0.2.1
`)

		_, err := generator.Generate(mockPM, mockK0s, outputPath)
		Expect(err).To(MatchError(ContainSubstring(`no k8s-cp or k8s-workers hosts defined in Ansible inventory "inventory.yaml"`)))
	})

	It("fails when the Ansible inventory cannot be read", func() {
		generator.Opts.AnsibleInventory = inventoryPath
		mockFileWriter.EXPECT().ReadFile(inventoryPath).Return(nil, errors.New("permission denied"))

		_, err := generator.Generate(mockPM, mockK0s, outputPath)
		Expect(err).To(MatchError(ContainSubstring(`failed to load Ansible inventory "inventory.yaml": failed to read file: permission denied`)))
	})
})
