package e2e

// Request bodies: forwarded intact, and the request size limit.

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const uploadLocation = `
  - path: "^/up"
    target_url: "{{backend "api"}}/up"
    replace_path: true`

func TestRequestBody_JSONIntact(t *testing.T) {
	Run(t, func(s *S) {
		api := s.Backend("api", text("ok"))
		p := s.Proxy(uploadLocation)
		body := `{"data":"` + strings.Repeat("x", 2048) + `"}`
		req := s.NewRequest(http.MethodPost, p.URL+"/up", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r := s.Do(req)
		assert.Equal(s, http.StatusOK, r.Status)
		got := onlyRequest(s, api)
		assert.Equal(s, int64(len(body)), got.BodyLen)
		assert.Equal(s, sha256Hex([]byte(body)), got.BodySHA256)
	})
}

// R-02: a form body reaches the backend.
func TestRequestBody_R02_FormPost(t *testing.T) {
	KnownBug("F-02").Run(t, func(s *S) {
		api := s.Backend("api", text("ok"))
		p := s.Proxy(uploadLocation)
		req := s.NewRequest(http.MethodPost, p.URL+"/up", strings.NewReader("a=1&b=2"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r := s.Do(req)
		assert.Equal(s, http.StatusOK, r.Status)
		reqs := api.Requests()
		require.Len(s, reqs, 1, "the form request reaches the backend")
		assert.Equal(s, "a=1&b=2", string(reqs[0].BodyPrefix))
	})
}

// R-03: the proxy forwards a query it cannot parse; the backend decides.
func TestRequestBody_R03_MalformedQuery(t *testing.T) {
	KnownBug("F-02").Run(t, func(s *S) {
		api := s.Backend("api", text("ok"))
		p := s.Proxy(uploadLocation)
		r := s.Get(p.URL + "/up?a=%zz")
		assert.Equal(s, http.StatusOK, r.Status)
		reqs := api.Requests()
		require.Len(s, reqs, 1, "the request reaches the backend")
		assert.Equal(s, "a=%zz", reqs[0].RawQuery)
	})
}

func TestRequestBody_LimitWithContentLength(t *testing.T) {
	Run(t, func(s *S) {
		api := s.Backend("api", text("ok"))
		p := s.Proxy(uploadLocation)
		const size = 11 << 20
		req := s.NewRequest(http.MethodPost, p.URL+"/up", &patternReader{n: size})
		req.ContentLength = size
		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set("Expect", "100-continue")
		r := s.Do(req)
		assert.Equal(s, http.StatusRequestEntityTooLarge, r.Status)
		e, err := r.ProxyError()
		require.NoError(s, err)
		assert.Equal(s, http.StatusRequestEntityTooLarge, e.Error.Code)
		assert.Empty(s, api.Requests())
	})
}

// R-04: the request size limit also applies without Content-Length.
func TestRequestBody_R04_LimitChunked(t *testing.T) {
	KnownBug("F-03").Run(t, func(s *S) {
		api := s.Backend("api", text("ok"))
		p := s.Proxy(uploadLocation)
		req := s.NewRequest(http.MethodPost, p.URL+"/up", struct{ *patternReader }{&patternReader{n: 11 << 20}})
		req.ContentLength = -1
		req.Header.Set("Content-Type", "application/octet-stream")
		r := s.Do(req)
		assert.Equal(s, http.StatusRequestEntityTooLarge, r.Status)
		assert.Empty(s, api.Requests(), "the body does not reach the backend")
	})
}

func TestRequestBody_LargeUnderLimit(t *testing.T) {
	Run(t, func(s *S) {
		api := s.Backend("api", text("ok"))
		p := s.Proxy(uploadLocation)
		const size = 5 << 20
		req := s.NewRequest(http.MethodPost, p.URL+"/up", &patternReader{n: size})
		req.ContentLength = size
		req.Header.Set("Content-Type", "application/octet-stream")
		r := s.Do(req)
		assert.Equal(s, http.StatusOK, r.Status)
		got := onlyRequest(s, api)
		assert.Equal(s, int64(size), got.BodyLen)
		assert.Equal(s, deterministicSHA256(size), got.BodySHA256)
	})
}
