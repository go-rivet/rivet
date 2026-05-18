package version

import (
	_ "embed"
	"fmt"
)

var (
	Version   = "dev"
	CommitSHA = "unknown"
	BuildTime = "unknown"
)

func GetVersion() string {
	return Version
}

func GetVersionWithBuildInfo() string {
	return fmt.Sprintf("Rivet:  %s\nCommit: %s\nBuilt:  %s", Version, CommitSHA, BuildTime)
}
