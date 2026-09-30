package e2e

// The compiled proxy: builds shared by the test run, processes, readiness.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// suiteStart is when the test process started: binaries built by this run
// are newer.
var suiteStart = time.Now()

const (
	readyTimeout = 30 * time.Second
	stopTimeout  = 10 * time.Second
	buildTimeout = 10 * time.Minute
)

// WithoutPluginsSection omits the plugins section of the configuration.
func WithoutPluginsSection() ProxyOption { return func(o *proxyOptions) { o.noPlugins = true } }

// WithRaceDetector runs the proxy built with the race detector.
func WithRaceDetector() ProxyOption { return func(o *proxyOptions) { o.race = true } }

// WithSignedPlugin loads the example plugin, signed with the keys of the run.
func WithSignedPlugin() ProxyOption { return func(o *proxyOptions) { o.signed = true } }

// WithTamperedPlugin is WithSignedPlugin with the plugin altered after signing.
func WithTamperedPlugin() ProxyOption {
	return func(o *proxyOptions) { o.signed, o.tamper = true, true }
}

// Binary starts the compiled proxy and fails the scenario if it does not
// become ready within readyTimeout; the error includes the proxy logs.
func (s *S) Binary(locations string, opts ...ProxyOption) *Proxy {
	s.t.Helper()
	p, err := s.startBinary(locations, newProxyOptions(opts))
	if err != nil {
		s.Fatalf("%v", err)
	}
	return p
}

// TryBinary starts the compiled proxy for scenarios that check the startup
// itself: it returns the readiness error instead of failing the scenario.
func (s *S) TryBinary(locations string, opts ...ProxyOption) (*Proxy, error) {
	s.t.Helper()
	p, err := s.startBinary(locations, newProxyOptions(opts))
	if err != nil {
		s.Logf("binary proxy not ready: %v", err)
	}
	return p, err
}

// Signal sends sig to the binary proxy.
func (p *Proxy) Signal(sig os.Signal) error { return p.proc.cmd.Process.Signal(sig) }

// WaitExit waits until the binary proxy exits and returns its exit code;
// false if it is still running after timeout.
func (p *Proxy) WaitExit(timeout time.Duration) (int, bool) {
	select {
	case <-p.proc.exited:
		return p.proc.cmd.ProcessState.ExitCode(), true
	case <-time.After(timeout):
		return 0, false
	}
}

func (s *S) startBinary(locations string, o *proxyOptions) (*Proxy, error) {
	s.t.Helper()
	bin, err := proxyBinary(o.race)
	if err != nil {
		s.Fatalf("build: %v", err)
	}
	work := s.TempDir()
	pluginsSection := ""
	if !o.noPlugins {
		pluginsSection = s.preparePlugins(work, o)
	}
	logs := s.newLogBuffer("binary proxy")
	var lastErr error
	for range 3 {
		port, err := freePort()
		if err != nil {
			s.Fatalf("free port: %v", err)
		}
		portText := strconv.Itoa(port)
		text := s.renderConfig(o.configText(locations, portText, pluginsSection), portText)
		path := s.writeConfig(work, text)
		proc, err := startProcess(bin, work, path, logs)
		if err != nil {
			s.Fatalf("start %s: %v", bin, err)
		}
		s.Cleanup(proc.stop)
		p := &Proxy{
			Name: "binary proxy", URL: "http://127.0.0.1:" + portText,
			ConfigText: text, ConfigPath: path, Logs: logs, proc: proc,
		}
		if lastErr = waitReady(p); lastErr == nil {
			return p, nil
		}
		if !strings.Contains(logs.String(), "address already in use") {
			break
		}
		_, _ = fmt.Fprintf(logs, "e2e: port %d already in use, retrying\n", port)
	}
	return nil, fmt.Errorf("binary proxy not ready: %w\n--- proxy logs ---\n%s", lastErr, tail(logs.String(), 40))
}

// preparePlugins creates the plugins directory of a scenario and returns the
// plugins section of its configuration.
func (s *S) preparePlugins(work string, o *proxyOptions) string {
	keys, hash, err := signingKeys()
	if err != nil {
		s.Fatalf("signing keys: %v", err)
	}
	dir := filepath.Join(work, "plugins")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		s.Fatalf("plugins dir: %v", err)
	}
	if o.signed {
		signed, err := signedPlugin()
		if err != nil {
			s.Fatalf("signed plugin: %v", err)
		}
		dst := filepath.Join(dir, "hello-plugin")
		if err := copyDir(signed, dst); err != nil {
			s.Fatalf("copy plugin: %v", err)
		}
		if o.tamper {
			f, err := os.OpenFile(filepath.Join(dst, "hello-plugin.so"), os.O_APPEND|os.O_WRONLY, 0) //nolint:gosec // path built by the harness
			if err != nil {
				s.Fatalf("tamper: %v", err)
			}
			_, _ = f.WriteString("tampered")
			_ = f.Close()
		}
	}
	return fmt.Sprintf("plugins:\n  directory: %q\n  public_key_path: %q\n  public_key_hash: %q\n",
		dir, filepath.Join(keys, "ed25519_public.key"), hash)
}

func waitReady(p *Proxy) error {
	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(readyTimeout)
	for time.Now().Before(deadline) {
		select {
		case <-p.proc.exited:
			return fmt.Errorf("the process exited: %w", p.proc.err)
		default:
		}
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, p.URL+"/__e2e_ready__", nil)
		if err != nil {
			return err
		}
		if resp, err := client.Do(req); err == nil {
			_ = resp.Body.Close()
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("no HTTP response within %v", readyTimeout)
}

func freePort() (int, error) {
	var lc net.ListenConfig
	l, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer func() { _ = l.Close() }()
	addr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		return 0, errors.New("not a TCP address")
	}
	return addr.Port, nil
}

// Processes.

type process struct {
	binary string
	cmd    *exec.Cmd
	exited chan struct{}
	err    error // result of Wait, readable after exited is closed
}

// children holds the proxy processes still running.
var children sync.Map // *process -> struct{}

func init() { afterSuite = append(afterSuite, checkNoChildren) }

// checkNoChildren fails the suite if a proxy process outlived its scenario.
func checkNoChildren() bool {
	alive := 0
	children.Range(func(k, _ any) bool {
		if p, ok := k.(*process); ok {
			alive++
			p.kill()
		}
		return true
	})
	if alive > 0 {
		fmt.Fprintf(os.Stderr, "e2e: %d proxy process(es) still running after the suite\n", alive)
		return false
	}
	return true
}

func startProcess(bin, dir, config string, logs io.Writer) (*process, error) {
	cmd := exec.CommandContext(context.Background(), bin, "-f", config) //nolint:gosec // the harness runs the proxy it has just built
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = logs, logs
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	p := &process{binary: bin, cmd: cmd, exited: make(chan struct{})}
	children.Store(p, struct{}{})
	go func() {
		p.err = cmd.Wait()
		children.Delete(p)
		close(p.exited)
	}()
	return p, nil
}

// stop ends the process with SIGTERM, then SIGKILL after stopTimeout.
func (p *process) stop() {
	select {
	case <-p.exited:
		return
	default:
	}
	_ = p.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-p.exited:
	case <-time.After(stopTimeout):
		p.kill()
	}
}

func (p *process) kill() {
	_ = p.cmd.Process.Kill()
	<-p.exited
}

// Builds shared by the run.

var shared struct {
	once sync.Once
	dir  string
	err  error

	builds sync.Map // name -> *buildResult

	keysOnce sync.Once
	keysDir  string
	keysHash string
	keysErr  error

	pluginOnce sync.Once
	pluginDir  string
	pluginErr  error
}

// sharedDir is the temporary directory of the builds of this run, removed
// after the suite.
func sharedDir() string {
	shared.once.Do(func() {
		shared.dir, shared.err = os.MkdirTemp("", "dito-e2e-*")
		afterSuite = append(afterSuite, func() bool {
			_ = os.RemoveAll(shared.dir)
			return true
		})
	})
	return shared.dir
}

type buildResult struct {
	once sync.Once
	path string
	err  error
}

func buildOnce(name string, build func(out string) error) (string, error) {
	v, _ := shared.builds.LoadOrStore(name, &buildResult{})
	br, ok := v.(*buildResult)
	if !ok {
		return "", errors.New("unexpected build cache entry")
	}
	br.once.Do(func() {
		if shared.err != nil || sharedDir() == "" {
			br.err = fmt.Errorf("shared dir: %w", shared.err)
			return
		}
		br.path = filepath.Join(sharedDir(), name)
		br.err = build(br.path)
	})
	return br.path, br.err
}

var goDirective = regexp.MustCompile(`(?m)^go (\S+)$`)

// moduleRoot is the directory of the dito go.mod (the parent of e2e/).
func moduleRoot() (string, error) {
	root, err := filepath.Abs("..")
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		return "", fmt.Errorf("go.mod not found in %s: %w", root, err)
	}
	return root, nil
}

// goBuild builds with the toolchain declared in go.mod, like the CI.
func goBuild(dir, out string, args ...string) error {
	root, err := moduleRoot()
	if err != nil {
		return err
	}
	mod, err := os.ReadFile(filepath.Join(root, "go.mod")) //nolint:gosec // path built by the harness
	if err != nil {
		return err
	}
	m := goDirective.FindSubmatch(mod)
	if m == nil {
		return errors.New("no go directive in go.mod")
	}
	ctx, cancel := context.WithTimeout(context.Background(), buildTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", append([]string{"build", "-o", out}, args...)...) //nolint:gosec // builds this module's own binaries
	cmd.Dir = filepath.Join(root, dir)
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=go"+string(m[1]))
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go build %s in %s: %w\n%s", strings.Join(args, " "), cmd.Dir, err, output)
	}
	return nil
}

func proxyBinary(race bool) (string, error) {
	if race {
		return buildOnce("dito-race", func(out string) error { return goBuild(".", out, "-race", "./cmd") })
	}
	return buildOnce("dito", func(out string) error { return goBuild(".", out, "./cmd") })
}

func signerBinary() (string, error) {
	return buildOnce("plugin-signer", func(out string) error { return goBuild(".", out, "./cmd/plugin-signer") })
}

func helloPlugin() (string, error) {
	return buildOnce("hello-plugin.so", func(out string) error {
		return goBuild(filepath.Join("plugins", "hello-plugin"), out, "-buildmode=plugin", ".")
	})
}

// signingKeys generates the key pair of the run and returns its directory
// and the SHA-256 of the public key.
func signingKeys() (string, string, error) {
	shared.keysOnce.Do(func() {
		signer, err := signerBinary()
		if err != nil {
			shared.keysErr = err
			return
		}
		dir := filepath.Join(sharedDir(), "keys")
		if err := os.MkdirAll(dir, 0o750); err != nil {
			shared.keysErr = err
			return
		}
		if err := runIn(dir, signer, "generate-keys"); err != nil {
			shared.keysErr = err
			return
		}
		pub, err := os.ReadFile(filepath.Join(dir, "ed25519_public.key")) //nolint:gosec // path built by the harness
		if err != nil {
			shared.keysErr = err
			return
		}
		sum := sha256.Sum256(pub)
		shared.keysDir, shared.keysHash = dir, hex.EncodeToString(sum[:])
	})
	return shared.keysDir, shared.keysHash, shared.keysErr
}

// signedPlugin returns a directory with the example plugin, its signature
// and its configuration, ready to be copied into a plugins directory.
func signedPlugin() (string, error) {
	shared.pluginOnce.Do(func() {
		so, err := helloPlugin()
		if err != nil {
			shared.pluginErr = err
			return
		}
		keys, _, err := signingKeys()
		if err != nil {
			shared.pluginErr = err
			return
		}
		signer, err := signerBinary()
		if err != nil {
			shared.pluginErr = err
			return
		}
		root, err := moduleRoot()
		if err != nil {
			shared.pluginErr = err
			return
		}
		dir := filepath.Join(sharedDir(), "signed-hello-plugin")
		if err := os.MkdirAll(dir, 0o750); err != nil {
			shared.pluginErr = err
			return
		}
		target := filepath.Join(dir, "hello-plugin.so")
		if err := copyFile(so, target); err != nil {
			shared.pluginErr = err
			return
		}
		if err := copyFile(filepath.Join(root, "plugins", "hello-plugin", "config.yaml"), filepath.Join(dir, "config.yaml")); err != nil {
			shared.pluginErr = err
			return
		}
		shared.pluginErr = runIn(keys, signer, "sign", target)
		shared.pluginDir = dir
	})
	return shared.pluginDir, shared.pluginErr
}

func runIn(dir, bin string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), buildTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // runs the plugin-signer built by the harness
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s %s: %w\n%s", filepath.Base(bin), strings.Join(args, " "), err, output)
	}
	return nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src) //nolint:gosec // paths built by the harness
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o600) //nolint:gosec // path built by the harness
}

func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0o750); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := copyFile(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
			return err
		}
	}
	return nil
}
