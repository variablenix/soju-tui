package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// SavedProfile contains connection preferences only. Passwords are deliberately
// not part of this file and continue to be requested interactively or supplied
// through SOJU_PASSWORD.
type SavedProfile struct {
	Server        string `json:"server"`
	TLS           *bool  `json:"tls,omitempty"`
	TLSServerName string `json:"tls_server_name,omitempty"`
	Username      string `json:"username,omitempty"`
	Nick          string `json:"nick,omitempty"`
	Realname      string `json:"realname,omitempty"`
	ClientName    string `json:"client_name,omitempty"`
	NetworkFilter string `json:"network,omitempty"`
}

type discoveredSojuConfig struct {
	Server        string
	TLS           bool
	TLSServerName string
}

func defaultProfilePath(explicit string) string {
	if explicit != "" {
		return explicit
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "soju-tui", "config.json")
}

func loadSavedProfile(path string) (SavedProfile, error) {
	if path == "" {
		return SavedProfile{}, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return SavedProfile{}, nil
	}
	if err != nil {
		return SavedProfile{}, err
	}
	var profile SavedProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return SavedProfile{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return profile, nil
}

func saveProfile(path string, profile SavedProfile) error {
	if path == "" {
		return nil
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temporary, err := os.CreateTemp(dir, ".config-*.tmp")
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
	if err := os.Rename(temporaryName, path); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func discoverSojuConfig(path string) (discoveredSojuConfig, error) {
	if path == "" {
		return discoveredSojuConfig{}, nil
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, os.ErrPermission) {
		return discoveredSojuConfig{}, nil
	}
	if err != nil {
		return discoveredSojuConfig{}, err
	}
	defer file.Close()

	var discovered discoveredSojuConfig
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 64*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || discovered.Server != "" {
			if discovered.Server != "" {
				if fields := strings.Fields(line); len(fields) >= 2 && fields[0] == "hostname" && discovered.TLSServerName == "" {
					discovered.TLSServerName = strings.Trim(fields[1], "\"'")
				}
			}
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "listen":
			server, tlsEnabled, ok := parseSojuListen(fields[1])
			if ok {
				discovered.Server = server
				discovered.TLS = tlsEnabled
			}
		case "hostname":
			discovered.TLSServerName = strings.Trim(fields[1], "\"'")
		}
	}
	if err := scanner.Err(); err != nil {
		return discoveredSojuConfig{}, err
	}
	return discovered, nil
}

func parseSojuListen(value string) (server string, tlsEnabled, ok bool) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		return "", false, false
	}
	scheme := strings.ToLower(parsed.Scheme)
	if strings.Contains(scheme, "unix") {
		return "", false, false
	}
	tlsEnabled = scheme == "ircs"
	host := parsed.Hostname()
	if host == "" {
		host = "127.0.0.1"
	}
	port := parsed.Port()
	if port == "" {
		if tlsEnabled {
			port = "6697"
		} else {
			port = "6667"
		}
	}
	return net.JoinHostPort(host, port), tlsEnabled, true
}
