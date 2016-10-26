package networktests

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestNeworkFunctions(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Network")
}
