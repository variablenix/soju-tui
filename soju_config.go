package main

import (
	"bufio"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type SojuConfigInfo struct {
	Hostname       string
	Title          string
	AdminSocket    string
	TLSCertPath    string
	TLSKeyPath     string
	ClientCertAuth bool
}

func readSojuConfig(path string) (SojuConfigInfo, error) {
	// #nosec G304,G703 -- the local operator explicitly selects the Soju config;
	// reading any file already readable by that same account crosses no boundary.
	file, err := os.Open(path)
	if err != nil {
		return SojuConfigInfo{}, fmt.Errorf("read soju config %s: %w", path, err)
	}
	defer file.Close()
	return parseSojuConfig(file)
}

func parseSojuConfig(reader io.Reader) (SojuConfigInfo, error) {
	var info SojuConfigInfo
	scanner := bufio.NewScanner(reader)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		words, err := splitConfigWords(scanner.Text())
		if err != nil {
			return SojuConfigInfo{}, fmt.Errorf("parse soju config line %d: %w", lineNumber, err)
		}
		if len(words) == 0 {
			continue
		}
		switch words[0] {
		case "hostname":
			if len(words) >= 2 {
				info.Hostname = words[1]
			}
		case "title":
			if len(words) >= 2 {
				info.Title = strings.Join(words[1:], " ")
			}
		case "tls":
			if len(words) >= 3 {
				info.TLSCertPath = words[1]
				info.TLSKeyPath = words[2]
			}
		case "client-cert-auth":
			if len(words) >= 2 {
				enabled, err := strconv.ParseBool(words[1])
				if err != nil {
					return SojuConfigInfo{}, fmt.Errorf("parse soju config line %d client-cert-auth: %w", lineNumber, err)
				}
				info.ClientCertAuth = enabled
			}
		case "listen":
			if len(words) >= 2 && strings.HasPrefix(words[1], "unix+admin://") {
				path := strings.TrimPrefix(words[1], "unix+admin://")
				if path == "" {
					path = "/run/soju/admin"
				}
				info.AdminSocket = path
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return SojuConfigInfo{}, err
	}
	return info, nil
}

func splitConfigWords(line string) ([]string, error) {
	var words []string
	var current []rune
	var quote rune
	escaped := false
	flush := func() {
		if len(current) > 0 {
			words = append(words, string(current))
			current = nil
		}
	}
	for _, char := range line {
		if escaped {
			current = append(current, char)
			escaped = false
			continue
		}
		if char == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if char == quote {
				quote = 0
			} else {
				current = append(current, char)
			}
			continue
		}
		switch char {
		case '\'', '"':
			quote = char
		case '#':
			flush()
			return words, nil
		case ' ', '\t':
			flush()
		default:
			current = append(current, char)
		}
	}
	if escaped || quote != 0 {
		return nil, errors.New("unterminated quote or escape")
	}
	flush()
	return words, nil
}

func serverTLSCertificateReport(configPath string, now time.Time) (string, error) {
	info, err := readSojuConfig(configPath)
	if err != nil {
		return "", err
	}
	if info.TLSCertPath == "" {
		return "", fmt.Errorf("%s has no tls certificate directive", configPath)
	}
	if !filepath.IsAbs(info.TLSCertPath) {
		return "", fmt.Errorf("TLS certificate path %q is relative and depends on Soju's process working directory; use an absolute tls path in %s so the TUI cannot inspect the wrong file", info.TLSCertPath, configPath)
	}
	data, err := os.ReadFile(info.TLSCertPath)
	if err != nil {
		return "", fmt.Errorf("read server TLS certificate %s: %w", info.TLSCertPath, err)
	}
	block, rest := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" {
		return "", fmt.Errorf("%s does not begin with a PEM certificate", info.TLSCertPath)
	}
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("parse server TLS certificate: %w", err)
	}
	chainLength := 1
	for len(rest) > 0 {
		var next *pem.Block
		next, rest = pem.Decode(rest)
		if next == nil {
			break
		}
		if next.Type == "CERTIFICATE" {
			chainLength++
		}
	}
	fingerprint := sha256.Sum256(certificate.Raw)
	status := "valid"
	if now.Before(certificate.NotBefore) {
		status = "not valid yet"
	} else if now.After(certificate.NotAfter) {
		status = "EXPIRED"
	}
	hostnameStatus := "not checked (no hostname configured)"
	if info.Hostname != "" {
		if err := certificate.VerifyHostname(info.Hostname); err != nil {
			hostnameStatus = "MISMATCH: " + err.Error()
		} else {
			hostnameStatus = "matches configured hostname"
		}
	}
	lines := []string{
		"SOJU SERVER TLS CERTIFICATE",
		"Configuration: " + configPath,
		"Hostname: " + displayUnknown(info.Hostname),
		"Title: " + displayUnknown(info.Title),
		"Certificate: " + info.TLSCertPath,
		"Private key: " + displayUnknown(info.TLSKeyPath) + " (contents are never read)",
		"Subject: " + certificate.Subject.String(),
		"Issuer: " + certificate.Issuer.String(),
		"DNS names: " + displayUnknown(strings.Join(certificate.DNSNames, ", ")),
		"Serial: " + certificate.SerialNumber.String(),
		"Valid from: " + certificate.NotBefore.Format(time.RFC3339),
		"Valid until: " + certificate.NotAfter.Format(time.RFC3339),
		"Status: " + status,
		"Hostname check: " + hostnameStatus,
		"SHA-256 fingerprint: " + colonHex(fingerprint[:]),
		fmt.Sprintf("Certificates in PEM chain: %d", chainLength),
	}
	return strings.Join(lines, "\n"), nil
}

func displayUnknown(value string) string {
	if strings.TrimSpace(value) == "" {
		return "(not configured)"
	}
	return value
}

func colonHex(value []byte) string {
	encoded := strings.ToUpper(hex.EncodeToString(value))
	var groups []string
	for i := 0; i < len(encoded); i += 2 {
		groups = append(groups, encoded[i:i+2])
	}
	return strings.Join(groups, ":")
}

func (a *App) adminShowServerTLSLocked() {
	report, err := serverTLSCertificateReport(a.backend.Config, time.Now())
	a.admin.Output = append(a.admin.Output, "> inspect server TLS certificate from "+a.backend.Config)
	if err != nil {
		a.admin.Output = append(a.admin.Output, "ERROR: "+err.Error())
		a.setStatusLocked("server TLS certificate inspection failed", 0)
	} else {
		a.admin.Output = append(a.admin.Output, report)
		a.setStatusLocked("server TLS certificate inspected (private key was not read)", 5*time.Second)
	}
	a.admin.Output = trimOutput(a.admin.Output)
	a.admin.View = adminOutput
}
