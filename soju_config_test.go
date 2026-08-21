package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseSojuConfig(t *testing.T) {
	info, err := parseSojuConfig(strings.NewReader(`
listen ircs://172.32.0.1:6697
listen unix+admin://
tls /etc/soju/tls/fullchain.pem /etc/soju/tls/privkey.pem
hostname soju.example.com
title "Private Soju"
client-cert-auth true
`))
	if err != nil {
		t.Fatal(err)
	}
	if info.Hostname != "soju.example.com" || info.Title != "Private Soju" || info.AdminSocket != "/run/soju/admin" || !info.ClientCertAuth {
		t.Fatalf("unexpected config: %#v", info)
	}
}

func TestConfirmAdminProfileDefaultsToNo(t *testing.T) {
	profile := AdminProfile{ConfigPath: "/etc/soju/config", SojuCtl: "/usr/bin/sojuctl"}
	info := SojuConfigInfo{Hostname: "soju.example.test", AdminSocket: "/run/soju/admin"}
	var output bytes.Buffer
	accepted, err := confirmAdminProfile(profile, info, strings.NewReader("\n"), &output)
	if err != nil || accepted {
		t.Fatalf("accepted=%v err=%v", accepted, err)
	}
	if !strings.Contains(output.String(), "soju.example.test") || !strings.Contains(output.String(), "/run/soju/admin") {
		t.Fatalf("review omitted discovered settings: %q", output.String())
	}
}

func TestServerTLSCertificateReportReadsCertificateOnly(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "fullchain.pem")
	keyPath := filepath.Join(dir, "unreadable-private-key.pem")
	configPath := filepath.Join(dir, "config")
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	template := x509.Certificate{
		SerialNumber: big.NewInt(42),
		Subject:      pkix.Name{CommonName: "soju.example.test"},
		DNSNames:     []string{"soju.example.test"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("hostname soju.example.test\ntls "+certPath+" "+keyPath+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := serverTLSCertificateReport(configPath, now)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(report, "matches configured hostname") || !strings.Contains(report, "SHA-256 fingerprint") || !strings.Contains(report, "contents are never read") {
		t.Fatalf("unexpected report: %s", report)
	}
}
