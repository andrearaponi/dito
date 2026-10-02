package e2e

// Client side of the scenarios: requests and what the client observes.

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/textproto"
	neturl "net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// requestTimeout bounds every request of a scenario.
const requestTimeout = 60 * time.Second

// Response is what the client observed. Err is set when no response arrived;
// ReadErr when the body ended with an error instead of a regular end.
type Response struct {
	Status           int
	Proto            string
	Header           http.Header
	Trailer          http.Header
	ContentLength    int64
	TransferEncoding []string
	Informational    []int
	BodyLen          int64
	BodySHA256       string
	BodyPrefix       []byte
	ReadErr          error
	Err              error
}

// Body returns the body, or its first 64 KiB for larger bodies.
func (r *Response) Body() string { return string(r.BodyPrefix) }

func (s *S) client() *http.Client {
	if s.httpClient == nil {
		s.httpClient = &http.Client{
			Transport: &http.Transport{
				DisableCompression: true, // observe the exact bytes on the wire
				DisableKeepAlives:  true, // no state shared between requests
				TLSClientConfig:    s.clientTLSConfig(),
			},
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
	}
	return s.httpClient
}

// NewRequest builds a request bound to the scenario lifetime.
func (s *S) NewRequest(method, url string, body io.Reader) *http.Request {
	s.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	s.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		s.Fatalf("request %s %s: %v", method, url, err)
	}
	return req
}

// Get sends a GET request and observes the response.
func (s *S) Get(url string) *Response { return s.Do(s.NewRequest(http.MethodGet, url, nil)) }

// Head sends a HEAD request and observes the response.
func (s *S) Head(url string) *Response { return s.Do(s.NewRequest(http.MethodHead, url, nil)) }

// Do sends req, reads the whole body while hashing it, and adds the exchange
// to the transcript shown when the scenario fails.
func (s *S) Do(req *http.Request) *Response {
	r := &Response{}
	trace := &httptrace.ClientTrace{
		Got1xxResponse: func(code int, _ textproto.MIMEHeader) error {
			r.Informational = append(r.Informational, code)
			return nil
		},
	}
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
	resp, err := s.client().Do(req)
	if err != nil {
		r.Err = err
		s.Logf("%s %s -> no response: %v", req.Method, req.URL, err)
		return r
	}
	defer func() { _ = resp.Body.Close() }()
	var d bodyDigest
	_, r.ReadErr = io.Copy(&d, resp.Body)
	r.Status = resp.StatusCode
	r.Proto = resp.Proto
	r.Header = resp.Header
	r.Trailer = resp.Trailer
	r.ContentLength = resp.ContentLength
	r.TransferEncoding = resp.TransferEncoding
	r.BodyLen, r.BodySHA256, r.BodyPrefix = d.n, d.sum(), d.prefix
	s.Logf("%s %s -> %d %s CL=%d TE=%v 1xx=%v body=%dB sha=%.12s readErr=%v body[:200]=%q",
		req.Method, req.URL, r.Status, r.Proto, r.ContentLength, r.TransferEncoding, r.Informational,
		r.BodyLen, r.BodySHA256, r.ReadErr, truncate(r.Body(), 200))
	return r
}

func truncate(text string, n int) string {
	if len(text) > n {
		return text[:n]
	}
	return text
}

// Stream is a response read line by line as it arrives.
type Stream struct {
	Err           error
	Status        int
	Header        http.Header
	ContentLength int64

	lines chan string
	done  chan error
	stop  chan struct{}
}

// Stream sends a GET request and returns once the response headers arrive;
// the body is then read line by line with Next and Rest.
func (s *S) Stream(url string) *Stream {
	st := &Stream{lines: make(chan string), done: make(chan error, 1), stop: make(chan struct{})}
	resp, err := s.client().Do(s.NewRequest(http.MethodGet, url, nil))
	if err != nil {
		st.Err = err
		s.Logf("STREAM %s -> no response: %v", url, err)
		close(st.lines)
		st.done <- err
		return st
	}
	st.Status, st.Header, st.ContentLength = resp.StatusCode, resp.Header, resp.ContentLength
	s.Logf("STREAM %s -> %d CL=%d TE=%v", url, resp.StatusCode, resp.ContentLength, resp.TransferEncoding)
	s.Cleanup(func() {
		close(st.stop)
		_ = resp.Body.Close()
	})
	go func() {
		br := bufio.NewReader(resp.Body)
		for {
			line, err := br.ReadString('\n')
			if line != "" {
				select {
				case st.lines <- strings.TrimRight(line, "\r\n"):
				case <-st.stop:
					return
				}
			}
			if err != nil {
				if errors.Is(err, io.EOF) {
					err = nil
				}
				st.done <- err
				close(st.lines)
				return
			}
		}
	}()
	return st
}

// Next returns the next non-empty line, or false if none arrives within
// timeout or the stream has ended.
func (st *Stream) Next(timeout time.Duration) (string, bool) {
	deadline := time.After(timeout)
	for {
		select {
		case line, ok := <-st.lines:
			if !ok {
				return "", false
			}
			if line == "" {
				continue
			}
			return line, true
		case <-deadline:
			return "", false
		}
	}
}

// Rest reads the stream until it ends and returns the remaining lines and the
// read error (nil for a regular end).
func (st *Stream) Rest(timeout time.Duration) (string, error) {
	var b strings.Builder
	deadline := time.After(timeout)
	for {
		select {
		case line, ok := <-st.lines:
			if !ok {
				return b.String(), <-st.done
			}
			b.WriteString(line + "\n")
		case <-deadline:
			return b.String(), errors.New("the stream did not end in time")
		}
	}
}

// DialWebSocket opens a WebSocket connection, closed when the scenario ends.
// It returns the status of the handshake response (0 if none arrived).
func (s *S) DialWebSocket(url string, header http.Header) (*websocket.Conn, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	d := websocket.Dialer{HandshakeTimeout: 10 * time.Second, TLSClientConfig: s.clientTLSConfig()}
	conn, resp, err := d.DialContext(ctx, url, header)
	status := 0
	if resp != nil {
		status = resp.StatusCode
		_ = resp.Body.Close()
	}
	s.Logf("WEBSOCKET %s -> status=%d err=%v", url, status, err)
	if conn != nil {
		s.Cleanup(func() { _ = conn.Close() })
	}
	return conn, status, err
}

// clientTLSConfig makes the scenario clients trust the run CA, so they can
// also talk to TLS backends directly.
func (s *S) clientTLSConfig() *tls.Config {
	m, err := runTLS()
	if err != nil {
		s.Fatalf("TLS material: %v", err)
	}
	return &tls.Config{RootCAs: m.caPool, MinVersion: tls.VersionTLS12}
}

// proxyError is the JSON envelope of the errors generated by the proxy.
type proxyError struct {
	Error struct {
		Code    int            `json:"code"`
		Message string         `json:"message"`
		Details map[string]any `json:"details"`
	} `json:"error"`
	RequestID string `json:"request_id"`
	Timestamp int64  `json:"timestamp"`
}

// ProxyError decodes the body as the proxy error envelope.
func (r *Response) ProxyError() (proxyError, error) {
	var e proxyError
	err := json.Unmarshal(r.BodyPrefix, &e)
	return e, err
}

// DoPartialUpload sends a POST declaring contentLength but writing only sent
// bytes of body, then stops writing and reads the response. It observes how
// the proxy answers an oversized upload without racing with the upload: a
// client still writing when the proxy closes may see a broken pipe instead.
func (s *S) DoPartialUpload(rawURL string, contentLength int64, sent int) *Response {
	s.t.Helper()
	r := &Response{}
	u, err := neturl.Parse(rawURL)
	if err != nil {
		s.Fatalf("url %s: %v", rawURL, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", u.Host)
	if err != nil {
		r.Err = err
		s.Logf("PARTIAL UPLOAD %s -> no connection: %v", rawURL, err)
		return r
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	head := fmt.Sprintf("POST %s HTTP/1.1\r\nHost: %s\r\nContent-Type: application/octet-stream\r\nContent-Length: %d\r\n\r\n",
		u.RequestURI(), u.Host, contentLength)
	if _, err := io.WriteString(conn, head); err != nil {
		r.Err = err
		return r
	}
	_, _ = conn.Write(make([]byte, sent))
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		r.Err = err
		s.Logf("PARTIAL UPLOAD %s (%d of %d bytes) -> no response: %v", rawURL, sent, contentLength, err)
		return r
	}
	defer func() { _ = resp.Body.Close() }()
	var dg bodyDigest
	_, r.ReadErr = io.Copy(&dg, resp.Body)
	r.Status, r.Proto, r.Header, r.ContentLength = resp.StatusCode, resp.Proto, resp.Header, resp.ContentLength
	r.BodyLen, r.BodySHA256, r.BodyPrefix = dg.n, dg.sum(), dg.prefix
	s.Logf("PARTIAL UPLOAD %s (%d of %d bytes) -> %d body[:200]=%q", rawURL, sent, contentLength, r.Status, truncate(r.Body(), 200))
	return r
}
