package e2e

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestBasicFeatures(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Basic")
}
