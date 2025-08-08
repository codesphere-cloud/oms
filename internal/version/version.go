package version

// Variables are injected by goreleaser on release
var (
	version string = "0.0.0"
	commit  string = "none"
	date    string = "unknown"
	os      string = "unknown"
	arch    string = "unknown"
)

type Build struct{}

func Version() string {
	return version
}

func Commit() string {
	return commit
}

func BuildDate() string {
	return date
}

func Os() string {
	return os
}

func Arch() string {
	return arch
}
