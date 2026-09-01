package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Alex4386/komitake/internal/config"
)

func TestGenerateSelfSignedCertificateIncludesBindHost(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)
	for _, testCase := range []struct {
		name        string
		address     string
		wantDNSName string
		wantIP      net.IP
	}{
		{name: "hostname", address: "komitake.local:8443", wantDNSName: "komitake.local"},
		{name: "IPv4", address: "192.168.137.1:8443", wantIP: net.ParseIP("192.168.137.1")},
		{name: "IPv6", address: "[fd00::1]:8443", wantIP: net.ParseIP("fd00::1")},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			certificate, err := generateSelfSignedCertificate(testCase.address, now)
			if err != nil {
				t.Fatal(err)
			}
			parsed := parseLeafCertificate(t, certificate)
			if parsed.Subject.CommonName != "Komitake Web" {
				t.Fatalf("common name = %q", parsed.Subject.CommonName)
			}
			if parsed.NotBefore.After(now) || parsed.NotAfter.Before(now.AddDate(0, 11, 0)) {
				t.Fatalf("validity = %s to %s", parsed.NotBefore, parsed.NotAfter)
			}
			if testCase.wantDNSName != "" && !containsString(parsed.DNSNames, testCase.wantDNSName) {
				t.Fatalf("DNS names = %v", parsed.DNSNames)
			}
			if testCase.wantIP != nil && !containsIP(parsed.IPAddresses, testCase.wantIP) {
				t.Fatalf("IP addresses = %v", parsed.IPAddresses)
			}
			if !containsString(parsed.DNSNames, "localhost") || !containsIP(parsed.IPAddresses, net.IPv4(127, 0, 0, 1)) || !containsIP(parsed.IPAddresses, net.IPv6loopback) {
				t.Fatalf("loopback SANs missing: DNS=%v IP=%v", parsed.DNSNames, parsed.IPAddresses)
			}
		})
	}
}

func TestLoadWebTLSCertificateModes(t *testing.T) {
	t.Parallel()
	selfSigned, source, err := loadWebTLSCertificate("127.0.0.1:8443", config.WebTLSFile{Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if source != "self-signed" || len(selfSigned.Certificate) == 0 {
		t.Fatalf("self-signed source = %q, certificate count = %d", source, len(selfSigned.Certificate))
	}

	directory := t.TempDir()
	certFile := filepath.Join(directory, "server.crt")
	keyFile := filepath.Join(directory, "server.key")
	writeCertificatePair(t, selfSigned, certFile, keyFile)
	configured, source, err := loadWebTLSCertificate("127.0.0.1:8443", config.WebTLSFile{
		Enabled: true, CertFile: certFile, KeyFile: keyFile,
	})
	if err != nil {
		t.Fatal(err)
	}
	if source != "configured" || len(configured.Certificate) == 0 {
		t.Fatalf("configured source = %q, certificate count = %d", source, len(configured.Certificate))
	}
}

func parseLeafCertificate(t *testing.T, certificate tls.Certificate) *x509.Certificate {
	t.Helper()
	if len(certificate.Certificate) == 0 {
		t.Fatal("certificate chain is empty")
	}
	parsed, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func writeCertificatePair(t *testing.T, certificate tls.Certificate, certFile, keyFile string) {
	t.Helper()
	certificatePEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Certificate[0]})
	privateKeyDER, err := x509.MarshalPKCS8PrivateKey(certificate.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateKeyDER})
	if err := os.WriteFile(certFile, certificatePEM, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, privateKeyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
