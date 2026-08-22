// Package version exposes the git commit a binary was built from.
package version

import "runtime/debug"

// Commit is set at build time via
// -ldflags "-X github.com/lk16/flippy/internal/version.Commit=<sha>";
// container builds use this, since images have no .git directory.
var Commit string

// Get returns the build's git commit: the ldflags value if set, else the Go
// toolchain's embedded VCS revision (local builds), else "unknown".
func Get() string {
	if Commit != "" {
		return Commit
	}

	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				return setting.Value
			}
		}
	}

	return "unknown"
}
