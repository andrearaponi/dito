package e2e

// Metrics endpoint and counters. The Prometheus registry is global to the
// process, so counters are compared as differences.

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sample is one line of the text exposition format.
type sample struct {
	name   string
	labels map[string]string
	value  float64
}

var (
	sampleLine = regexp.MustCompile(`^([a-zA-Z_:][a-zA-Z0-9_:]*)(?:\{(.*)\})? ([^ ]+)`)
	labelPair  = regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_]*)="((?:[^"\\]|\\.)*)"`)
)

// scrape reads the metrics endpoint of p.
func scrape(s *S, p *Proxy) []sample {
	r := s.Get(p.URL + "/metrics")
	require.Equal(s, http.StatusOK, r.Status, "metrics endpoint")
	var out []sample
	for line := range strings.SplitSeq(r.Body(), "\n") {
		m := sampleLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		v, err := strconv.ParseFloat(m[3], 64)
		if err != nil {
			continue
		}
		labels := map[string]string{}
		for _, lp := range labelPair.FindAllStringSubmatch(m[2], -1) {
			labels[lp[1]] = lp[2]
		}
		out = append(out, sample{name: m[1], labels: labels, value: v})
	}
	return out
}

// sum adds the samples of name whose labels include want.
func sum(samples []sample, name string, want map[string]string) float64 {
	total := 0.0
	for _, sm := range samples {
		if sm.name != name {
			continue
		}
		match := true
		for k, v := range want {
			if sm.labels[k] != v {
				match = false
				break
			}
		}
		if match {
			total += sm.value
		}
	}
	return total
}

const metricsLocation = `
  - path: "^/echo$"
    target_url: "{{backend "api"}}/echo"
    replace_path: true`

func TestMetrics_EndpointExposed(t *testing.T) {
	Run(t, func(s *S) {
		s.Backend("api", text("ok"))
		p := s.Proxy(metricsLocation, WithMetrics())
		s.Get(p.URL + "/echo")
		r := s.Get(p.URL + "/metrics")
		assert.Equal(s, http.StatusOK, r.Status)
		assert.Contains(s, r.Body(), "http_requests_total")
		assert.Contains(s, r.Body(), "go_goroutines")
	})
}

// R-08: each request is counted once, with its numeric status code.
func TestMetrics_R08_CountersPerRequest(t *testing.T) {
	KnownBug("F-28", "F-29").Run(t, func(s *S) {
		s.Backend("api", text("ok"))
		p := s.Proxy(metricsLocation, WithMetrics())
		before := scrape(s, p)
		s.Get(p.URL + "/echo")
		s.Do(s.NewRequest(http.MethodPost, p.URL+"/echo", strings.NewReader("x")))
		after := scrape(s, p)
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			labels := map[string]string{"method": method, "normalized_path": "/echo"}
			assert.InDelta(s, 1, sum(after, "http_requests_total", labels)-sum(before, "http_requests_total", labels), 0,
				"%s requests counted", method)
		}
		labels := map[string]string{"method": http.MethodGet, "normalized_path": "/echo", "status_code": "200"}
		assert.InDelta(s, 1, sum(after, "http_requests_total", labels)-sum(before, "http_requests_total", labels), 0,
			"the status_code label is the numeric status")
	})
}
