package server

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestGenerateSelfSignedServesHTTPS(t *testing.T) {
	cert, err := GenerateSelfSigned()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	// The generated cert must actually complete a TLS handshake and serve — a
	// cert that merely parses could still fail the handshake.
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "ok")
	})}
	go srv.Serve(ln)
	defer srv.Close()

	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}}
	resp, err := client.Get("https://" + ln.Addr().String())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Errorf("body = %q, want ok", body)
	}

	// SANs must cover loopback so a strict client can verify against 127.0.0.1.
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("parse leaf: %v", err)
	}
	if err := leaf.VerifyHostname("127.0.0.1"); err != nil {
		t.Errorf("cert should be valid for 127.0.0.1: %v", err)
	}
	if leaf.VerifyHostname("localhost") != nil {
		t.Error("cert should be valid for localhost")
	}

	// Fingerprint is the openssl-style colon-separated SHA-256 (32 bytes).
	fp := CertFingerprint(cert)
	if n := strings.Count(fp, ":"); n != 31 {
		t.Errorf("fingerprint %q has %d colons, want 31", fp, n)
	}
	if CertFingerprint(tls.Certificate{}) != "" {
		t.Error("empty cert should yield empty fingerprint")
	}
}
