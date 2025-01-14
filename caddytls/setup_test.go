// Copyright 2015 Light Code Labs, LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package caddytls

import (
	"crypto/tls"
	"io/ioutil"
	"log"
	"os"
	"testing"

	"github.com/go-acme/lego/certcrypto"
	"github.com/mholt/caddy"
	"github.com/mholt/certmagic"
)

func TestMain(m *testing.M) {
	// Write test certificates to disk before tests, and clean up
	// when we're done.
	err := ioutil.WriteFile(certFile, testCert, 0644)
	if err != nil {
		log.Fatal(err)
	}
	err = ioutil.WriteFile(keyFile, testKey, 0644)
	if err != nil {
		os.Remove(certFile)
		log.Fatal(err)
	}
	err = ioutil.WriteFile(caCertFile, caCert, 0644)
	if err != nil {
		os.Remove(keyFile)
		os.Remove(certFile)
		log.Fatal(err)
	}

	result := m.Run()

	os.Remove(certFile)
	os.Remove(keyFile)
	os.Remove(caCertFile)
	os.Exit(result)
}

func TestSetupParseBasic(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "caddytls_setup_test_")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)

	certmagic.Default.Storage = &certmagic.FileStorage{Path: tmpdir}
	cfg := &Config{Manager: certmagic.NewDefault()}
	RegisterConfigGetter("", func(c *caddy.Controller) *Config { return cfg })
	c := caddy.NewTestController("", `tls `+certFile+` `+keyFile+``)

	err = setupTLS(c)
	if err != nil {
		t.Errorf("Expected no errors, got: %v", err)
	}

	// Basic checks
	if !cfg.Manual {
		t.Error("Expected TLS Manual=true, but was false")
	}
	if !cfg.Enabled {
		t.Error("Expected TLS Enabled=true, but was false")
	}

	// Security defaults
	if cfg.ProtocolMinVersion != tls.VersionTLS12 {
		t.Errorf("Expected 'tls1.2 (0x0303)' as ProtocolMinVersion, got %#v", cfg.ProtocolMinVersion)
	}
	if cfg.ProtocolMaxVersion != tls.VersionTLS13 {
		t.Errorf("Expected 'tls1.3 (0x0304)' as ProtocolMaxVersion, got %#v", cfg.ProtocolMaxVersion)
	}

	// Cipher checks
	expectedCiphers := append([]uint16{tls.TLS_FALLBACK_SCSV}, getPreferredDefaultCiphers()...)

	// Ensure count is correct (plus one for TLS_FALLBACK_SCSV)
	if len(cfg.Ciphers) != len(expectedCiphers) {
		t.Errorf("Expected %v Ciphers (including TLS_FALLBACK_SCSV), got %v",
			len(expectedCiphers), len(cfg.Ciphers))
	}

	// Ensure ordering is correct
	for i, actual := range cfg.Ciphers {
		if actual != expectedCiphers[i] {
			t.Errorf("Expected cipher in position %d to be %0x, got %0x", i, expectedCiphers[i], actual)
		}
	}

	if !cfg.PreferServerCipherSuites {
		t.Error("Expected PreferServerCipherSuites = true, but was false")
	}

	if len(cfg.ALPN) != 0 {
		t.Error("Expected ALPN empty by default")
	}

	// Ensure curve count is correct
	if len(cfg.CurvePreferences) != len(defaultCurves) {
		t.Errorf("Expected %v Curves, got %v", len(defaultCurves), len(cfg.CurvePreferences))
	}

	// Ensure curve ordering is correct
	for i, actual := range cfg.CurvePreferences {
		if actual != defaultCurves[i] {
			t.Errorf("Expected curve in position %d to be %0x, got %0x", i, defaultCurves[i], actual)
		}
	}
}

func TestSetupParseIncompleteParams(t *testing.T) {
	// Using tls without args is an error because it's unnecessary.
	c := caddy.NewTestController("", `tls`)
	err := setupTLS(c)
	if err == nil {
		t.Error("Expected an error, but didn't get one")
	}
}

func TestSetupParseWithOptionalParams(t *testing.T) {
	params := `tls ` + certFile + ` ` + keyFile + ` {
            protocols tls1.0 tls1.2
            ciphers RSA-AES256-CBC-SHA ECDHE-RSA-AES128-GCM-SHA256 ECDHE-ECDSA-AES256-GCM-SHA384
            must_staple
            alpn http/1.1
        }`

	tmpdir, err := ioutil.TempDir("", "caddytls_setup_test_")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)

	certmagic.Default.Storage = &certmagic.FileStorage{Path: tmpdir}
	cfg := &Config{Manager: certmagic.NewDefault()}
	RegisterConfigGetter("", func(c *caddy.Controller) *Config { return cfg })
	c := caddy.NewTestController("", params)

	err = setupTLS(c)
	if err != nil {
		t.Errorf("Expected no errors, got: %v", err)
	}

	if cfg.ProtocolMinVersion != tls.VersionTLS10 {
		t.Errorf("Expected 'tls1.0 (0x0301)' as ProtocolMinVersion, got %#v", cfg.ProtocolMinVersion)
	}

	if cfg.ProtocolMaxVersion != tls.VersionTLS12 {
		t.Errorf("Expected 'tls1.2 (0x0303)' as ProtocolMaxVersion, got %#v", cfg.ProtocolMaxVersion)
	}

	if len(cfg.Ciphers)-1 != 3 {
		t.Errorf("Expected 3 Ciphers (not including TLS_FALLBACK_SCSV), got %v", len(cfg.Ciphers)-1)
	}

	if !cfg.Manager.MustStaple {
		t.Error("Expected must staple to be true")
	}

	if len(cfg.ALPN) != 1 || cfg.ALPN[0] != "http/1.1" {
		t.Errorf("Expected ALPN to contain only 'http/1.1' but got: %v", cfg.ALPN)
	}
}

func TestSetupDefaultWithOptionalParams(t *testing.T) {
	params := `tls {
            ciphers RSA-3DES-EDE-CBC-SHA
        }`
	cfg := &Config{Manager: &certmagic.Config{}}
	RegisterConfigGetter("", func(c *caddy.Controller) *Config { return cfg })
	c := caddy.NewTestController("", params)

	err := setupTLS(c)
	if err != nil {
		t.Errorf("Expected no errors, got: %v", err)
	}
	if len(cfg.Ciphers)-1 != 1 {
		t.Errorf("Expected 1 ciphers (not including TLS_FALLBACK_SCSV), got %v", len(cfg.Ciphers)-1)
	}
}

func TestSetupParseWithWrongOptionalParams(t *testing.T) {
	// Test protocols wrong params
	params := `tls ` + certFile + ` ` + keyFile + ` {
			protocols ssl tls
		}`
	cfg := &Config{Manager: &certmagic.Config{}}
	RegisterConfigGetter("", func(c *caddy.Controller) *Config { return cfg })
	c := caddy.NewTestController("", params)

	err := setupTLS(c)
	if err == nil {
		t.Errorf("Expected errors, but no error returned")
	}

	// Test ciphers wrong params
	params = `tls ` + certFile + ` ` + keyFile + ` {
			ciphers not-valid-cipher
		}`
	cfg = &Config{Manager: &certmagic.Config{}}
	RegisterConfigGetter("", func(c *caddy.Controller) *Config { return cfg })
	c = caddy.NewTestController("", params)
	err = setupTLS(c)
	if err == nil {
		t.Error("Expected errors, but no error returned")
	}

	// Test key_type wrong params
	params = `tls {
			key_type ab123
		}`
	cfg = &Config{Manager: &certmagic.Config{}}
	RegisterConfigGetter("", func(c *caddy.Controller) *Config { return cfg })
	c = caddy.NewTestController("", params)
	err = setupTLS(c)
	if err == nil {
		t.Error("Expected errors, but no error returned")
	}

	// Test curves wrong params
	params = `tls {
			curves ab123, cd456, ef789
		}`
	cfg = &Config{Manager: &certmagic.Config{}}
	RegisterConfigGetter("", func(c *caddy.Controller) *Config { return cfg })
	c = caddy.NewTestController("", params)
	err = setupTLS(c)
	if err == nil {
		t.Error("Expected errors, but no error returned")
	}
}

func TestSetupParseWithClientAuth(t *testing.T) {
	// Test missing client cert file
	params := `tls ` + certFile + ` ` + keyFile + ` {
			clients
		}`
	cfg := &Config{Manager: &certmagic.Config{}}
	RegisterConfigGetter("", func(c *caddy.Controller) *Config { return cfg })
	c := caddy.NewTestController("", params)
	err := setupTLS(c)
	if err == nil {
		t.Error("Expected an error, but no error returned")
	}

	noCAs, twoCAs := []string{}, []string{"client_ca.crt", "client2_ca.crt"}
	for caseNumber, caseData := range []struct {
		params         string
		clientAuthType tls.ClientAuthType
		expectedErr    bool
		expectedCAs    []string
	}{
		{"", tls.NoClientCert, false, noCAs},
		{`tls ` + certFile + ` ` + keyFile + ` {
			clients client_ca.crt client2_ca.crt
		}`, tls.RequireAndVerifyClientCert, false, twoCAs},
		// now come modifier
		{`tls ` + certFile + ` ` + keyFile + ` {
			clients request
		}`, tls.RequestClientCert, false, noCAs},
		{`tls ` + certFile + ` ` + keyFile + ` {
			clients require
		}`, tls.RequireAnyClientCert, false, noCAs},
		{`tls ` + certFile + ` ` + keyFile + ` {
			clients verify_if_given client_ca.crt client2_ca.crt
		}`, tls.VerifyClientCertIfGiven, false, twoCAs},
		{`tls ` + certFile + ` ` + keyFile + ` {
			clients verify_if_given
		}`, tls.VerifyClientCertIfGiven, true, noCAs},
	} {
		cfg := &Config{Manager: certmagic.NewDefault()}
		RegisterConfigGetter("", func(c *caddy.Controller) *Config { return cfg })
		c := caddy.NewTestController("", caseData.params)

		err := setupTLS(c)
		if caseData.expectedErr {
			if err == nil {
				t.Errorf("In case %d: Expected an error, got: %v", caseNumber, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("In case %d: Expected no errors, got: %v", caseNumber, err)
		}

		if caseData.clientAuthType != cfg.ClientAuth {
			t.Errorf("In case %d: Expected TLS client auth type %v, got: %v",
				caseNumber, caseData.clientAuthType, cfg.ClientAuth)
		}

		if count := len(cfg.ClientCerts); count < len(caseData.expectedCAs) {
			t.Fatalf("In case %d: Expected %d client certs, had %d", caseNumber, len(caseData.expectedCAs), count)
		}

		for idx, expected := range caseData.expectedCAs {
			if actual := cfg.ClientCerts[idx]; actual != expected {
				t.Errorf("In case %d: Expected %dth client cert file to be '%s', but was '%s'",
					caseNumber, idx, expected, actual)
			}
		}
	}
}

func TestSetupParseWithCAUrl(t *testing.T) {
	testURL := "https://acme-staging.api.letsencrypt.org/directory"
	for caseNumber, caseData := range []struct {
		params        string
		expectedErr   bool
		expectedCAUrl string
	}{
		// Test working case
		{`tls {
				ca ` + testURL + `
			}`, false, testURL},
		// Test too few args
		{`tls {
				ca
			}`, true, ""},
		// Test too many args
		{`tls {
				ca 1 2
			}`, true, ""},
	} {
		cfg := &Config{Manager: &certmagic.Config{}}
		RegisterConfigGetter("", func(c *caddy.Controller) *Config { return cfg })
		c := caddy.NewTestController("", caseData.params)

		err := setupTLS(c)
		if caseData.expectedErr {
			if err == nil {
				t.Errorf("In case %d: Expected an error, got: %v", caseNumber, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("In case %d: Expected no errors, got: %v", caseNumber, err)
		}

		if cfg.Manager.CA != caseData.expectedCAUrl {
			t.Errorf("Expected '%v' as CAUrl, got %#v", caseData.expectedCAUrl, cfg.Manager.CA)
		}
	}
}

func TestSetupParseWithKeyType(t *testing.T) {
	params := `tls {
            key_type p384
        }`
	cfg := &Config{Manager: &certmagic.Config{}}
	RegisterConfigGetter("", func(c *caddy.Controller) *Config { return cfg })
	c := caddy.NewTestController("", params)

	err := setupTLS(c)
	if err != nil {
		t.Errorf("Expected no errors, got: %v", err)
	}

	if cfg.Manager.KeyType != certcrypto.EC384 {
		t.Errorf("Expected 'P384' as KeyType, got %#v", cfg.Manager.KeyType)
	}
}

func TestSetupParseWithCurves(t *testing.T) {
	params := `tls {
            curves x25519 p256 p384 p521
        }`
	cfg := &Config{Manager: &certmagic.Config{}}
	RegisterConfigGetter("", func(c *caddy.Controller) *Config { return cfg })
	c := caddy.NewTestController("", params)

	err := setupTLS(c)
	if err != nil {
		t.Errorf("Expected no errors, got: %v", err)
	}

	if len(cfg.CurvePreferences) != 4 {
		t.Errorf("Expected 4 curves, got %v", len(cfg.CurvePreferences))
	}

	expectedCurves := []tls.CurveID{tls.X25519, tls.CurveP256, tls.CurveP384, tls.CurveP521}

	// Ensure ordering is correct
	for i, actual := range cfg.CurvePreferences {
		if actual != expectedCurves[i] {
			t.Errorf("Expected curve in position %d to be %v, got %v", i, expectedCurves[i], actual)
		}
	}
}

func TestSetupParseWithOneTLSProtocol(t *testing.T) {
	params := `tls {
            protocols tls1.2
        }`
	cfg := &Config{Manager: &certmagic.Config{}}
	RegisterConfigGetter("", func(c *caddy.Controller) *Config { return cfg })
	c := caddy.NewTestController("", params)

	err := setupTLS(c)
	if err != nil {
		t.Errorf("Expected no errors, got: %v", err)
	}

	if cfg.ProtocolMinVersion != cfg.ProtocolMaxVersion {
		t.Errorf("Expected ProtocolMinVersion to be the same as ProtocolMaxVersion")
	}

	if cfg.ProtocolMinVersion != tls.VersionTLS12 && cfg.ProtocolMaxVersion != tls.VersionTLS12 {
		t.Errorf("Expected 'tls1.2 (0x0303)' as ProtocolMinVersion/ProtocolMaxVersion, got %v/%v", cfg.ProtocolMinVersion, cfg.ProtocolMaxVersion)
	}
}

func TestSetupParseWithEmail(t *testing.T) {
	email := "user@example.com"
	params := "tls " + email
	cfg := &Config{Manager: &certmagic.Config{}}
	RegisterConfigGetter("", func(c *caddy.Controller) *Config { return cfg })
	c := caddy.NewTestController("", params)

	err := setupTLS(c)
	if err != nil {
		t.Errorf("Expected no errors, got: %v", err)
	}

	if cfg.ACMEEmail != email {
		t.Errorf("Expected cfg.ACMEEmail to be %#v, got %#v", email, cfg.ACMEEmail)
	}
	if cfg.Manager.Email != email {
		t.Errorf("Expected cfg.Manager.Email to be %#v, got %#v", email, cfg.Manager.Email)
	}
}

const (
	certFile   = "test_cert.pem"
	keyFile    = "test_key.pem"
	caCertFile = "ca_cert.crt"
)

var testCert = []byte(`-----BEGIN CERTIFICATE-----
MIIBkjCCATmgAwIBAgIJANfFCBcABL6LMAkGByqGSM49BAEwFDESMBAGA1UEAxMJ
bG9jYWxob3N0MB4XDTE2MDIxMDIyMjAyNFoXDTE4MDIwOTIyMjAyNFowFDESMBAG
A1UEAxMJbG9jYWxob3N0MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEs22MtnG7
9K1mvIyjEO9GLx7BFD0tBbGnwQ0VPsuCxC6IeVuXbQDLSiVQvFZ6lUszTlczNxVk
pEfqrM6xAupB7qN1MHMwHQYDVR0OBBYEFHxYDvAxUwL4XrjPev6qZ/BiLDs5MEQG
A1UdIwQ9MDuAFHxYDvAxUwL4XrjPev6qZ/BiLDs5oRikFjAUMRIwEAYDVQQDEwls
b2NhbGhvc3SCCQDXxQgXAAS+izAMBgNVHRMEBTADAQH/MAkGByqGSM49BAEDSAAw
RQIgRvBqbyJM2JCJqhA1FmcoZjeMocmhxQHTt1c+1N2wFUgCIQDtvrivbBPA688N
Qh3sMeAKNKPsx5NxYdoWuu9KWcKz9A==
-----END CERTIFICATE-----
`)

var testKey = []byte(`-----BEGIN EC PARAMETERS-----
BggqhkjOPQMBBw==
-----END EC PARAMETERS-----
-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIGLtRmwzYVcrH3J0BnzYbGPdWVF10i9p6mxkA4+b2fURoAoGCCqGSM49
AwEHoUQDQgAEs22MtnG79K1mvIyjEO9GLx7BFD0tBbGnwQ0VPsuCxC6IeVuXbQDL
SiVQvFZ6lUszTlczNxVkpEfqrM6xAupB7g==
-----END EC PRIVATE KEY-----
`)

var caCert = []byte(`-----BEGIN CERTIFICATE-----
MIIF2DCCA8CgAwIBAgIQTKr5yttjb+Af907YWwOGnTANBgkqhkiG9w0BAQwFADCB
hTELMAkGA1UEBhMCR0IxGzAZBgNVBAgTEkdyZWF0ZXIgTWFuY2hlc3RlcjEQMA4G
A1UEBxMHU2FsZm9yZDEaMBgGA1UEChMRQ09NT0RPIENBIExpbWl0ZWQxKzApBgNV
BAMTIkNPTU9ETyBSU0EgQ2VydGlmaWNhdGlvbiBBdXRob3JpdHkwHhcNMTAwMTE5
MDAwMDAwWhcNMzgwMTE4MjM1OTU5WjCBhTELMAkGA1UEBhMCR0IxGzAZBgNVBAgT
EkdyZWF0ZXIgTWFuY2hlc3RlcjEQMA4GA1UEBxMHU2FsZm9yZDEaMBgGA1UEChMR
Q09NT0RPIENBIExpbWl0ZWQxKzApBgNVBAMTIkNPTU9ETyBSU0EgQ2VydGlmaWNh
dGlvbiBBdXRob3JpdHkwggIiMA0GCSqGSIb3DQEBAQUAA4ICDwAwggIKAoICAQCR
6FSS0gpWsawNJN3Fz0RndJkrN6N9I3AAcbxT38T6KhKPS38QVr2fcHK3YX/JSw8X
pz3jsARh7v8Rl8f0hj4K+j5c+ZPmNHrZFGvnnLOFoIJ6dq9xkNfs/Q36nGz637CC
9BR++b7Epi9Pf5l/tfxnQ3K9DADWietrLNPtj5gcFKt+5eNu/Nio5JIk2kNrYrhV
/erBvGy2i/MOjZrkm2xpmfh4SDBF1a3hDTxFYPwyllEnvGfDyi62a+pGx8cgoLEf
Zd5ICLqkTqnyg0Y3hOvozIFIQ2dOciqbXL1MGyiKXCJ7tKuY2e7gUYPDCUZObT6Z
+pUX2nwzV0E8jVHtC7ZcryxjGt9XyD+86V3Em69FmeKjWiS0uqlWPc9vqv9JWL7w
qP/0uK3pN/u6uPQLOvnoQ0IeidiEyxPx2bvhiWC4jChWrBQdnArncevPDt09qZah
SL0896+1DSJMwBGB7FY79tOi4lu3sgQiUpWAk2nojkxl8ZEDLXB0AuqLZxUpaVIC
u9ffUGpVRr+goyhhf3DQw6KqLCGqR84onAZFdr+CGCe01a60y1Dma/RMhnEw6abf
Fobg2P9A3fvQQoh/ozM6LlweQRGBY84YcWsr7KaKtzFcOmpH4MN5WdYgGq/yapiq
crxXStJLnbsQ/LBMQeXtHT1eKJ2czL+zUdqnR+WEUwIDAQABo0IwQDAdBgNVHQ4E
FgQUu69+Aj36pvE8hI6t7jiY7NkyMtQwDgYDVR0PAQH/BAQDAgEGMA8GA1UdEwEB
/wQFMAMBAf8wDQYJKoZIhvcNAQEMBQADggIBAArx1UaEt65Ru2yyTUEUAJNMnMvl
wFTPoCWOAvn9sKIN9SCYPBMtrFaisNZ+EZLpLrqeLppysb0ZRGxhNaKatBYSaVqM
4dc+pBroLwP0rmEdEBsqpIt6xf4FpuHA1sj+nq6PK7o9mfjYcwlYRm6mnPTXJ9OV
2jeDchzTc+CiR5kDOF3VSXkAKRzH7JsgHAckaVd4sjn8OoSgtZx8jb8uk2Intzna
FxiuvTwJaP+EmzzV1gsD41eeFPfR60/IvYcjt7ZJQ3mFXLrrkguhxuhoqEwWsRqZ
CuhTLJK7oQkYdQxlqHvLI7cawiiFwxv/0Cti76R7CZGYZ4wUAc1oBmpjIXUDgIiK
boHGhfKppC3n9KUkEEeDys30jXlYsQab5xoq2Z0B15R97QNKyvDb6KkBPvVWmcke
jkk9u+UJueBPSZI9FoJAzMxZxuY67RIuaTxslbH9qh17f4a+Hg4yRvv7E491f0yL
S0Zj/gA0QHDBw7mh3aZw4gSzQbzpgJHqZJx64SIDqZxubw5lT2yHh17zbqD5daWb
QOhTsiedSrnAdyGN/4fy3ryM7xfft0kL0fJuMAsaDk527RH89elWsn2/x20Kk4yl
0MC2Hb46TpSi125sC8KKfPog88Tk5c0NqMuRkrF8hey1FGlmDoLnzc7ILaZRfyHB
NVOFBkpdn627G190
-----END CERTIFICATE-----
`)
