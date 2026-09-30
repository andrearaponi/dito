package e2e

// Hot reload of the configuration file, on the compiled proxy.

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// reloadTimeout covers two cycles of the watcher (2 s polling plus 1 s wait).
const reloadTimeout = 7 * time.Second

func routeTo(backend string) string {
	return fmt.Sprintf(`
  - path: "^/t$"
    target_url: "{{backend %q}}/"
    replace_path: true`, backend)
}

func TestReload_NewLocationServed(t *testing.T) {
	Run(t, func(s *S) {
		s.Backend("a", text("a"))
		s.Backend("b", text("b"))
		p := s.Binary(routeTo("a"), WithHotReload())
		s.Reload(p, routeTo("a")+`
  - path: "^/new$"
    target_url: "{{backend "b"}}/"
    replace_path: true`)
		served := s.Eventually(func() bool { return s.Get(p.URL+"/new").Body() == "b" }, reloadTimeout)
		assert.True(s, served, "the new location is served after the reload")
		assert.Equal(s, "a", s.Get(p.URL+"/t").Body(), "the existing location still works")
	})
}

// R-10: going back to a previous configuration is a change too.
func TestReload_R10_RevertDetected(t *testing.T) {
	KnownBug("F-24").Run(t, func(s *S) {
		s.Backend("a", text("a"))
		s.Backend("b", text("b"))
		p := s.Binary(routeTo("a"), WithHotReload())
		s.Reload(p, routeTo("b"))
		if !s.Eventually(func() bool { return s.Get(p.URL+"/t").Body() == "b" }, reloadTimeout) {
			s.Fatalf("the first reload (a to b) was not applied")
		}
		s.Reload(p, routeTo("a"))
		reverted := s.Eventually(func() bool { return s.Get(p.URL+"/t").Body() == "a" }, reloadTimeout)
		assert.True(s, reverted, "after going back to the first configuration, requests reach backend a again")
	})
}

// R-09: requests served while the configuration is reloaded do not race
// with the reload. The proxy runs with the race detector: a race is reported
// in its output (in-process it would fail the whole test binary).
func TestReload_R09_ConcurrentRequestsRace(t *testing.T) {
	KnownBug("F-23").Run(t, func(s *S) {
		api := s.Backend("api", text("ok"))
		locations := func(mark int) string {
			return fmt.Sprintf(`
  - path: "^/t$"
    target_url: "{{backend "api"}}/"
    replace_path: true
    additional_headers:
      X-Reload: "%d"`, mark)
		}
		p := s.Binary(locations(0), WithHotReload(), WithRaceDetector())

		stop := make(chan struct{})
		var clients sync.WaitGroup
		for range 4 {
			clients.Go(func() {
				client := &http.Client{Timeout: 2 * time.Second}
				for {
					select {
					case <-stop:
						return
					default:
					}
					req := s.NewRequest(http.MethodGet, p.URL+"/t", nil)
					if resp, err := client.Do(req); err == nil {
						_ = resp.Body.Close()
					}
				}
			})
		}
		const race = "WARNING: DATA RACE"
		for mark := 1; mark <= 3 && !strings.Contains(p.Logs.String(), race); mark++ {
			s.Reload(p, locations(mark))
			want := fmt.Sprint(mark)
			applied := s.Eventually(func() bool {
				reqs := api.Requests()
				return len(reqs) > 0 && reqs[len(reqs)-1].Header.Get("X-Reload") == want
			}, reloadTimeout)
			if !applied {
				close(stop)
				clients.Wait()
				s.Fatalf("reload %d was not applied", mark)
			}
		}
		close(stop)
		clients.Wait()
		if logs := p.Logs.String(); strings.Contains(logs, race) {
			var frames []string
			for line := range strings.SplitSeq(logs[strings.Index(logs, race):], "\n") {
				if line = strings.TrimSpace(line); strings.HasPrefix(line, "dito/") && !slices.Contains(frames, line) {
					frames = append(frames, line)
				}
				if len(frames) == 4 {
					break
				}
			}
			assert.Fail(s, "the race detector reported a data race", "in %s", strings.Join(frames, " | "))
		}
	})
}
