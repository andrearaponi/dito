package e2e

// Response bodies: integrity, framing, streaming, informational responses.

import (
	"net/http"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const bodyLocation = `
  - path: "^/body"
    target_url: "{{backend "api"}}/body"
    replace_path: true`

func TestResponseBody_Empty(t *testing.T) {
	Run(t, func(s *S) {
		s.Backend("api", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		p := s.Proxy(bodyLocation)
		r := s.Get(p.URL + "/body")
		require.NoError(s, r.Err)
		assert.Equal(s, http.StatusOK, r.Status)
		assert.Equal(s, int64(0), r.BodyLen)
		assert.NoError(s, r.ReadErr)
	})
}

func TestResponseBody_OneByte(t *testing.T) {
	Run(t, func(s *S) {
		s.Backend("api", text("x"))
		p := s.Proxy(bodyLocation)
		r := s.Get(p.URL + "/body")
		assert.Equal(s, http.StatusOK, r.Status)
		assert.Equal(s, "x", r.Body())
	})
}

// A response without body keeps the status and headers of the backend.
func TestResponseBody_StatusWithEmptyBody(t *testing.T) {
	KnownBug("F-57").Run(t, func(s *S) {
		s.Backend("nocontent", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))
		s.Backend("redirect", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", "/elsewhere")
			w.WriteHeader(http.StatusFound)
		}))
		p := s.Proxy(`
  - path: "^/nocontent$"
    target_url: "{{backend "nocontent"}}/"
    replace_path: true
  - path: "^/redirect$"
    target_url: "{{backend "redirect"}}/"
    replace_path: true`)
		r := s.Get(p.URL + "/nocontent")
		assert.Equal(s, http.StatusNoContent, r.Status)
		r = s.Get(p.URL + "/redirect")
		assert.Equal(s, http.StatusFound, r.Status)
		assert.Equal(s, "/elsewhere", r.Header.Get("Location"))
	})
}

// R-01 and probe P1: a 200 KiB body with Content-Length arrives intact.
func TestResponseBody_R01_P01_LargeWithContentLength(t *testing.T) {
	KnownBug("F-01").Run(t, func(s *S) {
		const size = 200 << 10
		s.Backend("api", deterministicBody(size, withContentLength))
		p := s.Proxy(bodyLocation)
		r := s.Get(p.URL + "/body")
		assert.Equal(s, http.StatusOK, r.Status)
		assert.Equal(s, int64(size), r.ContentLength, "Content-Length of the backend")
		assert.Equal(s, int64(size), r.BodyLen)
		assert.Equal(s, deterministicSHA256(size), r.BodySHA256)
	})
}

// Probe P2: a 200 KiB body without Content-Length arrives intact and chunked.
func TestResponseBody_P02_LargeChunked(t *testing.T) {
	KnownBug("F-01").Run(t, func(s *S) {
		const size = 200 << 10
		s.Backend("api", deterministicBody(size, chunked))
		p := s.Proxy(bodyLocation)
		r := s.Get(p.URL + "/body")
		assert.Equal(s, int64(-1), r.ContentLength, "no Content-Length is invented")
		assert.Equal(s, int64(size), r.BodyLen)
		assert.Equal(s, deterministicSHA256(size), r.BodySHA256)
	})
}

// Probe P5: the answer to HEAD has no body, so its Content-Length is not
// checked against the limit.
func TestResponseBody_P05_HeadOverLimit(t *testing.T) {
	KnownBug("F-55").Run(t, func(s *S) {
		s.Backend("api", deterministicBody(5000, withContentLength))
		p := s.Proxy(bodyLocation + `
    max_response_body_size: 90`)
		r := s.Head(p.URL + "/body")
		assert.Equal(s, http.StatusOK, r.Status)
		assert.Equal(s, int64(5000), r.ContentLength)
	})
}

// A 304 has no body either: its Content-Length (sent by servers such as
// nginx, not by net/http, hence the raw backend) is not checked against the
// limit, and the status reaches the client.
func TestResponseBody_NotModifiedOverLimit(t *testing.T) {
	KnownBug("F-55", "F-57").Run(t, func(s *S) {
		s.RawBackend("api", "HTTP/1.1 304 Not Modified\r\nContent-Length: 5000\r\nETag: \"v1\"\r\nConnection: close\r\n\r\n")
		p := s.Proxy(bodyLocation + `
    max_response_body_size: 90`)
		req := s.NewRequest(http.MethodGet, p.URL+"/body", nil)
		req.Header.Set("If-None-Match", `"v1"`)
		r := s.Do(req)
		assert.Equal(s, http.StatusNotModified, r.Status)
		assert.Equal(s, `"v1"`, r.Header.Get("ETag"))
	})
}

func earlyHintsThenNotFound() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Link", "</style.css>; rel=preload")
		w.WriteHeader(http.StatusEarlyHints)
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	})
}

// Probe P6: an informational response reaches the client.
func TestResponseBody_P06_EarlyHintsForwarded(t *testing.T) {
	KnownBug("F-56").Run(t, func(s *S) {
		s.Backend("api", earlyHintsThenNotFound())
		p := s.Proxy(bodyLocation)
		r := s.Get(p.URL + "/body")
		assert.Equal(s, []int{http.StatusEarlyHints}, r.Informational)
	})
}

// Probe P6: the final status after an informational response is kept.
func TestResponseBody_P06_FinalStatusAfterEarlyHints(t *testing.T) {
	Run(t, func(s *S) {
		s.Backend("api", earlyHintsThenNotFound())
		p := s.Proxy(bodyLocation)
		r := s.Get(p.URL + "/body")
		assert.Equal(s, http.StatusNotFound, r.Status)
		assert.Equal(s, "not found", r.Body())
	})
}

// Probe P7: server-sent events are delivered as they are sent.
func TestResponseBody_P07_ServerSentEvents(t *testing.T) {
	KnownBug("F-01").Run(t, func(s *S) {
		release := make(chan struct{})
		s.Backend("api", serverSentEvents(release))
		p := s.Proxy(bodyLocation)
		st := s.Stream(p.URL + "/body")
		require.NoError(s, st.Err)
		first, ok := st.Next(2 * time.Second)
		close(release)
		require.True(s, ok, "the first event arrives while the backend waits")
		assert.Equal(s, "data: one", first)
		assert.Equal(s, int64(-1), st.ContentLength, "a stream has no Content-Length")
		rest, err := st.Rest(3 * time.Second)
		assert.NoError(s, err)
		assert.Contains(s, rest, "data: two")
	})
}

// Probe P10: proxying a large body does not hold it in memory.
func TestResponseBody_P10_MemoryBounded(t *testing.T) {
	KnownBug("F-01").Run(t, func(s *S) {
		const size = 64 << 20
		s.Backend("api", deterministicBody(size, withContentLength))
		p := s.Proxy(bodyLocation)
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		r := s.Get(p.URL + "/body")
		runtime.ReadMemStats(&after)
		assert.Equal(s, int64(size), r.BodyLen)
		assert.Less(s, after.TotalAlloc-before.TotalAlloc, uint64(16<<20), "allocations while proxying 64 MiB")
	})
}

// Probe P11: a body cut by the backend is an error for the client.
func TestResponseBody_P11_BackendAbortsBody(t *testing.T) {
	Run(t, func(s *S) {
		s.Backend("api", abortAfter(1000, 500))
		p := s.Proxy(bodyLocation)
		r := s.Get(p.URL + "/body")
		assert.True(s, r.Err != nil || r.ReadErr != nil, "the client sees an error, not a complete response")
	})
}
