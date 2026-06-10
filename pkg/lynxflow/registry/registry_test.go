package registry

import (
	"strings"
	"testing"
)

func TestOperatorInvariants(t *testing.T) {
	seen := map[string]bool{}
	classes := map[Class]bool{ClassSource: true, ClassCore: true, ClassSugar: true, ClassHelper: true, ClassManagement: true}
	streaming := map[Streaming]bool{StreamingRow: true, StreamingAcc: true}
	for _, op := range Operators() {
		if op.Name == "" || op.Name != strings.ToLower(op.Name) {
			t.Errorf("operator %q: name must be non-empty lowercase", op.Name)
		}
		if seen[op.Name] {
			t.Errorf("operator %q: duplicate name", op.Name)
		}
		seen[op.Name] = true
		if !classes[op.Class] {
			t.Errorf("operator %q: invalid class %q", op.Name, op.Class)
		}
		if !streaming[op.Streaming] {
			t.Errorf("operator %q: invalid streaming class %q", op.Name, op.Streaming)
		}
		if op.Class == ClassSugar && op.DesugarsTo == "" {
			t.Errorf("operator %q: sugar without DesugarsTo", op.Name)
		}
		if op.Class != ClassSugar && op.DesugarsTo != "" {
			t.Errorf("operator %q: DesugarsTo on non-sugar class %q", op.Name, op.Class)
		}
		if op.Doc == "" {
			t.Errorf("operator %q: missing doc", op.Name)
		}
		for _, o := range op.Options {
			if o.Type == ArgEnum && len(o.Enum) == 0 {
				t.Errorf("operator %q option %q: enum type without values", op.Name, o.Name)
			}
			if o.Type != ArgEnum && len(o.Enum) > 0 {
				t.Errorf("operator %q option %q: enum values on non-enum type", op.Name, o.Name)
			}
		}
		variadicSeen := false
		for _, p := range op.Positionals {
			if variadicSeen {
				t.Errorf("operator %q: positional %q after variadic", op.Name, p.Name)
			}
			variadicSeen = p.Variadic
		}
	}
	for _, want := range []string{"from", "where", "parse", "extend", "keep", "drop", "rename", "stats", "eventstats", "streamstats", "sort", "head", "tail", "dedup", "join", "union", "explode", "describe"} {
		op, ok := LookupOperator(want)
		if !ok {
			t.Errorf("core operator %q missing", want)
			continue
		}
		if want != "from" && op.Class != ClassCore {
			t.Errorf("operator %q: expected core class, got %q", want, op.Class)
		}
	}
}

func TestFunctionInvariants(t *testing.T) {
	seen := map[string]bool{}
	for _, fn := range Functions() {
		if fn.Name == "" || fn.Name != strings.ToLower(fn.Name) {
			t.Errorf("function %q: name must be non-empty lowercase", fn.Name)
		}
		if seen[fn.Name] {
			t.Errorf("function %q: duplicate name", fn.Name)
		}
		seen[fn.Name] = true
		if fn.Category == "" {
			t.Errorf("function %q: missing category", fn.Name)
		}
		if fn.Result == "" {
			t.Errorf("function %q: missing result type", fn.Name)
		}
		if fn.Fallibility != Infallible && fn.Fallibility != NullOnFailure {
			t.Errorf("function %q: invalid fallibility %q", fn.Name, fn.Fallibility)
		}
		if fn.StrictVariant && fn.Fallibility == Infallible {
			t.Errorf("function %q: strict variant on infallible function", fn.Name)
		}
		optionalSeen := false
		for _, p := range fn.Params {
			if optionalSeen && !p.Optional && !p.Variadic {
				t.Errorf("function %q: required param %q after optional", fn.Name, p.Name)
			}
			if p.Optional {
				optionalSeen = true
			}
		}
	}
	for _, want := range []string{"has", "contains", "matches", "glob", "exists", "is_null", "is_missing", "int", "from_json", "any"} {
		if _, ok := LookupFunction(want); !ok {
			t.Errorf("function %q missing", want)
		}
	}
}

func TestAggregateInvariants(t *testing.T) {
	seen := map[string]bool{}
	for _, ag := range Aggregates() {
		if ag.Name == "" || ag.Name != strings.ToLower(ag.Name) {
			t.Errorf("aggregate %q: name must be non-empty lowercase", ag.Name)
		}
		if seen[ag.Name] {
			t.Errorf("aggregate %q: duplicate name", ag.Name)
		}
		seen[ag.Name] = true
		if ag.Result == "" {
			t.Errorf("aggregate %q: missing result type", ag.Name)
		}
		if ag.WindowOnly && ag.SupportsWhere {
			t.Errorf("aggregate %q: window functions do not take where clauses", ag.Name)
		}
	}
	for _, killed := range []string{"perc95", "percentile95", "exactperc95", "upperperc95", "median", "mean"} {
		if _, ok := LookupAggregate(killed); ok {
			t.Errorf("aggregate %q: RFC-002 kills the percentile alias zoo", killed)
		}
	}
}

func TestNamespacesDoNotCollide(t *testing.T) {
	fns := map[string]bool{}
	for _, fn := range Functions() {
		fns[fn.Name] = true
	}
	for _, ag := range Aggregates() {
		if fns[ag.Name] {
			// values/list-style collisions between scalar and aggregate
			// namespaces are resolved by context, but identical names with
			// different meanings are confusing; keep the overlap explicit.
			switch ag.Name {
			case "values": // object values() vs aggregate values()
			default:
				t.Errorf("name %q exists as both scalar function and aggregate", ag.Name)
			}
		}
	}
}
