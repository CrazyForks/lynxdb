package parser

import (
	"testing"

	"github.com/lynxbase/lynxdb/pkg/lynxflow/registry"
)

// TestRequiredPositionalsDiag feeds every registered operator an empty stage
// body; operators that declare required positionals (or required options)
// must produce at least one diagnostic instead of silently accepting the
// empty form. This pins the whole missing-argument bug class the fuzzer
// keeps finding one stage at a time.
func TestRequiredPositionalsDiag(t *testing.T) {
	for _, op := range registry.Operators() {
		if op.Class == registry.ClassSource {
			continue // from is tested separately
		}
		required := false
		for _, pos := range op.Positionals {
			if pos.Required {
				required = true
			}
		}
		for _, o := range op.Options {
			if o.Required {
				required = true
			}
		}
		if !required {
			continue
		}
		q := "from main | " + op.Name
		_, diags := Parse(q)
		if len(diags) == 0 {
			t.Errorf("%s: bare %q stage parsed with zero diags despite required arguments", op.Name, q)
		}
	}
}
