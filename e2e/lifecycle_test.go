package e2e

// Lifecycle of the compiled proxy: startup, graceful shutdown.

import (
	"net/http"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// R-13: Dito starts without a plugins section; plugins are optional.
func TestLifecycle_R13_StartWithoutPlugins(t *testing.T) {
	KnownBug("F-35").Run(t, func(s *S) {
		s.Backend("api", text("ok"))
		p, err := s.TryBinary(`
  - path: "^/x$"
    target_url: "{{backend "api"}}/"
    replace_path: true`, WithoutPluginsSection())
		require.NoError(s, err, "the proxy starts without a plugins section")
		assert.Equal(s, "ok", s.Get(p.URL+"/x").Body())
	})
}

// On SIGTERM a request in progress completes, then the process exits with 0.
func TestLifecycle_GracefulShutdown(t *testing.T) {
	Run(t, func(s *S) {
		started := make(chan struct{})
		s.Backend("slow", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			close(started)
			select {
			case <-r.Context().Done():
				return
			case <-time.After(time.Second):
			}
			_, _ = w.Write([]byte("done"))
		}))
		p := s.Binary(`
  - path: "^/slow$"
    target_url: "{{backend "slow"}}/"
    replace_path: true`)
		result := make(chan *Response, 1)
		go func() { result <- s.Get(p.URL + "/slow") }()
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			s.Fatalf("the request did not reach the backend")
		}
		require.NoError(s, p.Signal(syscall.SIGTERM))
		r := <-result
		assert.Equal(s, http.StatusOK, r.Status, "the request in progress completes")
		assert.Equal(s, "done", r.Body())
		code, exited := p.WaitExit(10 * time.Second)
		assert.True(s, exited, "the proxy exits after the shutdown")
		assert.Equal(s, 0, code)
	})
}

// R-14: a 200 KiB body with Content-Length arrives intact through the binary.
func TestLifecycle_R14_LargeResponseThroughBinary(t *testing.T) {
	KnownBug("F-01").Run(t, func(s *S) {
		const size = 200 << 10
		s.Backend("big", deterministicBody(size, withContentLength))
		p := s.Binary(`
  - path: "^/big$"
    target_url: "{{backend "big"}}/big"
    replace_path: true`)
		r := s.Get(p.URL + "/big")
		assert.Equal(s, int64(size), r.BodyLen)
		assert.Equal(s, deterministicSHA256(size), r.BodySHA256)
	})
}
