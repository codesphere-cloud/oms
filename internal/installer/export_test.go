// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

func (ci *CodesphereInstaller) ArgoCDHandoverArgs() []string {
	return ci.argoCDHandoverArgs()
}

var OrphanCodesphereExternalSecretsWithClient = orphanCodesphereExternalSecrets
