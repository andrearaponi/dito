package e2e

// TLS material of the run: a CA, a server certificate for 127.0.0.1 and a
// client certificate, as PEM files that configurations reference with
// {{ca "backend"}}, {{clientCert}} and {{clientKey}}.

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sync"
	"text/template"
	"time"
)

type tlsMaterial struct {
	caFile     string
	clientCert string
	clientKey  string
	caPool     *x509.CertPool
	server     tls.Certificate
	client     tls.Certificate
}

var tlsState struct {
	once sync.Once
	mat  *tlsMaterial
	err  error
}

// runTLS returns the TLS material of the run, generated on first use.
func runTLS() (*tlsMaterial, error) {
	tlsState.once.Do(func() {
		if sharedDir() == "" {
			tlsState.err = errors.New("no shared directory")
			return
		}
		tlsState.mat, tlsState.err = generateTLS(filepath.Join(sharedDir(), "tls"))
	})
	return tlsState.mat, tlsState.err
}

func init() {
	templateFuncExtensions = append(templateFuncExtensions, func(s *S, funcs template.FuncMap) {
		material := func() (*tlsMaterial, error) {
			m, err := runTLS()
			if err != nil {
				return nil, fmt.Errorf("TLS material: %w", err)
			}
			return m, nil
		}
		funcs["ca"] = func(name string) (string, error) {
			if _, ok := s.backends[name]; !ok {
				return "", fmt.Errorf("unknown backend %q", name)
			}
			m, err := material()
			if err != nil {
				return "", err
			}
			return m.caFile, nil
		}
		funcs["clientCert"] = func() (string, error) {
			m, err := material()
			if err != nil {
				return "", err
			}
			return m.clientCert, nil
		}
		funcs["clientKey"] = func() (string, error) {
			m, err := material()
			if err != nil {
				return "", err
			}
			return m.clientKey, nil
		}
	})
}

func generateTLS(dir string) (*tlsMaterial, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "dito e2e CA"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, err
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		return nil, err
	}
	leaf := func(serial int64, name string, usage x509.ExtKeyUsage, ips []net.IP) (certPEM, keyPEM []byte, err error) {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, nil, err
		}
		tmpl := &x509.Certificate{
			SerialNumber: big.NewInt(serial),
			Subject:      pkix.Name{CommonName: name},
			NotBefore:    now.Add(-time.Hour),
			NotAfter:     now.Add(24 * time.Hour),
			KeyUsage:     x509.KeyUsageDigitalSignature,
			ExtKeyUsage:  []x509.ExtKeyUsage{usage},
			IPAddresses:  ips,
		}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &key.PublicKey, caKey)
		if err != nil {
			return nil, nil, err
		}
		keyDER, err := x509.MarshalECPrivateKey(key)
		if err != nil {
			return nil, nil, err
		}
		return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
			pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), nil
	}
	serverPEM, serverKeyPEM, err := leaf(2, "127.0.0.1", x509.ExtKeyUsageServerAuth, []net.IP{net.IPv4(127, 0, 0, 1)})
	if err != nil {
		return nil, err
	}
	clientPEM, clientKeyPEM, err := leaf(3, "dito e2e client", x509.ExtKeyUsageClientAuth, nil)
	if err != nil {
		return nil, err
	}
	m := &tlsMaterial{
		caFile:     filepath.Join(dir, "ca.pem"),
		clientCert: filepath.Join(dir, "client.pem"),
		clientKey:  filepath.Join(dir, "client-key.pem"),
		caPool:     x509.NewCertPool(),
	}
	m.caPool.AddCert(caCert)
	files := map[string][]byte{
		m.caFile:     pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}),
		m.clientCert: clientPEM,
		m.clientKey:  clientKeyPEM,
	}
	for path, data := range files {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			return nil, err
		}
	}
	if m.server, err = tls.X509KeyPair(serverPEM, serverKeyPEM); err != nil {
		return nil, err
	}
	if m.client, err = tls.X509KeyPair(clientPEM, clientKeyPEM); err != nil {
		return nil, err
	}
	return m, nil
}

// serverTLSConfig is the TLS configuration of a backend: the server
// certificate of the run and, with clientAuth, a required client certificate.
func serverTLSConfig(clientAuth bool) (*tls.Config, error) {
	m, err := runTLS()
	if err != nil {
		return nil, err
	}
	cfg := &tls.Config{Certificates: []tls.Certificate{m.server}, MinVersion: tls.VersionTLS12}
	if clientAuth {
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
		cfg.ClientCAs = m.caPool
	}
	return cfg, nil
}
