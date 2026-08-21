package main

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSetupWizardDryRunDiscoversUserAndSocket(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix sockets and POSIX shell are required")
	}
	temporaryDir, err := os.MkdirTemp("/tmp", "soju-tui-setup-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(temporaryDir)

	socketPath := filepath.Join(temporaryDir, "admin.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	configPath := filepath.Join(temporaryDir, "soju.conf")
	config := "listen unix+admin://" + socketPath + "\n"
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}

	stubDir := filepath.Join(temporaryDir, "bin")
	if err := os.Mkdir(stubDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(stubDir, "id"), "#!/bin/sh\nif [ \"${1:-}\" = -u ]; then echo 0; fi\nexit 0\n")
	for _, name := range []string{"setfacl", "systemctl", "runuser", "sojuctl"} {
		writeExecutable(t, filepath.Join(stubDir, name), "#!/bin/sh\nexit 0\n")
	}

	setupPath, err := filepath.Abs(filepath.Join("scripts", "setup.sh"))
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(setupPath,
		"--user", "testadmin",
		"--config", configPath,
		"--sojuctl", filepath.Join(stubDir, "sojuctl"),
		"--dry-run",
	)
	command.Env = append(os.Environ(), "PATH="+stubDir+":"+os.Getenv("PATH"))
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("setup dry run failed: %v\n%s", err, output)
	}
	text := string(output)
	for _, expected := range []string{"Local administrator: testadmin", "Admin socket:        " + socketPath, "Dry run complete; no system files were changed."} {
		if !strings.Contains(text, expected) {
			t.Fatalf("setup output missing %q:\n%s", expected, text)
		}
	}
}

func writeExecutable(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o700); err != nil {
		t.Fatal(err)
	}
}
