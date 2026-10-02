package e2e

// Self-tests of the harness: they run scenarios on a fake TB and check how
// Run reports them.

import (
	"net/http"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeTB records what Run reports. Like testing.T, Fatalf and Skipf end the
// goroutine that runs the scenario. It keeps its own registry, so fake
// outcomes never reach the suite summary.
type fakeTB struct {
	registry
	mu       sync.Mutex
	name     string
	errors   []string
	fatals   []string
	skips    []string
	logs     []string
	cleanups []func()
}

func (f *fakeTB) Helper()      {}
func (f *fakeTB) Name() string { return f.name }
func (f *fakeTB) add(list *[]string, format string, args []any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	*list = append(*list, sprintf(format, args...))
}
func (f *fakeTB) Errorf(format string, args ...any) { f.add(&f.errors, format, args) }
func (f *fakeTB) Logf(format string, args ...any)   { f.add(&f.logs, format, args) }
func (f *fakeTB) Fatalf(format string, args ...any) {
	f.add(&f.fatals, format, args)
	runtime.Goexit()
}
func (f *fakeTB) Skipf(format string, args ...any) {
	f.add(&f.skips, format, args)
	runtime.Goexit()
}
func (f *fakeTB) Cleanup(fn func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cleanups = append(f.cleanups, fn)
}
func (f *fakeTB) TempDir() string {
	dir, err := os.MkdirTemp("", "e2e-fake-*")
	if err != nil {
		f.Fatalf("temp dir: %v", err)
	}
	f.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// runFake runs fn on a fake TB in its own goroutine, as testing does, then
// runs the registered cleanups.
func runFake(t *testing.T, fn func(tb TB)) *fakeTB {
	t.Helper()
	f := &fakeTB{name: t.Name() + "/fake"}
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn(f)
	}()
	<-done
	for _, cleanup := range slices.Backward(f.cleanups) {
		cleanup()
	}
	return f
}

func TestHarness_KnownBugReported(t *testing.T) {
	f := runFake(t, func(tb TB) {
		KnownBug("F-01").Run(tb, func(s *S) {
			assert.Equal(s, 204800, 3975, "body length")
		})
	})
	require.Len(t, f.skips, 1, "a failing known bug is reported as a skip")
	assert.Contains(t, f.skips[0], "KNOWN BUG F-01")
	assert.Contains(t, f.skips[0], "body length")
	assert.Empty(t, f.errors)
	assert.Empty(t, f.fatals)
	assert.Equal(t, 1, f.knownBugs)

	// require stops the body like FailNow, and still counts as the known bug.
	f = runFake(t, func(tb TB) {
		KnownBug("F-02").Run(tb, func(s *S) {
			require.True(s, false, "form body forwarded")
			t.Error("the body must stop at the failed require")
		})
	})
	require.Len(t, f.skips, 1)
	assert.Contains(t, f.skips[0], "KNOWN BUG F-02")
}

func TestHarness_KnownBugFixedFails(t *testing.T) {
	f := runFake(t, func(tb TB) {
		KnownBug("F-01", "F-54").Run(tb, func(s *S) {
			assert.Equal(s, 1, 1)
		})
	})
	require.Len(t, f.errors, 1, "a known bug that passes must fail the suite")
	assert.Contains(t, f.errors[0], `remove KnownBug("F-01", "F-54")`)
	assert.Empty(t, f.skips)
	assert.Equal(t, 1, f.failed)
}

func TestHarness_KnownBugInfraErrorFails(t *testing.T) {
	f := runFake(t, func(tb TB) {
		KnownBug("F-01").Run(tb, func(s *S) {
			s.Fatalf("backend %q did not start", "api")
		})
	})
	require.Len(t, f.fatals, 1, "a harness error fails even a known bug")
	assert.Contains(t, f.fatals[0], "e2e harness error")
	assert.Contains(t, f.fatals[0], `backend "api" did not start`)
	assert.Empty(t, f.skips)

	f = runFake(t, func(tb TB) {
		KnownBug("F-01").Run(tb, func(s *S) { panic("boom") })
	})
	require.Len(t, f.fatals, 1, "a panic in the body is a harness error")
	assert.Contains(t, f.fatals[0], "panic: boom")
	assert.Empty(t, f.skips)
}

func TestHarness_SummaryByFinding(t *testing.T) {
	var r registry
	r.record(outcomePassed, nil)
	r.record(outcomePassed, nil)
	r.record(outcomeFailed, nil)
	r.record(outcomeKnownBug, []string{"F-54"})
	r.record(outcomeKnownBug, []string{"F-01", "F-07"})
	r.record(outcomeKnownBug, []string{"F-01"})
	assert.Equal(t, "e2e summary: 2 passed, 1 failed, 3 known bugs (F-01: 2, F-07: 1, F-54: 1)", r.summary())

	var empty registry
	assert.Equal(t, "e2e summary: 0 passed, 0 failed, 0 known bugs", empty.summary())

	// Outcomes of scenarios run on a fake TB stay in the fake's registry.
	before := defaultRegistry.summary()
	f := runFake(t, func(tb TB) { Run(tb, func(s *S) {}) })
	assert.Equal(t, 1, f.passed)
	assert.Equal(t, before, defaultRegistry.summary())
}

func TestHarness_FailureDiagnostics(t *testing.T) {
	f := runFake(t, func(tb TB) {
		Run(tb, func(s *S) {
			s.Logf("GET /api -> 502 Bad Gateway")
			logs := s.newLogBuffer("proxy")
			_, _ = logs.Write([]byte("level=ERROR msg=\"Proxy error\" path=/api\n"))
			assert.Equal(s, 200, 502, "status")
		})
	})
	require.Len(t, f.errors, 1)
	assert.Contains(t, f.errors[0], "status")
	out := strings.Join(f.logs, "\n")
	assert.Contains(t, out, "GET /api -> 502 Bad Gateway", "the exchange is in the diagnostics")
	assert.Contains(t, out, `msg="Proxy error"`, "the proxy logs are in the diagnostics")
}

// Observations, checked directly against backends (no proxy involved).

func TestHarness_Observe64MiBByHash(t *testing.T) {
	Run(t, func(s *S) {
		const size = 64 << 20
		b := s.Backend("big", deterministicBody(size, withContentLength))
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		r := s.Get(b.URL + "/big")
		runtime.ReadMemStats(&after)
		require.NoError(s, r.Err)
		assert.NoError(s, r.ReadErr)
		assert.Equal(s, int64(size), r.BodyLen)
		assert.Equal(s, deterministicSHA256(size), r.BodySHA256)
		assert.Less(s, after.TotalAlloc-before.TotalAlloc, uint64(16<<20), "the body is hashed while streaming, not held in memory")
	})
}

func TestHarness_ObserveFraming(t *testing.T) {
	Run(t, func(s *S) {
		fixed := s.Backend("fixed", deterministicBody(100<<10, withContentLength))
		streamed := s.Backend("streamed", deterministicBody(100<<10, chunked))

		r := s.Get(fixed.URL + "/")
		assert.Equal(s, int64(100<<10), r.ContentLength)
		assert.Empty(s, r.TransferEncoding)

		r = s.Get(streamed.URL + "/")
		assert.Equal(s, int64(-1), r.ContentLength)
		assert.Equal(s, []string{"chunked"}, r.TransferEncoding)
		assert.Equal(s, deterministicSHA256(100<<10), r.BodySHA256)
	})
}

func TestHarness_ObserveEarlyHints(t *testing.T) {
	Run(t, func(s *S) {
		b := s.Backend("hints", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Link", "</style.css>; rel=preload")
			w.WriteHeader(http.StatusEarlyHints)
			w.WriteHeader(http.StatusNotFound)
		}))
		r := s.Get(b.URL + "/")
		assert.Equal(s, []int{http.StatusEarlyHints}, r.Informational)
		assert.Equal(s, http.StatusNotFound, r.Status)
	})
}

func TestHarness_ObserveStreamingTiming(t *testing.T) {
	Run(t, func(s *S) {
		release := make(chan struct{})
		b := s.Backend("sse", serverSentEvents(release))
		st := s.Stream(b.URL + "/events")
		first, ok := st.Next(2 * time.Second)
		require.True(s, ok, "the first event arrives while the backend is still waiting")
		assert.Equal(s, "data: one", first)
		close(release)
		rest, err := st.Rest(2 * time.Second)
		assert.NoError(s, err)
		assert.Contains(s, rest, "data: two")
	})
}

func TestHarness_ObserveAbort(t *testing.T) {
	Run(t, func(s *S) {
		b := s.Backend("abort", abortAfter(1000, 500))
		r := s.Get(b.URL + "/")
		require.NoError(s, r.Err)
		assert.Error(s, r.ReadErr, "a body cut by the peer is an error, not a regular end")
		assert.Equal(s, int64(500), r.BodyLen)

		complete := s.Backend("complete", deterministicBody(1000, withContentLength))
		r = s.Get(complete.URL + "/")
		assert.NoError(s, r.ReadErr)
		assert.Equal(s, int64(1000), r.BodyLen)
	})
}

func TestHarness_BackendRecordsRequests(t *testing.T) {
	Run(t, func(s *S) {
		b := s.Backend("echo", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		req := s.NewRequest(http.MethodPost, b.URL+"/api/items?a=1&b=2", strings.NewReader(`{"name":"x"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Test", "yes")
		s.Do(req)

		got := b.Requests()
		require.Len(s, got, 1)
		assert.Equal(s, http.MethodPost, got[0].Method)
		assert.Equal(s, "/api/items", got[0].Path)
		assert.Equal(s, "a=1&b=2", got[0].RawQuery)
		assert.Equal(s, "yes", got[0].Header.Get("X-Test"))
		assert.Equal(s, int64(12), got[0].BodyLen)
		assert.Equal(s, sha256Hex([]byte(`{"name":"x"}`)), got[0].BodySHA256)
	})
}

func TestHarness_WebSocketEcho(t *testing.T) {
	Run(t, func(s *S) {
		b := s.Backend("ws", websocketEcho())
		conn, _, err := s.DialWebSocket(wsURL(b.URL)+"/ws", nil)
		require.NoError(s, err)
		require.NoError(s, conn.WriteMessage(websocket.TextMessage, []byte("ping")))
		_, msg, err := conn.ReadMessage()
		require.NoError(s, err)
		assert.Equal(s, "ping", string(msg))
	})
}

// In-process proxy.

func TestHarness_InProcessProxy(t *testing.T) {
	Run(t, func(s *S) {
		api := s.Backend("api", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("hello"))
		}))
		p := s.Proxy(`
  - path: "^/hello$"
    target_url: "{{backend "api"}}/hello"
    replace_path: true`)
		r := s.Get(p.URL + "/hello")
		assert.Equal(s, http.StatusOK, r.Status)
		assert.Equal(s, "hello", r.Body())
		reqs := api.Requests()
		require.Len(s, reqs, 1)
		assert.Equal(s, "/hello", reqs[0].Path)
	})
}

func TestHarness_ConfigTemplateUsesBackends(t *testing.T) {
	Run(t, func(s *S) {
		api := s.Backend("api", http.NotFoundHandler())
		p := s.Proxy(`
  - path: "^/x$"
    target_url: "{{backend "api"}}/x"`)
		assert.Contains(s, p.ConfigText, api.URL+"/x")
		assert.NotContains(s, p.ConfigText, "{{")
	})

	// A reference to a backend the scenario did not start is a harness error.
	f := runFake(t, func(tb TB) {
		Run(tb, func(s *S) {
			s.Proxy(`
  - path: "^/x$"
    target_url: "{{backend "missing"}}/x"`)
		})
	})
	require.Len(t, f.fatals, 1)
	assert.Contains(t, f.fatals[0], `unknown backend "missing"`)
}

func TestHarness_ProxyLogsCaptured(t *testing.T) {
	Run(t, func(s *S) {
		s.Backend("api", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		p := s.Proxy(`
  - path: "^/logged$"
    target_url: "{{backend "api"}}/logged"
    replace_path: true`)
		s.Get(p.URL + "/logged")
		s.WaitLog(p, "url=/logged", 5*time.Second)
	})
}

// Binary.

func TestHarness_SameOutcomeInProcessAndBinary(t *testing.T) {
	Run(t, func(s *S) {
		s.Backend("api", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Backend", "yes")
			_, _ = w.Write([]byte("same outcome"))
		}))
		locations := `
  - path: "^/same$"
    target_url: "{{backend "api"}}/same"
    replace_path: true`
		inProcess := s.Get(s.Proxy(locations).URL + "/same")
		binary := s.Get(s.Binary(locations).URL + "/same")
		for _, r := range []*Response{inProcess, binary} {
			require.NoError(s, r.Err)
			assert.NotEmpty(s, r.Header.Get("X-Request-ID"))
		}
		assert.Equal(s, inProcess.Status, binary.Status)
		assert.Equal(s, inProcess.BodySHA256, binary.BodySHA256)
		assert.Equal(s, inProcess.Header.Get("X-Backend"), binary.Header.Get("X-Backend"))
	})
}

func TestHarness_BinaryBuiltInRun(t *testing.T) {
	Run(t, func(s *S) {
		s.Backend("api", http.NotFoundHandler())
		p := s.Binary(`
  - path: "^/x$"
    target_url: "{{backend "api"}}/x"`)
		info, err := os.Stat(p.proc.binary)
		require.NoError(s, err)
		assert.True(s, strings.HasPrefix(p.proc.binary, sharedDir()), "the binary is built in this run's directory")
		assert.False(s, info.ModTime().Before(suiteStart), "the binary is built during this run")
		assert.Positive(s, p.proc.cmd.Process.Pid)
	})
}

func TestHarness_ProxyNotReadyReportsLogs(t *testing.T) {
	start := time.Now()
	f := runFake(t, func(tb TB) {
		Run(tb, func(s *S) {
			s.Backend("api", http.NotFoundHandler())
			// Without the plugins section the binary exits at startup (F-35).
			s.Binary(`
  - path: "^/x$"
    target_url: "{{backend "api"}}/x"`, WithoutPluginsSection())
		})
	})
	require.Len(t, f.fatals, 1)
	assert.Contains(t, f.fatals[0], "not ready")
	assert.Contains(t, f.fatals[0], "public key", "the error shows the proxy logs")
	assert.Less(t, time.Since(start), readyTimeout, "an exited proxy is detected before the timeout")
}
