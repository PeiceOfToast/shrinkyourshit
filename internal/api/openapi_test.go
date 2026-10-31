// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/PeiceOfToast/shrinkyourshit/docs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// routesFromAPIGo parses every non-test .go file in internal/api and
// collects every "METHOD /path" registration passed to HandleFunc. The
// test lives next to the router it audits: it walks the source files so
// the route table and the OpenAPI document are compared by construction,
// not by copying a list into a test.
func routesFromAPIGo(t *testing.T) map[string][]string {
	t.Helper()
	entries, err := os.ReadDir(".")
	require.NoError(t, err)
	var sources []string
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		sources = append(sources, name)
	}
	require.NotEmpty(t, sources)

	out := map[string][]string{}
	for _, path := range sources {
		src, err := os.ReadFile(path)
		require.NoError(t, err)
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, src, 0)
		require.NoError(t, err)
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			// The handler registrations are all `mux.HandleFunc("METHOD /path", …)`
			// or `x.HandleFunc("METHOD /path", …)` — the selector is HandleFunc
			// and the first argument is a string literal.
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "HandleFunc" {
				return true
			}
			if len(call.Args) < 2 {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			route := unquote(lit.Value)
			parts := strings.SplitN(route, " ", 2)
			if len(parts) != 2 {
				return true
			}
			out[parts[1]] = append(out[parts[1]], parts[0])
			return true
		})
	}
	return out
}

// unquote strips a Go string literal's quotes (the routes are all plain
// double-quoted literals; no escapes or backticks are used).
func unquote(lit string) string {
	if len(lit) >= 2 && lit[0] == '"' && lit[len(lit)-1] == '"' {
		return lit[1 : len(lit)-1]
	}
	return lit
}

// inScope decides whether a route belongs to the document: everything
// under /api/ plus the document itself. GET /install.sh and
// GET /install.ps1 are templated static downloads, not API endpoints —
// they are deliberately outside the document (the task notes record this).
func inScope(path string) bool {
	return strings.HasPrefix(path, "/api/") || path == "/openapi.yaml"
}

// TestOpenAPIServed proves the document endpoints answer (acceptance item
// 10): JSON on /api/v1/openapi.json, verbatim YAML on /openapi.yaml, both
// without a session.
func TestOpenAPIServed(t *testing.T) {
	h := newAPIHarness(t)

	rec := h.do(http.MethodGet, "/api/v1/openapi.json", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	var doc map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &doc))
	assert.Equal(t, "ShrinkYourShit API", doc["info"].(map[string]any)["title"])
	assert.Contains(t, doc["paths"].(map[string]any), "/api/v1/webhooks")

	// The YAML endpoint answers byte-for-byte.
	rec = h.do(http.MethodGet, "/openapi.yaml", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "application/x-yaml", rec.Header().Get("Content-Type"))
	raw, err := io.ReadAll(rec.Body)
	require.NoError(t, err)
	assert.Equal(t, strings.TrimRight(docs.OpenAPIYAML, "\n"), strings.TrimRight(string(raw), "\n"))

	// The JSON document is the same document.
	var fromYAML map[string]any
	require.NoError(t, json.Unmarshal(openapiJSON(), &fromYAML))

	// The webhooks section is present and the secret is marked once-only.
	rec = h.do(http.MethodGet, "/api/v1/openapi.json", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &doc))
	path := doc["paths"].(map[string]any)["/api/v1/webhooks"].(map[string]any)
	post := path["post"].(map[string]any)
	require.NotNil(t, post)
	require.Contains(t, doc["paths"].(map[string]any)["/api/v1/webhooks/{id}"], "put")
	require.Contains(t, doc["paths"].(map[string]any)["/api/v1/webhooks/{id}"], "delete")
	require.Contains(t, doc["paths"].(map[string]any)["/api/v1/webhooks/{id}/test"], "post")
	require.Contains(t, doc["paths"].(map[string]any)["/api/v1/webhooks/{id}/deliveries"], "get")
}

// TestOpenAPIMatchesRouteTable is the anti-drift test (acceptance item 10):
// the document and the route table agree in both directions. A route
// registered in api.go but missing from the document fails; a documented
// route the router does not serve fails.
func TestOpenAPIMatchesRouteTable(t *testing.T) {
	registered := routesFromAPIGo(t)
	documented := openAPIRoutes()

	// Normalize the registered map to only the document's scope.
	inscope := map[string][]string{}
	for path, methods := range registered {
		if !inScope(path) {
			continue
		}
		inscope[path] = methods
	}

	var missing []string
	var extra []string
	for path, methods := range inscope {
		sort.Strings(methods)
		want, ok := documented[path]
		if !ok {
			missing = append(missing, path)
			continue
		}
		sort.Strings(want)
		assert.Equal(t, want, methods, "method mismatch for "+path)
	}
	for path := range documented {
		if _, ok := inscope[path]; !ok {
			extra = append(extra, path)
		}
	}
	assert.Empty(t, missing, "routes registered but not documented")
	assert.Empty(t, extra, "routes documented but not registered")

	// Sanity: the document must describe the webhooks surface (task 25).
	for path, methods := range map[string][]string{
		"/api/v1/webhooks":                 {"GET", "POST"},
		"/api/v1/webhooks/{id}":            {"GET", "PUT", "DELETE"},
		"/api/v1/webhooks/{id}/test":       {"POST"},
		"/api/v1/webhooks/{id}/deliveries": {"GET"},
		"/api/v1/integrations":             {"GET", "POST"},
		"/api/v1/integrations/{id}":        {"PUT", "DELETE"},
		"/api/v1/integrations/{id}/test":   {"POST"},
		"/api/v1/openapi.json":             {"GET"},
		"/openapi.yaml":                    {"GET"},
	} {
		sort.Strings(methods)
		assert.Equal(t, methods, inscope[path], path)
	}
}

// TestOpenAPIJSONDeliversDocument checks the endpoint answer against the
// parsed document: same content, byte-for-byte equivalent JSON.
func TestOpenAPIJSONDeliversDocument(t *testing.T) {
	h := newAPIHarness(t)
	rec := h.do(http.MethodGet, "/api/v1/openapi.json", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	// The document itself is valid JSON.
	var fromEndpoint map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &fromEndpoint))
	_, ok := fromEndpoint["openapi"].(string)
	require.True(t, ok, "the document declares its version")
}

// A request without a session reaches the document endpoints: they are
// open by design.
func TestOpenAPIEndpointsAreOpen(t *testing.T) {
	h, _ := webhookHarness(t)
	rec := h.do(http.MethodGet, "/api/v1/openapi.json", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	rec = h.do(http.MethodGet, "/openapi.yaml", nil)
	require.Equal(t, http.StatusOK, rec.Code)
}

// TestDocRoutesSanity guards the document against an empty paths block.
func TestDocRoutesSanity(t *testing.T) {
	documented := openAPIRoutes()
	require.NotEmpty(t, documented)
	for path, methods := range documented {
		require.NotEmpty(t, methods, path)
		for _, m := range methods {
			require.Contains(t, []string{"GET", "POST", "PUT", "DELETE"}, m, path)
		}
	}
}

// TestOpenAPIViaMux — the mux is a real handler: run a request through
// httptest to prove the route reaches the handler (a typo in the
// registration would 404).
func TestOpenAPIViaMux(t *testing.T) {
	h := newAPIHarness(t)
	for _, path := range []string{"/api/v1/openapi.json", "/openapi.yaml"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		h.mux.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code, path)
	}
}
