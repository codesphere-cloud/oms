// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package configtemplating

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"text/template"
)

// SecretStore is the template-facing abstraction for secret backends.
type SecretStore interface {
	LookupSecret(name string, selector ...string) (string, error)
}

// RenderInstallConfigTemplate renders the given config template data, resolving
// any `secret` template calls against store, and returns the rendered bytes.
// Referencing a missing template key or a missing secret is treated as an error.
func RenderInstallConfigTemplate(data []byte, store SecretStore) ([]byte, error) {
	tmpl, err := template.New("install-config").
		Option("missingkey=error").
		Funcs(template.FuncMap{
			"secret": func(name string, selector ...string) (string, error) {
				if store == nil {
					return "", fmt.Errorf("secret store is required to render config template")
				}
				return store.LookupSecret(name, selector...)
			},
			"vaultRef": func(name string) (string, error) {
				return vaultRef(name, store)
			},
		}).
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

// vaultEntryName matches the names a reference can name: the umbrella chart reads them as
// External Secrets template keys.
var vaultEntryName = regexp.MustCompile(`^\w+$`)

// vaultRef renders a reference to a vault entry instead of its value. The Codesphere
// installer resolves it, or the umbrella chart lets External Secrets fill it, so the value
// stays out of the rendered config and the values derived from it.
func vaultRef(name string, store SecretStore) (string, error) {
	if !vaultEntryName.MatchString(name) {
		return "", fmt.Errorf("vault entry name %q must only contain letters, digits and underscores", name)
	}

	if store == nil {
		return "", fmt.Errorf("secret store is required to render config template")
	}

	if _, err := store.LookupSecret(name); err != nil {
		return "", fmt.Errorf("failed to look up vault reference: %w", err)
	}

	return "vaultRef:" + name, nil
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
