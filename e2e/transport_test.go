package e2e

// Transport towards the backends: TLS, mutual TLS, per-location settings.

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTransport_HTTPSBackendCustomCA(t *testing.T) {
	Run(t, func(s *S) {
		s.Backend("secure", text("secure ok"), WithTLS())
		p := s.Proxy(`
  - path: "^/secure$"
    target_url: "{{backend "secure"}}/"
    replace_path: true
    transport:
      http:
        ca_file: "{{ca "secure"}}"`)
		r := s.Get(p.URL + "/secure")
		assert.Equal(s, http.StatusOK, r.Status)
		assert.Equal(s, "secure ok", r.Body())
	})
}

func TestTransport_MutualTLS(t *testing.T) {
	Run(t, func(s *S) {
		mtls := s.Backend("mtls", text("mtls ok"), WithClientAuth())
		p := s.Proxy(`
  - path: "^/with-cert$"
    target_url: "{{backend "mtls"}}/"
    replace_path: true
    transport:
      http:
        ca_file: "{{ca "mtls"}}"
        cert_file: "{{clientCert}}"
        key_file: "{{clientKey}}"
  - path: "^/without-cert$"
    target_url: "{{backend "mtls"}}/"
    replace_path: true
    transport:
      http:
        ca_file: "{{ca "mtls"}}"`)
		r := s.Get(p.URL + "/with-cert")
		assert.Equal(s, http.StatusOK, r.Status)
		assert.Equal(s, "mtls ok", r.Body())
		r = s.Get(p.URL + "/without-cert")
		assert.Equal(s, http.StatusBadGateway, r.Status, "the backend refuses a client without certificate")
		assert.Len(s, mtls.Requests(), 1)
	})
}

func TestTransport_LocationOverridesGlobal(t *testing.T) {
	Run(t, func(s *S) {
		s.Backend("slow", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(time.Second):
			}
			_, _ = w.Write([]byte("slow ok"))
		}))
		p := s.Proxy(`
  - path: "^/impatient$"
    target_url: "{{backend "slow"}}/"
    replace_path: true
    transport:
      http:
        response_header_timeout: 300ms
  - path: "^/patient$"
    target_url: "{{backend "slow"}}/"
    replace_path: true`, WithTransport("dial_timeout: 2s\nresponse_header_timeout: 5s"))
		assert.Equal(s, http.StatusGatewayTimeout, s.Get(p.URL+"/impatient").Status)
		r := s.Get(p.URL + "/patient")
		assert.Equal(s, http.StatusOK, r.Status)
		assert.Equal(s, "slow ok", r.Body())
	})
}
