package buildinfo

import (
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestCurrentUsesEmbeddedSemanticVersion(t *testing.T) {
	originalVersion, originalCommit, originalBuildTime := Version, Commit, BuildTime
	t.Cleanup(func() {
		Version, Commit, BuildTime = originalVersion, originalCommit, originalBuildTime
	})
	Version = ""
	Commit = ""
	BuildTime = ""

	info := Current()
	if !regexp.MustCompile(`^\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$`).MatchString(info.Version) {
		t.Fatalf("embedded version %q is not semantic", info.Version)
	}
	if info.Commit != "unknown" {
		t.Fatalf("commit = %q, want unknown", info.Commit)
	}
	if _, err := time.Parse(time.RFC3339, info.StartedAt); err != nil {
		t.Fatalf("started_at is invalid: %v", err)
	}
	if !strings.HasPrefix(info.GoVersion, "go") {
		t.Fatalf("unexpected Go version %q", info.GoVersion)
	}
}

func TestBuildOverridesAndSummary(t *testing.T) {
	originalVersion, originalCommit, originalBuildTime := Version, Commit, BuildTime
	t.Cleanup(func() {
		Version, Commit, BuildTime = originalVersion, originalCommit, originalBuildTime
	})
	Version = " 2.4.1 "
	Commit = "abcdef1234567890"
	BuildTime = " 2026-07-20T12:00:00Z "

	info := Current()
	if info.Version != "2.4.1" || info.Commit != "abcdef1234567890" || info.BuildTime != "2026-07-20T12:00:00Z" {
		t.Fatalf("unexpected normalized info: %+v", info)
	}
	if got := info.ShortCommit(); got != "abcdef123456" {
		t.Fatalf("ShortCommit() = %q", got)
	}
	if got := info.Summary(); got != "v2.4.1 (abcdef123456)" {
		t.Fatalf("Summary() = %q", got)
	}
}
