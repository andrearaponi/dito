package e2e

// Plugins and middlewares.

import (
	"context"
	"dito/plugin"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

// tagPlugin is an in-process plugin whose middleware adds its name to the
// X-Order request header.
type tagPlugin struct{ name string }

func (p tagPlugin) Name() string { return p.name }

func (p tagPlugin) Init(context.Context, map[string]any, plugin.AppAccessor) error { return nil }

func (p tagPlugin) MiddlewareFunc() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Header.Add("X-Order", p.name)
			next.ServeHTTP(w, r)
		})
	}
}

func TestPlugins_MiddlewareOrder(t *testing.T) {
	Run(t, func(s *S) {
		api := s.Backend("api", text("ok"))
		p := s.Proxy(`
  - path: "^/m$"
    target_url: "{{backend "api"}}/m"
    replace_path: true
    middlewares: ["first", "second"]`, WithPlugins(tagPlugin{"second"}, tagPlugin{"first"}))
		s.Get(p.URL + "/m")
		assert.Equal(s, []string{"first", "second"}, onlyRequest(s, api).Header.Values("X-Order"),
			"middlewares run in configuration order")
	})
}

func TestPlugins_CriticalMiddlewareMissing(t *testing.T) {
	Run(t, func(s *S) {
		api := s.Backend("api", text("ok"))
		p := s.Proxy(`
  - path: "^/protected$"
    target_url: "{{backend "api"}}/protected"
    replace_path: true
    middlewares: ["auth"]`)
		e := assertProxyError(s, s.Get(p.URL+"/protected"), http.StatusInternalServerError)
		assert.Equal(s, []any{"auth"}, e.Error.Details["missing_components"])
		assert.Empty(s, api.Requests(), "without its authentication middleware the location is blocked")
	})
}

func TestPlugins_SignedPluginLoaded(t *testing.T) {
	Run(t, func(s *S) {
		s.Backend("api", text("from backend"))
		p := s.Binary(`
  - path: "^/hello$"
    target_url: "{{backend "api"}}/hello"
    replace_path: true
    middlewares: ["hello-plugin"]`, WithSignedPlugin())
		r := s.Get(p.URL + "/hello")
		assert.Equal(s, http.StatusOK, r.Status)
		assert.NotEmpty(s, r.Header.Get("X-Hello-Plugin"), "the plugin middleware ran")
		assert.Equal(s, "from backend", r.Body())
		s.WaitLog(p, "Plugin loaded", 0)
	})
}

// A plugin altered after signing is rejected at startup: the signature check
// fails and the plugin is not loaded. Whether the proxy then keeps running is
// F-15, a security finding left to S-08 (C6), so it is not asserted here.
func TestPlugins_TamperedPluginRejected(t *testing.T) {
	Run(t, func(s *S) {
		s.Backend("api", text("from backend"))
		p, err := s.TryBinary(`
  - path: "^/hello$"
    target_url: "{{backend "api"}}/hello"
    replace_path: true
    middlewares: ["hello-plugin"]`, WithTamperedPlugin())
		var startupLogs string
		if err != nil {
			startupLogs = err.Error() // includes the logs of the process that exited
		} else {
			startupLogs = p.Logs.String() // plugins are loaded before the proxy is ready
		}
		assert.Contains(s, startupLogs, "plugin signature verification failed")
		assert.NotContains(s, startupLogs, "Plugin loaded")
	})
}
