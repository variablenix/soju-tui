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
	for _, name := range []string{"setfacl", "systemctl", "sojuctl", "chown"} {
		writeExecutable(t, filepath.Join(stubDir, name), "#!/bin/sh\nexit 0\n")
	}
	writeExecutable(t, filepath.Join(stubDir, "runuser"), "#!/bin/sh\nwhile [ \"$#\" -gt 0 ] && [ \"$1\" != -- ]; do shift; done\n[ \"$#\" -gt 0 ] || exit 1\nshift\nexec \"$@\"\n")
	writeExecutable(t, filepath.Join(stubDir, "stat"), "#!/bin/sh\ncase \"${2:-}\" in\n'%u') echo 0 ;;\n'%a') echo 755 ;;\n'%u:%g:%a:%h') echo 0:0:755:1 ;;\nesac\n")
	tuiBinary := filepath.Join(temporaryDir, "soju-tui")
	const sourceContents = "#!/bin/sh\nif [ \"${1:-}\" = -version ]; then echo 'test-build (abc123)'; fi\nexit 0\n"
	writeExecutable(t, tuiBinary, sourceContents)
	installPath := filepath.Join(temporaryDir, "installed", "soju-tui")

	repositoryScripts := filepath.Join(temporaryDir, "repository", "scripts")
	if err := os.MkdirAll(repositoryScripts, 0o700); err != nil {
		t.Fatal(err)
	}
	setupSource, err := os.ReadFile(filepath.Join("scripts", "setup.sh"))
	if err != nil {
		t.Fatal(err)
	}
	setupPath := filepath.Join(repositoryScripts, "setup.sh")
	writeExecutable(t, setupPath, string(setupSource))
	writeExecutable(t, filepath.Join(repositoryScripts, "grant-admin-access.sh"), "#!/bin/sh\nfor argument in \"$@\"; do\n  if [ \"$argument\" = --dry-run ]; then echo 'Dry run complete; no system files were changed.'; fi\ndone\nexit 0\n")
	command := exec.Command(setupPath,
		"--user", "testadmin",
		"--config", configPath,
		"--sojuctl", filepath.Join(stubDir, "sojuctl"),
		"--binary", tuiBinary,
		"--install-path", installPath,
		"--dry-run",
	)
	command.Env = append(os.Environ(), "PATH="+stubDir+":"+os.Getenv("PATH"))
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("setup dry run failed: %v\n%s", err, output)
	}
	text := string(output)
	for _, expected := range []string{
		"Local administrator: testadmin",
		"Admin socket:        " + socketPath,
		"Installed command:   " + installPath + " (will install)",
		"Dry run complete; no system files were changed.",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("setup output missing %q:\n%s", expected, text)
		}
	}
	if _, err := os.Stat(installPath); !os.IsNotExist(err) {
		t.Fatalf("dry run created install path %q: %v", installPath, err)
	}
	if err := os.MkdirAll(filepath.Dir(installPath), 0o700); err != nil {
		t.Fatal(err)
	}
	const existingContents = "existing unrelated command\n"
	if err := os.WriteFile(installPath, []byte(existingContents), 0o700); err != nil {
		t.Fatal(err)
	}
	command = exec.Command(setupPath,
		"--user", "testadmin",
		"--config", configPath,
		"--sojuctl", filepath.Join(stubDir, "sojuctl"),
		"--binary", tuiBinary,
		"--install-path", installPath,
	)
	command.Env = append(os.Environ(), "PATH="+stubDir+":"+os.Getenv("PATH"))
	command.Stdin = strings.NewReader("n\n")
	output, err = command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "cancelled before replacing "+installPath) {
		t.Fatalf("setup did not require replacement confirmation: %v\n%s", err, output)
	}
	contents, err := os.ReadFile(installPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != existingContents {
		t.Fatalf("cancelled setup changed existing command: %q", contents)
	}

	command = exec.Command(setupPath,
		"--user", "testadmin",
		"--config", configPath,
		"--sojuctl", filepath.Join(stubDir, "sojuctl"),
		"--binary", tuiBinary,
		"--install-path", installPath,
	)
	command.Env = append(os.Environ(), "PATH="+stubDir+":"+os.Getenv("PATH"))
	command.Stdin = strings.NewReader("y\n")
	output, err = command.CombinedOutput()
	if err != nil {
		t.Fatalf("setup replacement failed: %v\n%s", err, output)
	}
	for _, expected := range []string{
		"TUI build:           test-build (abc123)",
		"Replace the existing regular file at " + installPath + " now?",
		"Installed command: " + installPath,
		"Verified installed fingerprint:",
	} {
		if !strings.Contains(string(output), expected) {
			t.Fatalf("setup replacement output missing %q:\n%s", expected, output)
		}
	}
	contents, err = os.ReadFile(installPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != sourceContents {
		t.Fatalf("installed command does not match selected source: %q", contents)
	}

	command = exec.Command(setupPath,
		"--user", "testadmin",
		"--config", configPath,
		"--sojuctl", filepath.Join(stubDir, "sojuctl"),
		"--binary", tuiBinary,
		"--no-install",
		"--dry-run",
	)
	command.Env = append(os.Environ(), "PATH="+stubDir+":"+os.Getenv("PATH"))
	output, err = command.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "Installed command:   disabled (--no-install)") {
		t.Fatalf("setup --no-install dry run failed: %v\n%s", err, output)
	}

	symlinkPath := filepath.Join(temporaryDir, "linked-command")
	if err := os.Symlink(tuiBinary, symlinkPath); err != nil {
		t.Fatal(err)
	}
	command = exec.Command(setupPath,
		"--user", "testadmin",
		"--config", configPath,
		"--sojuctl", filepath.Join(stubDir, "sojuctl"),
		"--binary", tuiBinary,
		"--install-path", symlinkPath,
		"--dry-run",
	)
	command.Env = append(os.Environ(), "PATH="+stubDir+":"+os.Getenv("PATH"))
	output, err = command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "refusing to replace a symbolic-link install path") {
		t.Fatalf("setup did not reject a symlink install path: %v\n%s", err, output)
	}
}

func writeExecutable(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o700); err != nil {
		t.Fatal(err)
	}
}
