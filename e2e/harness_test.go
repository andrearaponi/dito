package e2e

// Core of the e2e harness: Run, known bugs, outcome registry, diagnostics.

import (
	"bytes"
	"fmt"
	"net/http"
	"regexp"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
)

// TB is the part of testing.TB that Run needs. *testing.T implements it, and
// so does the fake TB of the harness self-tests.
type TB interface {
	Helper()
	Errorf(format string, args ...any)
	Fatalf(format string, args ...any)
	Skipf(format string, args ...any)
	Logf(format string, args ...any)
	Cleanup(func())
	Name() string
	TempDir() string
}

// Run runs a scenario. Assertions go through s (testify's assert and require
// accept it); any failed assertion fails the scenario, with diagnostics.
func Run(t TB, body func(*S)) {
	t.Helper()
	run(t, nil, body)
}

// Marker marks a scenario whose contract is broken by open findings.
type Marker struct{ findings []string }

// KnownBug marks a scenario as broken by the given findings (IDs such as
// "F-01"). While its assertions fail, the scenario is reported as a skip with
// "KNOWN BUG <ids>"; once they all pass, it fails and asks to remove the
// marker. Harness errors fail it in any case.
func KnownBug(findings ...string) Marker { return Marker{findings: findings} }

// Run runs a scenario marked as a known bug.
func (m Marker) Run(t TB, body func(*S)) {
	t.Helper()
	run(t, m.findings, body)
}

// S is the handle of a running scenario. It implements testify's TestingT:
// assertion failures are recorded, and Run decides the outcome at the end.
type S struct {
	t          TB
	known      []string
	failures   []string
	transcript []string
	logs       []*LogBuffer
	diagnosers []func(*strings.Builder)
	recorded   bool
	backends   map[string]*Backend
	httpClient *http.Client
}

// failNow is the sentinel panic of FailNow: it stops the scenario body.
type failNow struct{}

// Errorf records a failed assertion.
func (s *S) Errorf(format string, args ...any) {
	s.failures = append(s.failures, fmt.Sprintf(format, args...))
}

// FailNow stops the scenario body after a failed require assertion.
func (s *S) FailNow() { panic(failNow{}) }

// Helper satisfies testify's tHelper; the outcome is reported by Run.
func (s *S) Helper() {}

// Logf adds a line to the transcript shown when the scenario fails.
func (s *S) Logf(format string, args ...any) {
	s.transcript = append(s.transcript, fmt.Sprintf(format, args...))
}

// Fatalf reports a harness error (setup failure, timeout, unexpected state):
// it always fails the scenario, even when it is marked as a known bug.
func (s *S) Fatalf(format string, args ...any) {
	s.t.Helper()
	s.record(outcomeFailed)
	s.logDiagnostics()
	s.t.Fatalf("e2e harness error: "+format, args...)
}

// TempDir returns a directory removed when the scenario ends.
func (s *S) TempDir() string { return s.t.TempDir() }

// Cleanup registers a function run when the scenario ends.
func (s *S) Cleanup(fn func()) { s.t.Cleanup(fn) }

func run(t TB, known []string, body func(*S)) {
	t.Helper()
	s := &S{t: t, known: known}
	s.runBody(body)
	s.evaluate()
}

func (s *S) runBody(body func(*S)) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(failNow); ok {
				return
			}
			s.Fatalf("panic: %v\n%s", r, debug.Stack())
		}
	}()
	body(s)
}

func (s *S) evaluate() {
	s.t.Helper()
	failed := len(s.failures) > 0
	switch {
	case len(s.known) == 0 && !failed:
		s.record(outcomePassed)
	case len(s.known) == 0:
		s.record(outcomeFailed)
		s.logDiagnostics()
		s.t.Errorf("scenario failed:\n%s", strings.Join(s.failures, "\n"))
	case failed:
		s.record(outcomeKnownBug)
		s.t.Skipf("KNOWN BUG %s: %s", strings.Join(s.known, ", "), summarizeFailure(s.failures[0]))
	default:
		s.record(outcomeFailed)
		s.t.Errorf("scenario passes: remove KnownBug(%s)", quoteAll(s.known))
	}
}

// logDiagnostics shows what the scenario observed: exchanges, requests seen
// by the backends and proxy logs.
func (s *S) logDiagnostics() {
	s.t.Helper()
	var b strings.Builder
	b.WriteString("--- e2e diagnostics ---\n")
	if len(s.transcript) > 0 {
		b.WriteString("exchanges:\n")
		for _, line := range s.transcript {
			b.WriteString("  " + line + "\n")
		}
	}
	for _, d := range s.diagnosers {
		d(&b)
	}
	for _, l := range s.logs {
		fmt.Fprintf(&b, "logs of %s (last 60 lines):\n%s\n", l.name, tail(l.String(), 60))
	}
	s.t.Logf("%s", b.String())
}

// summarizeFailure condenses a testify failure message to its useful lines.
func summarizeFailure(msg string) string {
	var parts []string
	continuation := false
	for line := range strings.SplitSeq(msg, "\n") {
		line = strings.TrimSpace(line)
		if continuation && line != "" && !hasTestifyLabel(line) {
			// testify prints the value of "Received unexpected error:" on the next line.
			parts[len(parts)-1] += " " + strings.Join(strings.Fields(line), " ")
		}
		continuation = false
		for _, key := range []string{"Error:", "expected", "actual", "Messages:"} {
			if strings.HasPrefix(line, key) {
				parts = append(parts, strings.Join(strings.Fields(line), " "))
				continuation = key == "Error:" && strings.HasSuffix(line, ":")
				break
			}
		}
	}
	out := strings.Join(parts, "; ")
	if out == "" {
		out = strings.Join(strings.Fields(msg), " ")
	}
	if len(out) > 400 {
		out = out[:400] + "..."
	}
	return out
}

// hasTestifyLabel reports whether line starts a new field of a testify message.
func hasTestifyLabel(line string) bool {
	for _, label := range []string{"Error Trace:", "Error:", "Test:", "Messages:", "Diff:", "expected", "actual"} {
		if strings.HasPrefix(line, label) {
			return true
		}
	}
	return false
}

func quoteAll(ids []string) string {
	q := make([]string, len(ids))
	for i, id := range ids {
		q[i] = fmt.Sprintf("%q", id)
	}
	return strings.Join(q, ", ")
}

func tail(text string, lines int) string {
	all := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(all) > lines {
		all = all[len(all)-lines:]
	}
	return strings.Join(all, "\n")
}

func sprintf(format string, args ...any) string { return fmt.Sprintf(format, args...) }

// Outcomes and summary.

type outcome int

const (
	outcomePassed outcome = iota
	outcomeFailed
	outcomeKnownBug
)

// registry counts scenario outcomes for the summary printed by TestMain.
type registry struct {
	mu        sync.Mutex
	passed    int
	failed    int
	knownBugs int
	byFinding map[string]int
}

var defaultRegistry = &registry{}

// recorder lets the fake TB keep its own outcomes out of the suite summary.
type recorder interface {
	record(o outcome, known []string)
}

func (s *S) record(o outcome) {
	if s.recorded {
		return
	}
	s.recorded = true
	var r recorder = defaultRegistry
	if own, ok := s.t.(recorder); ok {
		r = own
	}
	r.record(o, s.known)
}

func (r *registry) record(o outcome, known []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch o {
	case outcomePassed:
		r.passed++
	case outcomeFailed:
		r.failed++
	case outcomeKnownBug:
		r.knownBugs++
		if r.byFinding == nil {
			r.byFinding = map[string]int{}
		}
		for _, id := range known {
			r.byFinding[id]++
		}
	}
}

// summary returns "e2e summary: P passed, F failed, K known bugs (F-01: n, ...)".
func (r *registry) summary() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := fmt.Sprintf("e2e summary: %d passed, %d failed, %d known bugs", r.passed, r.failed, r.knownBugs)
	if len(r.byFinding) == 0 {
		return out
	}
	ids := make([]string, 0, len(r.byFinding))
	for id := range r.byFinding {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = fmt.Sprintf("%s: %d", id, r.byFinding[id])
	}
	return out + " (" + strings.Join(parts, ", ") + ")"
}

// Logs.

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// LogBuffer collects the output of a proxy: the in-process logger or the
// stdout and stderr of the binary. String strips terminal colors.
type LogBuffer struct {
	name string
	mu   sync.Mutex
	buf  bytes.Buffer
}

func (l *LogBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *LogBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return ansiEscape.ReplaceAllString(l.buf.String(), "")
}

// newLogBuffer creates a log buffer included in the scenario diagnostics.
func (s *S) newLogBuffer(name string) *LogBuffer {
	l := &LogBuffer{name: name}
	s.logs = append(s.logs, l)
	return l
}
