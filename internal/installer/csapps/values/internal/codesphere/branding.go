// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// Branding are the texts and links that brand the IDE.
type Branding struct {
	Desc        string `yaml:"desc,omitempty"`
	DocsURL     string `yaml:"docsUrl,omitempty"`
	FaviconHref string `yaml:"faviconHref,omitempty"`
	Title       string `yaml:"title,omitempty"`
	TosURL      string `yaml:"tosUrl,omitempty"`
}

func brandingValues(branding *files.BrandingConfig) *Branding {
	if branding == nil {
		return nil
	}

	return &Branding{
		Desc:        branding.Desc,
		DocsURL:     branding.DocsURL,
		FaviconHref: branding.FaviconHref,
		Title:       branding.Title,
		TosURL:      branding.TosURL,
	}
}
