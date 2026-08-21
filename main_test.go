package main

import "testing"

func TestVersionStringIncludesBuildRevisionWhenAvailable(t *testing.T) {
	originalVersion, originalRevision := version, revision
	t.Cleanup(func() {
		version, revision = originalVersion, originalRevision
	})

	version, revision = "0.2.2-dev", "abc123"
	if got := versionString(); got != "0.2.2-dev (abc123)" {
		t.Fatalf("versionString() = %q", got)
	}

	revision = "unknown"
	if got := versionString(); got != "0.2.2-dev" {
		t.Fatalf("versionString() without revision = %q", got)
	}
}
