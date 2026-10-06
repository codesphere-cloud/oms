// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

func TestLocalComponentsUseSharedStepSelection(t *testing.T) {
	dir := t.TempDir()
	bundle := filepath.Join(dir, "bundle")
	if err := os.Mkdir(bundle, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "node"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" >> \"$OMS_COMPONENT_TEST_LOG\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "components.log")
	t.Setenv("OMS_COMPONENT_TEST_LOG", logPath)
	ci := &CodesphereInstaller{
		ConfigPath: filepath.Join(dir, "config.yaml"), PrivKey: filepath.Join(dir, "key"),
		LocalConfigDir: filepath.Join(dir, "config"), LocalComponents: true,
		AllowedSteps: []string{"set-up-cluster", "codesphere", "ms-backends"},
	}
	config := files.RootConfig{Operations: &files.OperationsConfig{Skip: []string{"codesphere"}}}
	if err := ci.runLocalComponents(NewPackage(dir, bundle), config); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, "setUpCluster") || !strings.Contains(got, "msBackends") || strings.Contains(got, "codesphere\n") {
		t.Fatalf("unexpected local component selection: %s", got)
	}
}
