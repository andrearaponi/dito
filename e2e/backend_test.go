package e2e

// Backends of the scenarios: loopback servers that record what they receive,
// and handlers that produce the responses the scenarios need.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// RecordedRequest is a request as received by a backend.
type RecordedRequest struct {
	Method           string
	Path             string
	RawQuery         string
	Host             string
	Proto            string
	Header           http.Header
	ContentLength    int64
	TransferEncoding []string
	BodyLen          int64
	BodySHA256       string
	BodyPrefix       []byte
}

// Backend is a loopback HTTP server owned by a scenario. The proxy
// configuration refers to it as {{backend "name"}}.
type Backend struct {
	Name   string
	URL    string
	server *httptest.Server

	mu       sync.Mutex
	requests []RecordedRequest
}

// BackendOption configures a backend.
type BackendOption func(*backendConfig)

type backendConfig struct{}

// Backend starts a backend named name that serves h and records its requests.
func (s *S) Backend(name string, h http.Handler, opts ...BackendOption) *Backend {
	s.t.Helper()
	if _, dup := s.backends[name]; dup {
		s.Fatalf("backend %q declared twice", name)
	}
	var cfg backendConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	b := &Backend{Name: name}
	b.server = httptest.NewServer(b.record(h))
	b.URL = b.server.URL
	s.Cleanup(func() {
		b.server.CloseClientConnections()
		b.server.Close()
	})
	if s.backends == nil {
		s.backends = map[string]*Backend{}
	}
	s.backends[name] = b
	s.diagnosers = append(s.diagnosers, b.describe)
	return b
}

// Requests returns the requests received so far; a request is recorded when
// its handler returns.
func (b *Backend) Requests() []RecordedRequest {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]RecordedRequest(nil), b.requests...)
}

func (b *Backend) record(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := &recordingBody{rc: r.Body}
		r.Body = body
		h.ServeHTTP(w, r)
		_, _ = io.Copy(io.Discard, body)
		rec := RecordedRequest{
			Method:           r.Method,
			Path:             r.URL.Path,
			RawQuery:         r.URL.RawQuery,
			Host:             r.Host,
			Proto:            r.Proto,
			Header:           r.Header.Clone(),
			ContentLength:    r.ContentLength,
			TransferEncoding: r.TransferEncoding,
			BodyLen:          body.digest.n,
			BodySHA256:       body.digest.sum(),
			BodyPrefix:       body.digest.prefix,
		}
		b.mu.Lock()
		b.requests = append(b.requests, rec)
		b.mu.Unlock()
	})
}

func (b *Backend) describe(out *strings.Builder) {
	reqs := b.Requests()
	fmt.Fprintf(out, "backend %s (%s) received %d request(s):\n", b.Name, b.URL, len(reqs))
	for _, r := range reqs {
		fmt.Fprintf(out, "  %s %s?%s Host=%s body=%dB sha=%.12s\n", r.Method, r.Path, r.RawQuery, r.Host, r.BodyLen, r.BodySHA256)
	}
}

// recordingBody observes a request body while the handler reads it.
type recordingBody struct {
	rc     io.ReadCloser
	digest bodyDigest
}

func (b *recordingBody) Read(p []byte) (int, error) {
	n, err := b.rc.Read(p)
	_, _ = b.digest.Write(p[:n])
	return n, err
}

func (b *recordingBody) Close() error { return b.rc.Close() }

// bodyDigest counts, hashes and keeps the first bytes of a body, so bodies of
// any size are compared without holding them in memory.
type bodyDigest struct {
	n      int64
	h      hash.Hash
	prefix []byte
}

const bodyPrefixLimit = 64 << 10

func (d *bodyDigest) Write(p []byte) (int, error) {
	if d.h == nil {
		d.h = sha256.New()
	}
	d.h.Write(p)
	d.n += int64(len(p))
	if room := bodyPrefixLimit - len(d.prefix); room > 0 {
		d.prefix = append(d.prefix, p[:min(room, len(p))]...)
	}
	return len(p), nil
}

func (d *bodyDigest) sum() string {
	if d.h == nil {
		d.h = sha256.New()
	}
	return hex.EncodeToString(d.h.Sum(nil))
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Deterministic bodies: byte i of a body of any size is patternByte(i), so a
// scenario knows the expected hash without storing the body.

type bodyFraming int

const (
	withContentLength bodyFraming = iota // declares Content-Length
	chunked                              // no Content-Length, flushed in 32 KiB pieces
)

func patternByte(off int64) byte {
	return byte(off*31 + off>>11) //nolint:gosec // keeping only the low byte is the point: a deterministic pattern
}

type patternReader struct{ off, n int64 }

func (p *patternReader) Read(b []byte) (int, error) {
	if p.off >= p.n {
		return 0, io.EOF
	}
	if rem := p.n - p.off; int64(len(b)) > rem {
		b = b[:rem]
	}
	for i := range b {
		b[i] = patternByte(p.off + int64(i))
	}
	p.off += int64(len(b))
	return len(b), nil
}

// deterministicBody serves n deterministic bytes with the given framing.
func deterministicBody(n int64, framing bodyFraming) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		if framing == withContentLength {
			w.Header().Set("Content-Length", strconv.FormatInt(n, 10))
		}
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodHead {
			return
		}
		rc := http.NewResponseController(w)
		buf := make([]byte, 32<<10)
		src := &patternReader{n: n}
		for {
			k, err := src.Read(buf)
			if k > 0 {
				if _, werr := w.Write(buf[:k]); werr != nil {
					return
				}
				if framing == chunked {
					_ = rc.Flush()
				}
			}
			if err != nil {
				return
			}
		}
	})
}

var patternHashes sync.Map // int64 -> string

// deterministicSHA256 returns the SHA-256 of the first n pattern bytes.
func deterministicSHA256(n int64) string {
	if v, ok := patternHashes.Load(n); ok {
		return v.(string) //nolint:forcetypeassert // the map only holds strings
	}
	h := sha256.New()
	_, _ = io.Copy(h, &patternReader{n: n})
	sum := hex.EncodeToString(h.Sum(nil))
	patternHashes.Store(n, sum)
	return sum
}

// serverSentEvents sends "data: one", flushes, waits for release (or for the
// client to go away), then sends "data: two".
func serverSentEvents(release <-chan struct{}) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		rc := http.NewResponseController(w)
		_, _ = io.WriteString(w, "data: one\n\n")
		_ = rc.Flush()
		select {
		case <-release:
		case <-r.Context().Done():
			return
		case <-time.After(10 * time.Second):
		}
		_, _ = io.WriteString(w, "data: two\n\n")
	})
}

// abortAfter declares a body of declared bytes, sends only sent bytes and
// closes the connection.
func abortAfter(declared, sent int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(declared))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(bytes.Repeat([]byte("a"), sent))
		rc := http.NewResponseController(w)
		_ = rc.Flush()
		if conn, _, err := rc.Hijack(); err == nil {
			_ = conn.Close()
		}
	})
}

// websocketEcho upgrades the connection and echoes every message.
func websocketEcho() http.Handler {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		for {
			mt, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if err := conn.WriteMessage(mt, msg); err != nil {
				return
			}
		}
	})
}

// wsURL turns an http(s) URL into the matching ws(s) URL.
func wsURL(u string) string {
	if rest, ok := strings.CutPrefix(u, "https://"); ok {
		return "wss://" + rest
	}
	return "ws://" + strings.TrimPrefix(u, "http://")
}
