// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

func brandingValues(branding *files.BrandingConfig) any {
	if branding == nil {
		return nil
	}

	return omitEmpty(map[string]any{
		"desc":        branding.Desc,
		"docsUrl":     branding.DocsURL,
		"faviconHref": branding.FaviconHref,
		"title":       branding.Title,
		"tosUrl":      branding.TosURL,
	})
}
