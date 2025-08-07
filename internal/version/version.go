package version

// Variables are injected by goreleaser on release
var (
	version string = "dev"
	commit  string = "none"
	date    string = "unknown"
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
