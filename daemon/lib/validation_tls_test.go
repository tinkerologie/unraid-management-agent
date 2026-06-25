package lib

import (
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

// writeTestKeyPair generates a throwaway self-signed certificate/key pair and
// writes them to the given directory, returning the absolute file paths. It is
// only used to exercise the loadable-key-pair branch of ValidateTLSConfig.
func writeTestKeyPair(t *testing.T, dir string) (certPath, keyPath string) {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("creating certificate: %v", err)
	}

	certPath = filepath.Join(dir, "cert.pem")
	certOut := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(certPath, certOut, 0o600); err != nil {
		t.Fatalf("writing cert: %v", err)
	}

	keyBytes, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		t.Fatalf("marshaling key: %v", err)
	}
	keyPath = filepath.Join(dir, "key.pem")
	keyOut := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})
	if err := os.WriteFile(keyPath, keyOut, 0o600); err != nil {
		t.Fatalf("writing key: %v", err)
	}

	return certPath, keyPath
}

func TestValidateTLSConfig(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := writeTestKeyPair(t, dir)

	// A loadable pair living under a directory whose name merely contains dots
	// (not a ".." segment). This must be accepted — it guards against an
	// over-broad traversal check rejecting legitimate paths.
	dottedDir := filepath.Join(dir, "archive..old")
	if err := os.MkdirAll(dottedDir, 0o700); err != nil {
		t.Fatalf("mkdir dotted dir: %v", err)
	}
	dottedCert, dottedKey := writeTestKeyPair(t, dottedDir)

	tests := []struct {
		name     string
		certFile string
		keyFile  string
		wantErr  bool
	}{
		{name: "both empty is TLS disabled", certFile: "", keyFile: "", wantErr: false},
		{name: "valid loadable key pair", certFile: certPath, keyFile: keyPath, wantErr: false},
		{name: "dotted but legitimate component name", certFile: dottedCert, keyFile: dottedKey, wantErr: false},
		{name: "only cert provided", certFile: certPath, keyFile: "", wantErr: true},
		{name: "only key provided", certFile: "", keyFile: keyPath, wantErr: true},
		{name: "relative cert path", certFile: "certs/cert.pem", keyFile: keyPath, wantErr: true},
		{name: "path traversal in key (unix)", certFile: certPath, keyFile: "/boot/../etc/key.pem", wantErr: true},
		{name: "path traversal in key (windows sep)", certFile: certPath, keyFile: `/boot/..\etc/key.pem`, wantErr: true},
		{name: "relative traversal cert", certFile: "../../etc/passwd", keyFile: keyPath, wantErr: true},
		{name: "null byte in cert", certFile: "/boot/cert\x00.pem", keyFile: keyPath, wantErr: true},
		{name: "arbitrary absolute file is not a cert", certFile: "/etc/passwd", keyFile: keyPath, wantErr: true},
		{name: "command-injection style path", certFile: "/boot/cert.pem; rm -rf /", keyFile: keyPath, wantErr: true},
		{name: "subshell style path", certFile: "/boot/$(whoami)/cert.pem", keyFile: keyPath, wantErr: true},
		{name: "backtick style path", certFile: "/boot/`id`/cert.pem", keyFile: keyPath, wantErr: true},
		{name: "missing files but valid paths", certFile: "/nonexistent/cert.pem", keyFile: "/nonexistent/key.pem", wantErr: true},
		{name: "key file is not a key pair", certFile: certPath, keyFile: certPath, wantErr: true},
		{name: "over-long cert path", certFile: "/" + strings.Repeat("a", maxTLSPathLen), keyFile: keyPath, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTLSConfig(tt.certFile, tt.keyFile)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTLSConfig(%q, %q) error = %v, wantErr %v", tt.certFile, tt.keyFile, err, tt.wantErr)
			}
		})
	}
}
