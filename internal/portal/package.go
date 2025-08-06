package portal

import (
	"time"
)

type CodesphereBuilds struct {
	Builds []CodesphereBuild `json:"builds"`
}

type CodesphereBuild struct {
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
