// Package testutil contains ginkgo test helpers.
package testutil

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Must is a test helper that fails when error has occurred.
func Must[V any](v V, err error) V {
	GinkgoHelper()
	Expect(err).NotTo(HaveOccurred())
	Expect(v).NotTo(BeNil())

	return v
}

// Must2 is Must for functions returning two values and an error.
func Must2[V, W any](v V, w W, err error) (V, W) {
	GinkgoHelper()
	Expect(err).NotTo(HaveOccurred())

	return v, w
}
