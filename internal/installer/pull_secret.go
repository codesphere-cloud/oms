// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// DockerConfigJSON encodes registry credentials as the .dockerconfigjson of an image pull secret.
// The credentials are keyed by the registry host, with its port.
func DockerConfigJSON(host, username, password string) ([]byte, error) {
	config, err := json.Marshal(map[string]any{
		"auths": map[string]any{
			host: map[string]string{
				"username": username,
				"password": password,
				"auth":     base64.StdEncoding.EncodeToString([]byte(username + ":" + password)),
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("encoding docker config for %q: %w", host, err)
	}

	return config, nil
}
