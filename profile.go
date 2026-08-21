package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// AdminProfile stores only local tool/configuration paths. It never stores
// soju credentials because sojuctl authenticates through the Unix admin socket.
type AdminProfile struct {
	ConfigPath string `json:"config_path"`
	SojuCtl    string `json:"sojuctl"`
}

func defaultAdminProfilePath(explicit string) string {
	if explicit != "" {
		return explicit
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "soju-tui", "admin.json")
}

func loadAdminProfile(path string) (AdminProfile, error) {
	if path == "" {
		return AdminProfile{}, nil
	}
	// #nosec G304,G703 -- this is the operator-selected per-user profile path.
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return AdminProfile{}, nil
	}
	if err != nil {
		return AdminProfile{}, err
	}
	var profile AdminProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return AdminProfile{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return profile, nil
}

func saveAdminProfile(path string, profile AdminProfile) error {
	if path == "" {
		return nil
	}
	dir := filepath.Dir(path)
	// #nosec G703 -- the directory belongs to the unprivileged account that
	// explicitly selected the profile path; the profile itself remains 0600.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temporary, err := os.CreateTemp(dir, ".admin-*.tmp")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	// #nosec G703 -- atomic replacement targets the operator-selected profile;
	// the temporary inode is already mode 0600 and rename preserves that mode.
	if err := os.Rename(temporaryName, path); err != nil {
		return err
	}
	return nil
}

func resolveSojuCtl(explicit, saved string) (string, error) {
	candidate := explicit
	if candidate == "" {
		candidate = saved
	}
	if candidate == "" {
		candidate = "sojuctl"
	}
	path, err := exec.LookPath(candidate)
	if err != nil {
		return "", fmt.Errorf("sojuctl %q was not found in PATH; install soju or use -sojuctl PATH: %w", candidate, err)
	}
	return path, nil
}
