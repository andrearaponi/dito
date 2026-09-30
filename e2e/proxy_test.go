package e2e

// Proxy configuration and the in-process proxy.

import (
	"bytes"
	"context"
	"dito/app"
	"dito/config"
	"dito/handlers"
	"dito/metrics"
	"dito/plugin"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/template"
	"time"
)

// Proxy is a proxy started by a scenario, in-process (S.Proxy) or as the
// compiled binary (S.Binary).
type Proxy struct {
	Name       string
	URL        string
	ConfigText string
	ConfigPath string
	Logs       *LogBuffer

	proc *process // binary only
}

// ProxyOption adjusts the configuration of a proxy.
type ProxyOption func(*proxyOptions)

type proxyOptions struct {
	plugins    []plugin.Plugin
	metrics    bool
	logEnabled bool
	logVerbose bool
	logLevel   string
	hotReload  bool
	transport  string
	extra      string
	raw        bool
	noPlugins  bool // binary only: no plugins section
	race       bool // binary only: built with the race detector
	signed     bool // binary only: load the signed example plugin
	tamper     bool // binary only: alter the plugin after signing it
}

const defaultTransport = `idle_conn_timeout: 5s
dial_timeout: 2s
response_header_timeout: 10s`

func newProxyOptions(opts []ProxyOption) *proxyOptions {
	o := &proxyOptions{logEnabled: true, logLevel: "info", transport: defaultTransport}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// WithPlugins passes plugins to the in-process proxy (fakes implementing
// plugin.Plugin); locations use them by name in "middlewares".
func WithPlugins(p ...plugin.Plugin) ProxyOption {
	return func(o *proxyOptions) { o.plugins = append(o.plugins, p...) }
}

// WithMetrics enables the metrics endpoint on /metrics.
func WithMetrics() ProxyOption { return func(o *proxyOptions) { o.metrics = true } }

// WithLogging sets the logging section.
func WithLogging(enabled, verbose bool, level string) ProxyOption {
	return func(o *proxyOptions) { o.logEnabled, o.logVerbose, o.logLevel = enabled, verbose, level }
}

// WithHotReload enables hot reload (binary only).
func WithHotReload() ProxyOption { return func(o *proxyOptions) { o.hotReload = true } }

// WithTransport replaces the global transport.http settings (YAML lines).
func WithTransport(yaml string) ProxyOption { return func(o *proxyOptions) { o.transport = yaml } }

// WithConfig appends top-level YAML, such as response_limits.
func WithConfig(yaml string) ProxyOption {
	return func(o *proxyOptions) { o.extra += strings.Trim(yaml, "\n") + "\n" }
}

// WithRawConfig makes the locations argument the whole configuration.
func WithRawConfig() ProxyOption { return func(o *proxyOptions) { o.raw = true } }

// configText assembles the configuration; backend references are resolved
// later by renderConfig.
func (o *proxyOptions) configText(locations, port, pluginsSection string) string {
	if o.raw {
		return locations
	}
	var b strings.Builder
	fmt.Fprintf(&b, "port: %q\n", port)
	fmt.Fprintf(&b, "hot_reload: %t\n", o.hotReload)
	fmt.Fprintf(&b, "logging:\n  enabled: %t\n  verbose: %t\n  level: %q\n", o.logEnabled, o.logVerbose, o.logLevel)
	fmt.Fprintf(&b, "metrics:\n  enabled: %t\n  path: \"/metrics\"\n", o.metrics)
	b.WriteString("transport:\n  http:\n" + indent(o.transport, "    ") + "\n")
	b.WriteString(pluginsSection)
	b.WriteString(o.extra)
	b.WriteString("locations:\n" + strings.Trim(locations, "\n") + "\n")
	return b.String()
}

func indent(text, prefix string) string {
	lines := strings.Split(strings.Trim(text, "\n"), "\n")
	for i, l := range lines {
		lines[i] = prefix + strings.TrimLeft(l, " \t")
	}
	return strings.Join(lines, "\n")
}

// templateFuncExtensions lets other files add configuration placeholders
// (TLS material in certs_test.go).
var templateFuncExtensions []func(s *S, funcs template.FuncMap)

// renderConfig resolves {{backend "name"}}, {{ws "name"}} and {{port}}.
func (s *S) renderConfig(text, port string) string {
	s.t.Helper()
	funcs := template.FuncMap{
		"backend": func(name string) (string, error) {
			b, ok := s.backends[name]
			if !ok {
				return "", fmt.Errorf("unknown backend %q", name)
			}
			return b.URL, nil
		},
		"ws": func(name string) (string, error) {
			b, ok := s.backends[name]
			if !ok {
				return "", fmt.Errorf("unknown backend %q", name)
			}
			return wsURL(b.URL), nil
		},
		"port": func() string { return port },
	}
	for _, ext := range templateFuncExtensions {
		ext(s, funcs)
	}
	tmpl, err := template.New("config").Funcs(funcs).Parse(text)
	if err != nil {
		s.Fatalf("config template: %v\n%s", err, text)
	}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, nil); err != nil {
		s.Fatalf("config template: %v\n%s", err, text)
	}
	return out.String()
}

func (s *S) writeConfig(dir, text string) string {
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		s.Fatalf("write config: %v", err)
	}
	return path
}

var metricsOnce sync.Once

// Proxy starts the proxy in-process: the configuration is loaded like the
// binary does, and the handler chain is handlers.NewHandler. In-process
// proxies share global state (current configuration, metrics registry), so
// scenarios using them must not run in parallel.
func (s *S) Proxy(locations string, opts ...ProxyOption) *Proxy {
	s.t.Helper()
	o := newProxyOptions(opts)
	text := s.renderConfig(o.configText(locations, "0", ""), "0")
	path := s.writeConfig(s.TempDir(), text)
	cfg, err := config.LoadConfiguration(path)
	if err != nil {
		s.Fatalf("config not loaded: %v\n%s", err, text)
	}
	config.UpdateConfig(cfg)
	metricsOnce.Do(metrics.InitMetrics) // as cmd/main.go does at startup

	logs := s.newLogBuffer("in-process proxy")
	logger := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: parseLevel(cfg.Logging.Level)}))
	dito := app.NewDito(&cfg.Transport.HTTP, logger)
	for _, p := range o.plugins {
		if err := p.Init(context.Background(), map[string]any{}, dito); err != nil {
			s.Fatalf("plugin %s init: %v", p.Name(), err)
		}
	}
	server := httptest.NewServer(handlers.NewHandler(dito, o.plugins))
	s.Cleanup(server.Close)
	return &Proxy{Name: "in-process proxy", URL: server.URL, ConfigText: text, ConfigPath: path, Logs: logs}
}

func parseLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// WaitLog waits until the proxy logs contain text. Logs are asynchronous, so
// a missing line becomes a failed assertion only after timeout.
func (s *S) WaitLog(p *Proxy, text string, timeout time.Duration) bool {
	if s.Eventually(func() bool { return strings.Contains(p.Logs.String(), text) }, timeout) {
		return true
	}
	s.Errorf("the logs of the %s do not contain %q after %v", p.Name, text, timeout)
	return false
}

// NoLog checks that text does not appear in the proxy logs within settle. A
// late line can make it pass wrongly, never fail wrongly.
func (s *S) NoLog(p *Proxy, text string, settle time.Duration) bool {
	if s.Eventually(func() bool { return strings.Contains(p.Logs.String(), text) }, settle) {
		s.Errorf("the logs of the %s contain %q", p.Name, text)
		return false
	}
	return true
}

// Eventually polls cond until it holds or timeout expires.
func (s *S) Eventually(cond func() bool, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if cond() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
}
