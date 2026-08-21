package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSojuListenSkipsAdminSocket(t *testing.T) {
	if server, _, ok := parseSojuListen("unix+admin://"); ok || server != "" {
		t.Fatalf("admin socket should not be a chat endpoint: server=%q ok=%v", server, ok)
	}
	server, tlsEnabled, ok := parseSojuListen("ircs://172.32.0.1:6697")
	if !ok || !tlsEnabled || server != "172.32.0.1:6697" {
		t.Fatalf("unexpected listener parse: server=%q tls=%v ok=%v", server, tlsEnabled, ok)
	}
}

func TestDiscoverSojuConfigFindsChatListener(t *testing.T) {
	path := filepath.Join(t.TempDir(), "soju.conf")
	contents := "listen unix+admin://\nlisten ircs://172.32.0.1:6697\nhostname soju.kode.im\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	discovered, err := discoverSojuConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if discovered.Server != "172.32.0.1:6697" || !discovered.TLS || discovered.TLSServerName != "soju.kode.im" {
		t.Fatalf("unexpected discovery result: %#v", discovered)
	}
}

func TestProfileRoundTripDoesNotRequirePassword(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	tlsEnabled := true
	want := SavedProfile{Server: "soju.kode.im:6697", TLS: &tlsEnabled, Username: "ak"}
	if err := saveProfile(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := loadSavedProfile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Server != want.Server || got.Username != want.Username || got.TLS == nil || !*got.TLS {
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
