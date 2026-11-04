// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package argocd

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	argov1alpha1 "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestWaitForApplicationHealthyUsesConfiguredTimeout(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := argov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	var firstLog string

	start := time.Now()

	err := WaitForApplicationHealthy(context.Background(), fake.NewClientBuilder().WithScheme(scheme).Build(), "missing", "1.2.3", 20*time.Millisecond, func(format string, args ...interface{}) {
		if firstLog == "" {
			firstLog = fmt.Sprintf(format, args...)
		}
	})
	if err == nil || !strings.Contains(err.Error(), "timed out waiting") {
		t.Fatalf("expected readiness timeout, got %v", err)
	}

	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("configured timeout was ignored: elapsed %s", elapsed)
	}

	if !strings.Contains(firstLog, "timeout 20ms") {
		t.Fatalf("expected timeout in readiness log, got %q", firstLog)
	}
}
