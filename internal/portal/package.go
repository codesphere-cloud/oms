// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package portal

import (
	"fmt"
	"time"
)

type Builds struct {
	Builds []Build `json:"builds"`
}

type Build struct {
	Version   string     `json:"version"`
	Date      time.Time  `json:"date"`
	Hash      string     `json:"hash"`
	Artifacts []Artifact `json:"artifacts"`
	Internal  bool       `json:"internal"`
}

type Artifact struct {
	Md5Sum   string `json:"md5sum"`
	Filename string `json:"filename"`
	Name     string `json:"name"`
}

func (b *Build) GetBuildForDownload(filename string) (Build, error) {
	for _, a := range b.Artifacts {
		if a.Filename != filename {
			continue
		}

		// Generate identical build but with only the matching artifact
		build := *b
		build.Artifacts = []Artifact{
			a,
		}
		return build, nil
	}

	return Build{}, fmt.Errorf("artifact not found: %s", filename)
}
