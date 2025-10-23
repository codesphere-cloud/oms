// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

//go:build integration
// +build integration

package cmd_test

import "errors"

// interface for tests and allows injecting a specific API key and API URL without modifying process env
type testEnv struct {
	apiKey  string
	apiURL  string
	workdir string
}

func NewTestEnv(apiKey, apiURL, workdir string) *testEnv {
	return &testEnv{apiKey: apiKey, apiURL: apiURL, workdir: workdir}
}

func (e *testEnv) GetOmsPortalApiKey() (string, error) {
	if e.apiKey == "" {
		return "", errors.New("OMS_PORTAL_API_KEY not set in test env")
	}
	return e.apiKey, nil
}

func (e *testEnv) GetOmsPortalApi() string {
	if e.apiURL == "" {
		return "https://oms-portal.codesphere.com/api"
	}
	return e.apiURL
}

func (e *testEnv) GetOmsWorkdir() string {
	if e.workdir == "" {
		return "./oms-workdir"
	}
	return e.workdir
}
