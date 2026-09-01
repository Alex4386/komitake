package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"time"

	"github.com/Alex4386/komitake/internal/config"
)

func loadWebTLSCertificate(webAddress string, webTLS config.WebTLSFile) (tls.Certificate, string, error) {
	if webTLS.CertFile != "" {
		certificate, err := tls.LoadX509KeyPair(webTLS.CertFile, webTLS.KeyFile)
		if err != nil {
			return tls.Certificate{}, "", fmt.Errorf("load web TLS certificate: %w", err)
		}
		return certificate, "configured", nil
	}

	certificate, err := generateSelfSignedCertificate(webAddress, time.Now())
	if err != nil {
		return tls.Certificate{}, "", fmt.Errorf("generate self-signed web TLS certificate: %w", err)
	}
	return certificate, "self-signed", nil
}

func generateSelfSignedCertificate(webAddress string, now time.Time) (tls.Certificate, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject:      pkix.Name{CommonName: "Komitake Web"},
		NotBefore:    now.Add(-5 * time.Minute),
		NotAfter:     now.AddDate(1, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
	}
	addWebCertificateHost(&template, webAddress)

	certificateDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return tls.Certificate{}, err
	}
	privateKeyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return tls.Certificate{}, err
	}
	certificatePEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificateDER})
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateKeyDER})
	return tls.X509KeyPair(certificatePEM, privateKeyPEM)
}

func addWebCertificateHost(certificate *x509.Certificate, webAddress string) {
	host, _, err := net.SplitHostPort(webAddress)
	if err != nil {
		return
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		addLocalInterfaceIPs(certificate)
		return
	}
	host = trimIPv6Zone(host)
	if hostIP := net.ParseIP(host); hostIP != nil {
		if !containsIP(certificate.IPAddresses, hostIP) {
			certificate.IPAddresses = append(certificate.IPAddresses, hostIP)
		}
		return
	}
	if host != "localhost" {
		certificate.DNSNames = append(certificate.DNSNames, host)
	}
}

func addLocalInterfaceIPs(certificate *x509.Certificate) {
	interfaceAddresses, err := net.InterfaceAddrs()
	if err != nil {
		return
	}
	for _, interfaceAddress := range interfaceAddresses {
		address, _, parseErr := net.ParseCIDR(interfaceAddress.String())
		if parseErr != nil || address == nil || containsIP(certificate.IPAddresses, address) {
			continue
		}
		certificate.IPAddresses = append(certificate.IPAddresses, address)
	}
}

func trimIPv6Zone(host string) string {
	for index, character := range host {
		if character == '%' {
			return host[:index]
		}
	}
	return host
}

func containsIP(addresses []net.IP, candidate net.IP) bool {
	for _, address := range addresses {
		if address.Equal(candidate) {
			return true
		}
	}
	return false
}
