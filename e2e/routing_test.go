package e2e

// Routing: how a request path selects a location and the backend path.

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRouting_RegexMatch(t *testing.T) {
	Run(t, func(s *S) {
		api := s.Backend("api", text("api"))
		p := s.Proxy(`
  - path: "^/v[0-9]+/items$"
    target_url: "{{backend "api"}}/items"
    replace_path: true`)
		r := s.Get(p.URL + "/v2/items")
		assert.Equal(s, http.StatusOK, r.Status)
		assert.Equal(s, "api", r.Body())
		r = s.Get(p.URL + "/vx/items")
		assert.Equal(s, http.StatusNotFound, r.Status, "a path the regex does not match")
		assert.Len(s, api.Requests(), 1)
	})
}

func TestRouting_NoLocation404(t *testing.T) {
	Run(t, func(s *S) {
		api := s.Backend("api", text("api"))
		p := s.Proxy(`
  - path: "^/known$"
    target_url: "{{backend "api"}}/known"`)
		r := s.Get(p.URL + "/unknown")
		assert.Equal(s, http.StatusNotFound, r.Status)
		assert.True(s, strings.HasPrefix(r.Header.Get("Content-Type"), "application/json"))
		e, err := r.ProxyError()
		require.NoError(s, err)
		assert.Equal(s, http.StatusNotFound, e.Error.Code)
		assert.Empty(s, api.Requests(), "no backend is called")
	})
}

func TestRouting_FirstLocationWins(t *testing.T) {
	Run(t, func(s *S) {
		s.Backend("shop", text("shop"))
		s.Backend("cart", text("cart"))
		p := s.Proxy(`
  - path: "^/shop"
    target_url: "{{backend "shop"}}/"
    replace_path: true
  - path: "^/shop/cart$"
    target_url: "{{backend "cart"}}/"
    replace_path: true`)
		r := s.Get(p.URL + "/shop/cart")
		assert.Equal(s, "shop", r.Body(), "locations are matched in configuration order")
	})
}

func TestRouting_ReplacePathTrue(t *testing.T) {
	Run(t, func(s *S) {
		api := s.Backend("api", text("ok"))
		p := s.Proxy(`
  - path: "^/old/path$"
    target_url: "{{backend "api"}}/new/path"
    replace_path: true`)
		s.Get(p.URL + "/old/path")
		reqs := api.Requests()
		require.Len(s, reqs, 1)
		assert.Equal(s, "/new/path", reqs[0].Path)
	})
}

// R-05: without replace_path, the part of the path matched by the location is
// replaced by the target path.
func TestRouting_R05_ReplacePathFalse(t *testing.T) {
	KnownBug("F-04").Run(t, func(s *S) {
		api := s.Backend("api", text("ok"))
		p := s.Proxy(`
  - path: "^/api"
    target_url: "{{backend "api"}}/base"
    replace_path: false`)
		s.Get(p.URL + "/api/users")
		reqs := api.Requests()
		require.Len(s, reqs, 1)
		assert.Equal(s, "/base/users", reqs[0].Path)
	})
}

func TestRouting_QueryPreserved(t *testing.T) {
	Run(t, func(s *S) {
		api := s.Backend("api", text("ok"))
		p := s.Proxy(`
  - path: "^/search$"
    target_url: "{{backend "api"}}/search"
    replace_path: true`)
		s.Get(p.URL + "/search?q=two%20words&page=2")
		reqs := api.Requests()
		require.Len(s, reqs, 1)
		assert.Equal(s, "q=two%20words&page=2", reqs[0].RawQuery)
	})
}
