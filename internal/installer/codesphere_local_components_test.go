// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"errors"
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

func newLocalComponentsInstaller(t *testing.T) (*CodesphereInstaller, PackageManager, string) {
	t.Helper()

	dir := t.TempDir()

	bundle := filepath.Join(dir, "bundle")
	if err := os.Mkdir(bundle, 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(bundle, "node"), []byte("#!/bin/sh\necho installer >> \"$OMS_COMPONENT_TEST_LOG\"\n"), 0755); err != nil {
		t.Fatal(err)
	}

	logPath := filepath.Join(dir, "components.log")
	t.Setenv("OMS_COMPONENT_TEST_LOG", logPath)

	ci := &CodesphereInstaller{
		ConfigPath: filepath.Join(dir, "config.yaml"), PrivKey: filepath.Join(dir, "key"),
		LocalConfigDir: filepath.Join(dir, "config"), LocalComponents: true,
		AllowedSteps: []string{"codesphere"},
	}

	return ci, NewPackage(dir, bundle), logPath
}

func TestBeforeRunRunsBeforeTheInstaller(t *testing.T) {
	ci, pm, logPath := newLocalComponentsInstaller(t)
	ci.BeforeRun = func() error {
		return os.WriteFile(logPath, []byte("before-run\n"), 0600)
	}

	if err := ci.runInstaller(pm, files.RootConfig{}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}

	if got := string(data); got != "before-run\ninstaller\n" {
		t.Fatalf("unexpected order: %q", got)
	}
}

func TestBeforeRunFailureSkipsTheInstaller(t *testing.T) {
	ci, pm, logPath := newLocalComponentsInstaller(t)
	ci.BeforeRun = func() error {
		return errors.New("handover failed")
	}

	if err := ci.runInstaller(pm, files.RootConfig{}); err == nil || err.Error() != "handover failed" {
		t.Fatalf("expected the BeforeRun error, got %v", err)
	}

	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Fatalf("installer ran despite the BeforeRun failure: %v", err)
	}
}
