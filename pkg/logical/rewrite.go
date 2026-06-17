package logical

import "github.com/lynxbase/lynxdb/pkg/lynxflow/desugar"

// Rewrite documents a visible query rewrite produced outside the pure desugar
// pass, such as schema-resolved macro expansion during lowering.
type Rewrite = desugar.Rewrite
