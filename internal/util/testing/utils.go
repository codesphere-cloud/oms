// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package testing

import (
	"github.com/onsi/gomega"
)

func MustMap[T any](value any) map[string]T {
	result, ok := value.(map[string]T)
	gomega.Expect(ok).To(gomega.BeTrue(), "expected map[string]%T, got %T", *new(T), value)
	return result
}

func AssertZeroRequests(value any) {
	requests := MustMap[int](value)
	gomega.Expect(requests).To(gomega.HaveKeyWithValue("cpu", 0))
	gomega.Expect(requests).To(gomega.HaveKeyWithValue("memory", 0))
}
