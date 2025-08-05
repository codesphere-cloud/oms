package portal_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPortal(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Portal Suite")
}
