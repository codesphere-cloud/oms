// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package steps

import (
	"slices"

	"github.com/codesphere-cloud/oms/internal/codesphere/testplan"
)

// The playlists that can be run. DefaultPlaylist is used when the test command
// is called without an explicit selection.
const (
	DefaultPlaylist   = "default"
	ReadinessPlaylist = "readiness"
)

// playlists are the named, ordered selections of the steps in this folder.
// Unlike the steps themselves, playlists are listed here explicitly, because
// their order is what makes them a playlist.
var playlists = []testplan.Playlist{
	{
		Name:        DefaultPlaylist,
		Description: "Verify the installation is up and can run a workspace",
		Tests:       []string{StatusName, SmoketestName},
	},
	{
		Name:        ReadinessPlaylist,
		Description: "Only check that the installation is reachable and ready",
		Tests:       []string{StatusName},
	},
}

// Playlists returns all available playlists.
func Playlists() []testplan.Playlist {
	return slices.Clone(playlists)
}

// PlaylistNames returns the names of all available playlists.
func PlaylistNames() []string {
	names := make([]string, 0, len(playlists))
	for _, p := range playlists {
		names = append(names, p.Name)
	}

	return names
}
