// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package env

import (
	"errors"
	"os"
)

//mockery:generate: true
type Env interface {
	GetOmsPortalApiKey() (string, error)
	GetOmsPortalApi() string
	GetOmsWorkdir() string
	GetOmsRegistry() string
}

type Environment struct {
}

func NewEnv() *Environment {
	return &Environment{}
}

func (e *Environment) GetOmsPortalApiKey() (string, error) {
	apiToken := os.Getenv("OMS_PORTAL_API_KEY")
	if apiToken == "" {
		return "", errors.New("OMS_PORTAL_API_KEY env var required, but not set")
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

func (e *Environment) GetOmsPortalApi() string {
	apiUrl := os.Getenv("OMS_PORTAL_API")
	if apiUrl == "" {
		return "https://oms-portal.codesphere.com/api"
	}
	return apiUrl
}

// GetOmsRegistry returns the base URL for the OCI registry proxy.
// It derives the registry URL from OMS_PORTAL_API by stripping the /api suffix.
func (e *Environment) GetOmsRegistry() string {
	apiUrl := e.GetOmsPortalApi()
	// Strip trailing /api to get the registry base URL
	if len(apiUrl) > 4 && apiUrl[len(apiUrl)-4:] == "/api" {
		return apiUrl[:len(apiUrl)-4]
	}
	return apiUrl
}
