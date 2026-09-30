package e2e

// Access logging and its configuration.

import (
	"testing"
	"time"
)

const loggingLocation = `
  - path: "^/logged"
    target_url: "{{backend "api"}}/"
    replace_path: true`

func TestLogging_CompactAccessLog(t *testing.T) {
	Run(t, func(s *S) {
		s.Backend("api", text("ok"))
		p := s.Proxy(loggingLocation, WithLogging(true, false, "info"))
		s.Get(p.URL + "/logged/compact")
		s.WaitLog(p, "HTTP request processed", 3*time.Second)
		s.WaitLog(p, "url=/logged/compact", 3*time.Second)
	})
}

// R-11: with logging disabled, requests are not logged.
func TestLogging_R11_Disabled(t *testing.T) {
	KnownBug("F-32").Run(t, func(s *S) {
		s.Backend("api", text("ok"))
		p := s.Proxy(loggingLocation, WithLogging(false, false, "info"))
		s.Get(p.URL + "/logged/quiet")
		s.NoLog(p, "url=/logged/quiet", 500*time.Millisecond)
	})
}

// R-12: with verbose logging at level info, each request is still logged.
func TestLogging_R12_Verbose(t *testing.T) {
	KnownBug("F-32").Run(t, func(s *S) {
		s.Backend("api", text("ok"))
		p := s.Proxy(loggingLocation, WithLogging(true, true, "info"))
		s.Get(p.URL + "/logged/verbose")
		s.WaitLog(p, "/logged/verbose", 2*time.Second)
	})
}
