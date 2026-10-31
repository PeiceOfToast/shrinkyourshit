// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/PeiceOfToast/shrinkyourshit/docs"
	"gopkg.in/yaml.v3"
)

// openapiJSON is the OpenAPI document converted to JSON once at startup
// (task 25). A conversion failure is a programming error in the shipped
// document; it is fatal so a broken build never reaches a release.
func openapiJSON() []byte {
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(docs.OpenAPIYAML), &doc); err != nil {
		panic("api: shipped docs/openapi.yaml does not parse: " + err.Error())
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		panic("api: shipped docs/openapi.yaml does not convert to JSON: " + err.Error())
	}
	return raw
}

// registerOpenAPI wires the two document endpoints (task 25, API.md):
//
//	GET /api/v1/openapi.json — the document as JSON.
//	GET /openapi.yaml        — the document verbatim.
//
// Both are open: a document that needed a login would be unreadable from
// a fresh browser tab, and it carries no secrets.
func registerOpenAPI(mux *http.ServeMux) {
	docJSON := openapiJSON()
	docYAML := []byte(docs.OpenAPIYAML)

	mux.HandleFunc("GET /api/v1/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(docJSON)
	})
	mux.HandleFunc("GET /openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-yaml")
		_, _ = w.Write(bytes.TrimRight(docYAML, "\n"))
	})
}

// openAPIRoutes is every route in the document, as METHOD → path. The
// anti-drift test (openapi_test.go) diffs this against the mux's own
// registrations so the document and the router can never disagree.
func openAPIRoutes() map[string][]string {
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(docs.OpenAPIYAML), &doc); err != nil {
		panic("api: shipped docs/openapi.yaml does not parse: " + err.Error())
	}
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		panic("api: shipped docs/openapi.yaml has no paths")
	}
	out := map[string][]string{}
	for path, def := range paths {
		ops, ok := def.(map[string]any)
		if !ok {
			continue
		}
		for method := range ops {
			if strings.EqualFold(method, "parameters") {
				continue
			}
			out[path] = append(out[path], strings.ToUpper(method))
		}
	}
	return out
}
