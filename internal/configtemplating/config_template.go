// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package configtemplating

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"text/template"
)

// SecretStore is the template-facing abstraction for secret backends.
type SecretStore interface {
	LookupSecret(name string, selector ...string) (string, error)
}

const secretRefMarker = "__oms_secret_ref__"

type secretResolver func(name string, selector ...string) (string, error)

// RenderInstallConfigTemplate renders the given config template data, resolving
// any `secret` template calls against store, and returns the rendered bytes.
// Referencing a missing template key or a missing secret is treated as an error.
func RenderInstallConfigTemplate(data []byte, store SecretStore) ([]byte, error) {
	return render(data, func(name string, selector ...string) (string, error) {
		if store == nil {
			return "", fmt.Errorf("secret store is required to render config template")
		}

		return store.LookupSecret(name, selector...)
	})
}

// RenderSecretMarkers renders the given config template data with every `secret` call replaced by
// a marker, so CheckNoCodesphereSecretRefs can find where secrets are used without resolving them.
func RenderSecretMarkers(data []byte) ([]byte, error) {
	return render(data, func(string, ...string) (string, error) {
		return secretRefMarker, nil
	})
}

// CheckNoCodesphereSecretRefs rejects `secret` calls in the codesphere block of a config
// rendered with RenderSecretMarkers. Their values reach the Codesphere chart's ExternalSecrets in
// plaintext. Dev releases, the only ones that set codesphere.releaseName, are exempt for now
// (only use case atm.). Long term config and secrets should be more strictly separated, i.e. no
// secrets in config.yml (requires some changes in auth-service).
func CheckNoCodesphereSecretRefs(config map[string]any) error {
	codesphere, ok := config["codesphere"].(map[string]any)
	if !ok {
		return nil
	}

	if releaseName, _ := codesphere["releaseName"].(string); releaseName != "" {
		return nil
	}

	if strings.Contains(fmt.Sprint(codesphere), secretRefMarker) {
		return fmt.Errorf("secret templates are not supported in the codesphere block")
	}

	return nil
}

func render(data []byte, secret secretResolver) ([]byte, error) {
	tmpl, err := template.New("install-config").
		Option("missingkey=error").
		Funcs(template.FuncMap{"secret": secret}).
		Parse(string(data))
	if err != nil {
		return nil, fmt.Errorf("failed to parse config template: %w", err)
	}

	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, nil); err != nil {
		return nil, fmt.Errorf("failed to render config template: %w", err)
	}

	return rendered.Bytes(), nil
}

// RenderConfigFileToTemp renders configPath with store into a temporary
// 0600 YAML file and returns that path plus a cleanup function. Callers should
// pass the returned path to downstream code and defer cleanup immediately.
func RenderConfigFileToTemp(configPath string, store SecretStore) (string, func(), error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", nil, fmt.Errorf("failed to read config file %s: %w", configPath, err)
	}

	rendered, err := RenderInstallConfigTemplate(data, store)
	if err != nil {
		return "", nil, err
	}

	tmp, err := os.CreateTemp("", "oms-rendered-config-*.yaml")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create temporary rendered config: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		_ = os.Remove(tmpPath)
	}

	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		cleanup()
		return "", nil, fmt.Errorf("failed to restrict temporary rendered config permissions: %w", err)
	}
	if _, err := tmp.Write(rendered); err != nil {
		_ = tmp.Close()
		cleanup()
		return "", nil, fmt.Errorf("failed to write temporary rendered config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("failed to close temporary rendered config: %w", err)
	}

	return tmpPath, cleanup, nil
}
