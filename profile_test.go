package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAdminProfileRoundTripIsPrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "admin.json")
	want := AdminProfile{ConfigPath: "/etc/soju/config", SojuCtl: "/usr/bin/sojuctl"}
	if err := saveAdminProfile(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := loadAdminProfile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("profile mismatch: got=%#v want=%#v", got, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("profile mode = %o, want 600", info.Mode().Perm())
	}
}
