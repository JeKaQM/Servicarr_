package buildinfo

import (
	_ "embed"
	"fmt"
	"runtime"
	"strings"
	"time"
)

// embeddedVersion is the canonical application version used when a build does
// not provide an override through -ldflags.
//
//go:embed VERSION
var embeddedVersion string

// Version, Commit, and BuildTime can be overridden by release builds.
var (
	Version   string
	Commit    = "unknown"
	BuildTime string
	startedAt = time.Now().UTC()
)

// Info describes the currently running Servicarr build.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time,omitempty"`
	StartedAt string `json:"started_at"`
	GoVersion string `json:"go_version"`
}

// Current returns normalized build details suitable for APIs, logs, and storage.
func Current() Info {
	version := strings.TrimSpace(Version)
	if version == "" {
		version = strings.TrimSpace(embeddedVersion)
	}
	if version == "" {
		version = "dev"
	}

	commit := strings.TrimSpace(Commit)
	if commit == "" {
		commit = "unknown"
	}

	return Info{
		Version:   version,
		Commit:    commit,
		BuildTime: strings.TrimSpace(BuildTime),
		StartedAt: startedAt.Format(time.RFC3339),
		GoVersion: runtime.Version(),
	}
}

// ShortCommit returns a compact commit identifier for display.
func (i Info) ShortCommit() string {
	if len(i.Commit) > 12 {
		return i.Commit[:12]
	}
	return i.Commit
}

// Summary returns a concise human-readable build identifier.
func (i Info) Summary() string {
	if i.Commit == "" || i.Commit == "unknown" || i.Commit == "local" {
		return "v" + i.Version
	}
	return fmt.Sprintf("v%s (%s)", i.Version, i.ShortCommit())
}
