package installer_test

import (
	"os"
	"path/filepath"

	"github.com/codesphere-cloud/oms/internal/installer"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	// . "github.com/codesphere-cloud/oms/internal/util/testing"
)

var _ = Describe("ConfigManagerAnsible", func() {
	var (
		manager installer.InstallConfigManager

		tempDir           string
		inventoryFilePath string
	)

	BeforeEach(func() {
		tempDir = GinkgoT().TempDir()
		inventoryFilePath = filepath.Join(tempDir, "inventory.yaml")
	})

	Describe("FetchFromAnsibleInventory", func() {
		Context("inventory is empty", func() {
			It("adds empty host lists to the config", func() {
				file, err := os.Create(inventoryFilePath)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = os.Remove(inventoryFilePath) }()

				_, err = file.Write([]byte(""))
				Expect(err).ToNot(HaveOccurred())

				err = manager.FetchFromAnsibleInventory(inventoryFilePath)
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})
})
