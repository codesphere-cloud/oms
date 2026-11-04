// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package env

import (
	"errors"
	"os"
	"path/filepath"
)

//mockery:generate: true
type Env interface {
	GetOmsPortalApiKey() (string, error)
	GetOmsPortalApi() string
	GetOmsWorkdir() string
	GetOmsCacheDir() (string, error)
}

type Environment struct {
}

// legacyAPIKeyLength is the length of API keys in the old format, which are no longer supported.
const legacyAPIKeyLength = 25

// ErrLegacyAPIKey is returned when the configured API key uses the old, unsupported format.
var ErrLegacyAPIKey = errors.New("OMS_PORTAL_API_KEY uses the old API key format, which is no longer supported. Please request a new API key")

// IsLegacyAPIKey reports whether the given API key uses the old, unsupported format.
func IsLegacyAPIKey(apiKey string) bool {
	return len(apiKey) == legacyAPIKeyLength
}

func NewEnv() *Environment {
	return &Environment{}
}

func (e *Environment) GetOmsPortalApiKey() (string, error) {
	apiToken := os.Getenv("OMS_PORTAL_API_KEY")
	if apiToken == "" {
		return "", errors.New("OMS_PORTAL_API_KEY env var required, but not set")
	}

	if IsLegacyAPIKey(apiToken) {
		return "", ErrLegacyAPIKey
	}
	return apiToken, nil
}

func (e *Environment) GetOmsWorkdir() string {
	workdir := os.Getenv("OMS_WORKDIR")
	if workdir == "" {
		return "./oms-workdir"
	}
	return workdir
}

func (e *Environment) GetOmsCacheDir() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, "oms"), nil
}

func (e *Environment) GetOmsPortalApi() string {
	apiUrl := os.Getenv("OMS_PORTAL_API")
	if apiUrl == "" {
		return "https://oms-portal.codesphere.com/api"
	}
	return apiUrl
}
