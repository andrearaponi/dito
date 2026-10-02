package e2e

// Headers: what the proxy adds, rewrites and removes.

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const headersLocation = `
  - path: "^/h$"
    target_url: "{{backend "api"}}/h"
    replace_path: true`

// onlyRequest returns the single request received by b.
func onlyRequest(s *S, b *Backend) RecordedRequest {
	reqs := b.Requests()
	require.Len(s, reqs, 1)
	return reqs[0]
}

// R-06: X-Forwarded-* describe the client request once.
func TestHeaders_R06_XForwarded(t *testing.T) {
	KnownBug("F-05").Run(t, func(s *S) {
		api := s.Backend("api", text("ok"))
		p := s.Proxy(headersLocation)
		req := s.NewRequest(http.MethodGet, p.URL+"/h", nil)
		req.Host = "public.example.com"
		s.Do(req)
		got := onlyRequest(s, api)
		assert.Equal(s, "127.0.0.1", got.Header.Get("X-Forwarded-For"))
		assert.Equal(s, "public.example.com", got.Header.Get("X-Forwarded-Host"))
		assert.Equal(s, "http", got.Header.Get("X-Forwarded-Proto"))
	})
}

func TestHeaders_HostRewritten(t *testing.T) {
	Run(t, func(s *S) {
		api := s.Backend("api", text("ok"))
		p := s.Proxy(headersLocation)
		s.Get(p.URL + "/h")
		assert.Equal(s, strings.TrimPrefix(api.URL, "http://"), onlyRequest(s, api).Host)
	})
}

func TestHeaders_RequestIDGenerated(t *testing.T) {
	Run(t, func(s *S) {
		api := s.Backend("api", text("ok"))
		p := s.Proxy(headersLocation)
		r := s.Get(p.URL + "/h")
		id := r.Header.Get("X-Request-ID")
		assert.NotEmpty(s, id)
		assert.Equal(s, id, onlyRequest(s, api).Header.Get("X-Request-ID"))
	})
}

func TestHeaders_RequestIDPropagated(t *testing.T) {
	Run(t, func(s *S) {
		api := s.Backend("api", text("ok"))
		p := s.Proxy(headersLocation)
		req := s.NewRequest(http.MethodGet, p.URL+"/h", nil)
		req.Header.Set("X-Request-ID", "e2e-request-123")
		r := s.Do(req)
		assert.Equal(s, "e2e-request-123", r.Header.Get("X-Request-ID"))
		assert.Equal(s, "e2e-request-123", onlyRequest(s, api).Header.Get("X-Request-ID"))
	})
}

func TestHeaders_HopByHopRemoved(t *testing.T) {
	Run(t, func(s *S) {
		api := s.Backend("api", text("ok"))
		p := s.Proxy(headersLocation)
		req := s.NewRequest(http.MethodGet, p.URL+"/h", nil)
		req.Header.Set("Proxy-Authorization", "Basic c2VjcmV0")
		req.Header.Set("Proxy-Connection", "keep-alive")
		s.Do(req)
		got := onlyRequest(s, api)
		assert.Empty(s, got.Header.Get("Proxy-Authorization"))
		assert.Empty(s, got.Header.Get("Proxy-Connection"))
	})
}

func TestHeaders_AdditionalHeaders(t *testing.T) {
	Run(t, func(s *S) {
		api := s.Backend("api", text("ok"))
		p := s.Proxy(headersLocation + `
    additional_headers:
      X-Extra: "added"`)
		s.Get(p.URL + "/h")
		assert.Equal(s, "added", onlyRequest(s, api).Header.Get("X-Extra"))
	})
}

func TestHeaders_ExcludedHeaders(t *testing.T) {
	Run(t, func(s *S) {
		api := s.Backend("api", text("ok"))
		p := s.Proxy(headersLocation + `
    excluded_headers: ["X-Secret"]`)
		req := s.NewRequest(http.MethodGet, p.URL+"/h", nil)
		req.Header.Set("X-Secret", "s3cret")
		s.Do(req)
		assert.Empty(s, onlyRequest(s, api).Header.Get("X-Secret"))
	})
}

// Header names are case-insensitive, also in excluded_headers.
func TestHeaders_ExcludedHeadersCaseInsensitive(t *testing.T) {
	KnownBug("F-12").Run(t, func(s *S) {
		api := s.Backend("api", text("ok"))
		p := s.Proxy(headersLocation + `
    excluded_headers: ["x-forwarded-for"]`)
		s.Get(p.URL + "/h")
		assert.Empty(s, onlyRequest(s, api).Header.Values("X-Forwarded-For"))
	})
}

func TestHeaders_SecurityHeaders(t *testing.T) {
	Run(t, func(s *S) {
		s.Backend("api", text("ok"))
		p := s.Proxy(headersLocation)
		r := s.Get(p.URL + "/h")
		assert.Equal(s, "nosniff", r.Header.Get("X-Content-Type-Options"))
		assert.Equal(s, "DENY", r.Header.Get("X-Frame-Options"))
		assert.Equal(s, "strict-origin-when-cross-origin", r.Header.Get("Referrer-Policy"))
	})
}
