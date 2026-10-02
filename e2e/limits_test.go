package e2e

// Response size limits, with the contract documented today in the README
// (413 and a JSON error). S-03 revises this contract and its scenarios.

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// assertLimitError checks the documented limit error for path.
func assertLimitError(s *S, r *Response, limit int, path string) {
	require.NoError(s, r.Err, "a response arrives")
	assert.Equal(s, http.StatusRequestEntityTooLarge, r.Status)
	e, err := r.ProxyError()
	require.NoError(s, err, "the error body is valid JSON: %q", r.Body())
	assert.Equal(s, http.StatusRequestEntityTooLarge, e.Error.Code)
	assert.InDelta(s, float64(limit), e.Error.Details["limit_bytes"], 0)
	assert.Equal(s, path, e.Error.Details["path"])
}

// oneChunkWithoutLength commits the headers without Content-Length, then
// sends n bytes in one chunk.
func oneChunkWithoutLength(n int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = http.NewResponseController(w).Flush()
		_, _ = w.Write(make([]byte, n))
	})
}

// Probe P3: a declared Content-Length over the limit gets the limit error.
func TestLimits_P03_DeclaredOverLimit(t *testing.T) {
	KnownBug("F-54").Run(t, func(s *S) {
		s.Backend("api", deterministicBody(99, withContentLength))
		p := s.Proxy(bodyLocation + `
    max_response_body_size: 90`)
		assertLimitError(s, s.Get(p.URL+"/body"), 90, "/body")
	})
}

// Probe P12: a body without Content-Length whose first chunk exceeds the
// limit gets the limit error.
func TestLimits_P12_FirstChunkOverLimit(t *testing.T) {
	KnownBug("F-54").Run(t, func(s *S) {
		s.Backend("api", oneChunkWithoutLength(99))
		p := s.Proxy(bodyLocation + `
    max_response_body_size: 90`)
		assertLimitError(s, s.Get(p.URL+"/body"), 90, "/body")
	})
}

// Probe P4: when the limit is exceeded after the response started, the
// client never receives a truncated body that looks complete. The broken
// behavior depends on a race inside the proxy (see P12), so the exchange is
// repeated: the scenario fails as soon as one response looks complete.
func TestLimits_P04_OverLimitAfterStart(t *testing.T) {
	KnownBug("F-01").Run(t, func(s *S) {
		s.Backend("api", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(make([]byte, 50))
			_ = http.NewResponseController(w).Flush()
			time.Sleep(20 * time.Millisecond)
			_, _ = w.Write(make([]byte, 49))
		}))
		p := s.Proxy(bodyLocation + `
    max_response_body_size: 90`)
		for attempt := 1; attempt <= 20; attempt++ {
			r := s.Get(p.URL + "/body")
			looksComplete := r.Err == nil && r.ReadErr == nil && r.Status == http.StatusOK
			if looksComplete && r.BodyLen < 99 {
				assert.Fail(s, "a truncated body arrives as a complete response",
					"attempt %d: 200 with %d of 99 bytes and a regular end", attempt, r.BodyLen)
				return
			}
		}
	})
}

func TestLimits_LocationLimitOverridesGlobal(t *testing.T) {
	KnownBug("F-54").Run(t, func(s *S) {
		s.Backend("api", deterministicBody(99, withContentLength))
		p := s.Proxy(`
  - path: "^/global$"
    target_url: "{{backend "api"}}/"
    replace_path: true
  - path: "^/strict$"
    target_url: "{{backend "api"}}/"
    replace_path: true
    max_response_body_size: 90`, WithConfig("response_limits:\n  max_response_body_size: 1000"))
		r := s.Get(p.URL + "/global")
		assert.Equal(s, http.StatusOK, r.Status, "99 bytes are under the global limit")
		assert.Equal(s, int64(99), r.BodyLen)
		assertLimitError(s, s.Get(p.URL+"/strict"), 90, "/strict")
	})
}

func TestLimits_GlobalLimitWithoutLocationLimit(t *testing.T) {
	KnownBug("F-54").Run(t, func(s *S) {
		s.Backend("api", deterministicBody(99, withContentLength))
		p := s.Proxy(bodyLocation, WithConfig("response_limits:\n  max_response_body_size: 90"))
		assertLimitError(s, s.Get(p.URL+"/body"), 90, "/body")
	})
}

// Probe P9: the limit error is valid JSON whatever the request path.
func TestLimits_P09_ErrorJSONForAnyPath(t *testing.T) {
	KnownBug("F-54", "F-07").Run(t, func(s *S) {
		s.Backend("api", deterministicBody(99, withContentLength))
		p := s.Proxy(`
  - path: "^/lim"
    target_url: "{{backend "api"}}/"
    replace_path: true
    max_response_body_size: 90`)
		for escaped, path := range map[string]string{
			"/lim%22quote":      `/lim"quote`,
			"/lim%5Cbackslash":  `/lim\backslash`,
			"/lim%01control":    "/lim\x01control",
			"/lim/%C3%BCnicode": "/lim/ünicode",
		} {
			assertLimitError(s, s.Get(p.URL+escaped), 90, path)
		}
	})
}

func TestLimits_WarningLogged(t *testing.T) {
	Run(t, func(s *S) {
		s.Backend("api", deterministicBody(99, withContentLength))
		p := s.Proxy(bodyLocation + `
    max_response_body_size: 90`)
		s.Get(p.URL + "/body")
		s.WaitLog(p, "exceeds limit", 3*time.Second)
		s.WaitLog(p, "limit_bytes=90", 3*time.Second)
	})
}
