// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package k0s_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"

	"github.com/codesphere-cloud/oms/cli/cmd/k0s"
	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	intutil "github.com/codesphere-cloud/oms/internal/util"
)

var _ = Describe("InitK0sConfigCmd", func() {
	var (
		c              k0s.InitK0sConfigCmd
		mockEnv        *env.MockEnv
		mockFileWriter *intutil.MockFileIO
		mockPM         *installer.MockPackageManager
		mockK0s        *installer.MockK0sManager
		tempDir        string
	)

	// isAirgapConfig matches k0sctl configs that upload the bundle to the workers and stop
	// k0s from pulling images.
	isAirgapConfig := mock.MatchedBy(func(data []byte) bool {
		return strings.Contains(string(data), "dstDir: /var/lib/k0s/images") &&
			strings.Contains(string(data), "default_pull_policy: Never")
	})

	BeforeEach(func() {
		mockEnv = env.NewMockEnv(GinkgoT())
		mockFileWriter = intutil.NewMockFileIO(GinkgoT())
		mockPM = installer.NewMockPackageManager(GinkgoT())
		mockK0s = installer.NewMockK0sManager(GinkgoT())
		tempDir = GinkgoT().TempDir()

		config := newTestInstallConfig(true)
		config.Kubernetes.Workers = []files.K8sNode{{IPAddress: "192.168.1.101"}}

		c = k0s.InitK0sConfigCmd{
			Opts: k0s.InitK0sConfigOpts{
				GlobalOptions: &util.GlobalOptions{},
				K0sctlConfigOpts: installer.K0sctlConfigOpts{
					InstallConfig: writeTestInstallConfig(tempDir, config),
					Version:       "v1.30.0+k0s.0",
				},
			},
			Env:        mockEnv,
			FileWriter: mockFileWriter,
		}
	})

	cacheOnly := installer.DownloadOptions{CacheOnly: true}

	setupWorkdirMocks := func() {
		mockEnv.EXPECT().GetOmsWorkdir().Return(tempDir)
		mockFileWriter.EXPECT().MkdirAll(tempDir, os.FileMode(0755)).Return(nil)
	}

	It("writes the k0sctl config to the OMS workdir by default", func() {
		setupWorkdirMocks()
		mockK0s.EXPECT().Download("v1.30.0+k0s.0", cacheOnly).Return("/cache/k0s", nil)
		mockFileWriter.EXPECT().WriteFile(filepath.Join(tempDir, "k0sctl-config-test-dc.yaml"), mock.Anything, os.FileMode(0644)).Return(nil)

		Expect(c.InitK0sConfig(mockPM, mockK0s)).To(Succeed())
	})

	It("writes an airgap k0sctl config to --output with the bundle from the OMS cache", func() {
		c.Opts.Airgap = true
		c.Opts.Output = "/etc/codesphere/k0sctl-config.yaml"

		setupWorkdirMocks()
		mockK0s.EXPECT().Download("v1.30.0+k0s.0", cacheOnly).Return("/cache/k0s", nil)
		mockK0s.EXPECT().DownloadAirgapBundle("v1.30.0+k0s.0", "", cacheOnly).Return("/cache/bundle.tar", nil)
		mockFileWriter.EXPECT().WriteFile("/etc/codesphere/k0sctl-config.yaml", isAirgapConfig, os.FileMode(0644)).Return(nil)

		Expect(c.InitK0sConfig(mockPM, mockK0s)).To(Succeed())
	})

	It("uses the airgap bundle given with --path", func() {
		c.Opts.Airgap = true
		c.Opts.AirgapBundlePath = "/opt/k0s/bundle.tar"

		setupWorkdirMocks()
		mockK0s.EXPECT().Download("v1.30.0+k0s.0", cacheOnly).Return("/cache/k0s", nil)
		mockFileWriter.EXPECT().Exists("/opt/k0s/bundle.tar").Return(true)
		mockFileWriter.EXPECT().WriteFile(mock.Anything, mock.MatchedBy(func(data []byte) bool {
			return strings.Contains(string(data), "src: /opt/k0s/bundle.tar")
		}), os.FileMode(0644)).Return(nil)

		Expect(c.InitK0sConfig(mockPM, mockK0s)).To(Succeed())
	})

	It("writes relative --path and --ssh-key-path as absolute paths", func() {
		c.Opts.Airgap = true
		c.Opts.AirgapBundlePath = "bundle.tar"
		c.Opts.SSHKeyPath = "keys/id_rsa"
		cwd, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())

		bundlePath := filepath.Join(cwd, "bundle.tar")

		setupWorkdirMocks()
		mockK0s.EXPECT().Download("v1.30.0+k0s.0", cacheOnly).Return("/cache/k0s", nil)
		mockFileWriter.EXPECT().Exists(bundlePath).Return(true)
		mockFileWriter.EXPECT().WriteFile(mock.Anything, mock.MatchedBy(func(data []byte) bool {
			return strings.Contains(string(data), "src: "+bundlePath) &&
				strings.Contains(string(data), "keyPath: "+filepath.Join(cwd, "keys/id_rsa"))
		}), os.FileMode(0644)).Return(nil)

		Expect(c.InitK0sConfig(mockPM, mockK0s)).To(Succeed())
	})

	It("keeps an SSH key path starting with ~ for k0sctl to expand", func() {
		c.Opts.SSHKeyPath = "~/.ssh/id_rsa"

		setupWorkdirMocks()
		mockK0s.EXPECT().Download("v1.30.0+k0s.0", cacheOnly).Return("/cache/k0s", nil)
		mockFileWriter.EXPECT().WriteFile(mock.Anything, mock.MatchedBy(func(data []byte) bool {
			return strings.Contains(string(data), "keyPath: ~/.ssh/id_rsa")
		}), os.FileMode(0644)).Return(nil)

		Expect(c.InitK0sConfig(mockPM, mockK0s)).To(Succeed())
	})

	It("takes the k0s binary from the package but still uses the airgap bundle", func() {
		c.Opts.Airgap = true
		c.Opts.Package = "test-package.tar.gz"

		setupWorkdirMocks()
		mockPM.EXPECT().ExtractDependency("kubernetes/files/k0s", false, false).Return(nil)
		mockPM.EXPECT().GetDependencyPath("kubernetes/files/k0s").Return("/test/path/k0s")
		mockK0s.EXPECT().DownloadAirgapBundle("v1.30.0+k0s.0", "", cacheOnly).Return("/cache/bundle.tar", nil)
		mockFileWriter.EXPECT().WriteFile(mock.Anything, isAirgapConfig, os.FileMode(0644)).Return(nil)

		Expect(c.InitK0sConfig(mockPM, mockK0s)).To(Succeed())
	})

	It("fails when the airgap bundle given with --path does not exist", func() {
		c.Opts.Airgap = true
		c.Opts.AirgapBundlePath = "/nonexistent/bundle.tar"

		setupWorkdirMocks()
		mockK0s.EXPECT().Download("v1.30.0+k0s.0", cacheOnly).Return("/cache/k0s", nil)
		mockFileWriter.EXPECT().Exists("/nonexistent/bundle.tar").Return(false)

		Expect(c.InitK0sConfig(mockPM, mockK0s)).To(MatchError(ContainSubstring("does not exist")))
	})

	It("does not download a missing k0s binary", func() {
		setupWorkdirMocks()
		mockK0s.EXPECT().Download("v1.30.0+k0s.0", cacheOnly).Return("", errors.New("k0s v1.30.0+k0s.0 not found in OMS cache"))

		Expect(c.InitK0sConfig(mockPM, mockK0s)).To(MatchError(ContainSubstring("not found in OMS cache")))
	})

	It("does not download a missing airgap bundle", func() {
		c.Opts.Airgap = true

		setupWorkdirMocks()
		mockK0s.EXPECT().Download("v1.30.0+k0s.0", cacheOnly).Return("/cache/k0s", nil)
		mockK0s.EXPECT().DownloadAirgapBundle("v1.30.0+k0s.0", "", cacheOnly).Return("", errors.New("k0s airgap bundle v1.30.0+k0s.0 not found"))

		Expect(c.InitK0sConfig(mockPM, mockK0s)).To(MatchError(ContainSubstring("airgap bundle v1.30.0+k0s.0 not found")))
	})

	It("fails when --path is given without --airgapped", func() {
		c.Opts.AirgapBundlePath = "/opt/k0s/bundle.tar"

		Expect(c.InitK0sConfig(mockPM, mockK0s)).To(MatchError("--path requires --airgapped"))
	})

	It("fails when --no-download is combined with --airgapped", func() {
		c.Opts.Airgap = true
		c.Opts.NoDownload = true

		setupWorkdirMocks()

		Expect(c.InitK0sConfig(mockPM, mockK0s)).To(MatchError(ContainSubstring("--no-download cannot be combined with --airgapped")))
	})

	It("fails without install-config, Ansible inventory and control planes", func() {
		c.Opts.InstallConfig = ""

		Expect(c.InitK0sConfig(mockPM, mockK0s)).To(MatchError("one of --install-config, --ansible-inventory or --control-plane is required"))
	})

	It("fails without a data center name when there is no install-config", func() {
		c.Opts.InstallConfig = ""
		c.Opts.ControlPlanes = []string{"10.0.0.1"}

		Expect(c.InitK0sConfig(mockPM, mockK0s)).To(MatchError("--datacenter-name is required without --install-config"))
	})

	Context("with an Ansible inventory and host flags", func() {
		const (
			inventoryPath = "inventory.yaml"
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

		var written installer.K0sctlConfig

		BeforeEach(func() {
			written = installer.K0sctlConfig{}
		})

		expectInventoryRead := func(content string) {
			c.Opts.AnsibleInventory = inventoryPath
			mockFileWriter.EXPECT().ReadFile(inventoryPath).Return([]byte(content), nil)
		}

		expectConfigWritten := func(path string) {
			mockFileWriter.EXPECT().WriteFile(path, mock.Anything, os.FileMode(0644)).
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

		It("replaces the hosts of the install-config with the ones of the Ansible inventory", func() {
			expectInventoryRead(inventory)

			setupWorkdirMocks()
			mockK0s.EXPECT().Download("v1.30.0+k0s.0", cacheOnly).Return("/cache/k0s", nil)
			expectConfigWritten(filepath.Join(tempDir, "k0sctl-config-test-dc.yaml"))

			Expect(c.InitK0sConfig(mockPM, mockK0s)).To(Succeed())
			Expect(hostAddresses("controller")).To(Equal([]string{"10.0.0.1"}))
			Expect(hostAddresses("worker")).To(Equal([]string{"10.0.1.1", "10.0.1.2"}))
			Expect(written.Spec.K0s.Config.Spec.API.ExternalAddress).To(Equal("api.test.example.com"))
		})

		It("keeps the control planes of the install-config when the Ansible inventory only defines workers", func() {
			expectInventoryRead(`k8s-workers:
  hosts:
    cs-k8s-worker-1:
      internal_ip: 10.0.1.1
`)

			setupWorkdirMocks()
			mockK0s.EXPECT().Download("v1.30.0+k0s.0", cacheOnly).Return("/cache/k0s", nil)
			expectConfigWritten(filepath.Join(tempDir, "k0sctl-config-test-dc.yaml"))

			Expect(c.InitK0sConfig(mockPM, mockK0s)).To(Succeed())
			Expect(hostAddresses("controller")).To(Equal([]string{"192.168.1.100"}))
			Expect(hostAddresses("worker")).To(Equal([]string{"10.0.1.1"}))
		})

		It("replaces the install-config and the Ansible inventory with the values given as flags", func() {
			expectInventoryRead(inventory)

			c.Opts.Workers = []string{"10.0.9.1"}
			c.Opts.DatacenterName = "dc2"
			c.Opts.APIServerHost = "k8s.example.com"

			setupWorkdirMocks()
			mockK0s.EXPECT().Download("v1.30.0+k0s.0", cacheOnly).Return("/cache/k0s", nil)
			expectConfigWritten(filepath.Join(tempDir, "k0sctl-config-dc2.yaml"))

			Expect(c.InitK0sConfig(mockPM, mockK0s)).To(Succeed())
			Expect(hostAddresses("controller")).To(Equal([]string{"10.0.0.1"}))
			Expect(hostAddresses("worker")).To(Equal([]string{"10.0.9.1"}))
			Expect(written.Metadata.Name).To(Equal("codesphere-dc2"))
			Expect(written.Spec.K0s.Config.Spec.API.ExternalAddress).To(Equal("k8s.example.com"))
		})

		It("generates the config from host flags without an install-config", func() {
			c.Opts.InstallConfig = ""
			c.Opts.ControlPlanes = []string{"10.0.0.1"}
			c.Opts.Workers = []string{"10.0.0.2"}
			c.Opts.DatacenterName = "dc1"

			setupWorkdirMocks()
			mockK0s.EXPECT().Download("v1.30.0+k0s.0", cacheOnly).Return("/cache/k0s", nil)
			expectConfigWritten(filepath.Join(tempDir, "k0sctl-config-dc1.yaml"))

			Expect(c.InitK0sConfig(mockPM, mockK0s)).To(Succeed())
			Expect(hostAddresses("controller")).To(Equal([]string{"10.0.0.1"}))
			Expect(hostAddresses("worker")).To(Equal([]string{"10.0.0.2"}))
		})
	})
})
