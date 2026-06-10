package rest

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/lynxbase/lynxdb/pkg/lynxflow/registry"
)

// catalogCache holds the pre-serialized catalog JSON and its ETag.
// Computed once on first request; immutable thereafter (the registry is frozen).
var catalogCache struct {
	once sync.Once
	body []byte
	etag string
}

// handleCatalog serves GET /api/v1/catalog: the full LynxFlow v2 language
// surface from pkg/lynxflow/registry. The response is cache-friendly with an
// ETag header (the registry is frozen at compile time).
func (s *Server) handleCatalog(w http.ResponseWriter, r *http.Request) {
	catalogCache.once.Do(func() {
		catalogCache.body, catalogCache.etag = buildCatalogResponse()
	})

	// ETag-based conditional GET.
	if match := r.Header.Get("If-None-Match"); match == catalogCache.etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("ETag", catalogCache.etag)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(catalogCache.body)
}

// buildCatalogResponse serializes the registry into the catalog JSON shape
// and computes a content-based ETag.
func buildCatalogResponse() ([]byte, string) {
	type catalogOperator struct {
		Name        string                `json:"name"`
		Class       string                `json:"class"`
		Streaming   string                `json:"streaming"`
		Positionals []registry.Positional `json:"positionals,omitempty"`
		Options     []registry.Option     `json:"options,omitempty"`
		DesugarsTo  string                `json:"desugars_to,omitempty"`
		Doc         string                `json:"doc"`
		Examples    []string              `json:"examples,omitempty"`
	}

	type catalogFunction struct {
		Name          string           `json:"name"`
		Category      string           `json:"category"`
		Params        []registry.Param `json:"params,omitempty"`
		Result        string           `json:"result"`
		Fallibility   string           `json:"fallibility"`
		StrictVariant bool             `json:"strict_variant,omitempty"`
		Doc           string           `json:"doc"`
	}

	type catalogAggregate struct {
		Name          string           `json:"name"`
		Params        []registry.Param `json:"params,omitempty"`
		SupportsWhere bool             `json:"supports_where,omitempty"`
		WindowOnly    bool             `json:"window_only,omitempty"`
		Result        string           `json:"result"`
		Doc           string           `json:"doc"`
	}

	type catalog struct {
		Operators    []catalogOperator  `json:"operators"`
		Functions    []catalogFunction  `json:"functions"`
		Aggregates   []catalogAggregate `json:"aggregates"`
		ParseFormats []string           `json:"parse_formats"`
	}

	ops := registry.Operators()
	catOps := make([]catalogOperator, len(ops))
	for i, op := range ops {
		catOps[i] = catalogOperator{
			Name:        op.Name,
			Class:       string(op.Class),
			Streaming:   string(op.Streaming),
			Positionals: op.Positionals,
			Options:     op.Options,
			DesugarsTo:  op.DesugarsTo,
			Doc:         op.Doc,
			Examples:    op.Examples,
		}
	}

	fns := registry.Functions()
	catFns := make([]catalogFunction, len(fns))
	for i, fn := range fns {
		catFns[i] = catalogFunction{
			Name:          fn.Name,
			Category:      fn.Category,
			Params:        fn.Params,
			Result:        string(fn.Result),
			Fallibility:   string(fn.Fallibility),
			StrictVariant: fn.StrictVariant,
			Doc:           fn.Doc,
		}
	}

	aggs := registry.Aggregates()
	catAggs := make([]catalogAggregate, len(aggs))
	for i, ag := range aggs {
		catAggs[i] = catalogAggregate{
			Name:          ag.Name,
			Params:        ag.Params,
			SupportsWhere: ag.SupportsWhere,
			WindowOnly:    ag.WindowOnly,
			Result:        string(ag.Result),
			Doc:           ag.Doc,
		}
	}

	c := catalog{
		Operators:    catOps,
		Functions:    catFns,
		Aggregates:   catAggs,
		ParseFormats: registry.ParseFormats(),
	}

	body, _ := json.Marshal(c)
	hash := sha256.Sum256(body)
	etag := fmt.Sprintf(`"%x"`, hash[:8])

	return body, etag
}
