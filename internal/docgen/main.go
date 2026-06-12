// Command docgen generates documentation artefacts from the LynxFlow registry
// (pkg/lynxflow/registry). It is the single source of truth for:
//
//   - docs/site/docs/lynxflow/operators/<name>.md   — one page per operator
//   - docs/site/docs/lynxflow/functions.md           — scalar function table
//   - docs/site/docs/lynxflow/aggregates.md          — aggregate function table
//   - docs/grammar/lynxflow.ebnf                     — EBNF grammar
//
// Run: go run ./internal/docgen
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lynxbase/lynxdb/pkg/lynxflow/registry"
)

func main() {
	root := findRepoRoot()
	if err := Generate(root); err != nil {
		fmt.Fprintf(os.Stderr, "docgen: %v\n", err)
		os.Exit(1)
	}
}

// Generate writes all generated documentation artefacts under root.
func Generate(root string) error {
	if err := generateOperatorPages(root); err != nil {
		return fmt.Errorf("operator pages: %w", err)
	}
	if err := generateFunctionsPage(root); err != nil {
		return fmt.Errorf("functions page: %w", err)
	}
	if err := generateAggregatesPage(root); err != nil {
		return fmt.Errorf("aggregates page: %w", err)
	}
	if err := generateEBNF(root); err != nil {
		return fmt.Errorf("ebnf: %w", err)
	}
	return nil
}

// operator pages

func generateOperatorPages(root string) error {
	dir := filepath.Join(root, "docs", "site", "docs", "lynxflow", "operators")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, op := range registry.Operators() {
		path := filepath.Join(dir, op.Name+".md")
		content := renderOperatorPage(op)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	return nil
}

func renderOperatorPage(op registry.Operator) string {
	var b strings.Builder

	// Front-matter
	b.WriteString("---\n")
	fmt.Fprintf(&b, "title: \"%s\"\n", op.Name)
	fmt.Fprintf(&b, "sidebar_label: \"%s\"\n", op.Name)
	b.WriteString("---\n\n")

	// Title + badges
	fmt.Fprintf(&b, "# %s\n\n", op.Name)
	fmt.Fprintf(&b, "**Class:** `%s`", op.Class)
	if op.Streaming == registry.StreamingRow {
		b.WriteString(" &middot; **Streaming:** row-at-a-time")
	} else {
		b.WriteString(" &middot; **Streaming:** accumulating")
	}
	b.WriteString("\n\n")
	b.WriteString(escapeMDX(op.Doc) + "\n\n")

	// Signature
	b.WriteString("## Signature\n\n```\n")
	b.WriteString("| " + op.Name)
	for _, p := range op.Positionals {
		if p.Required {
			fmt.Fprintf(&b, " <%s>", p.Name)
		} else {
			fmt.Fprintf(&b, " [%s]", p.Name)
		}
		if p.Variadic {
			b.WriteString("...")
		}
	}
	for _, o := range op.Options {
		if o.Required {
			fmt.Fprintf(&b, " %s=<%s>", o.Name, o.Type)
		} else {
			fmt.Fprintf(&b, " [%s=<%s>]", o.Name, o.Type)
		}
	}
	b.WriteString("\n```\n\n")

	// Positional arguments
	if len(op.Positionals) > 0 {
		b.WriteString("## Positional Arguments\n\n")
		b.WriteString("| Name | Type | Required | Description |\n")
		b.WriteString("|------|------|----------|-------------|\n")
		for _, p := range op.Positionals {
			req := "No"
			if p.Required {
				req = "Yes"
			}
			name := p.Name
			if p.Variadic {
				name += "..."
			}
			doc := escapeMDX(p.Doc)
			if doc == "" {
				doc = "-"
			}
			fmt.Fprintf(&b, "| `%s` | `%s` | %s | %s |\n", name, p.Type, req, doc)
		}
		b.WriteString("\n")
	}

	// Options
	if len(op.Options) > 0 {
		b.WriteString("## Options\n\n")
		b.WriteString("| Name | Type | Default | Description |\n")
		b.WriteString("|------|------|---------|-------------|\n")
		for _, o := range op.Options {
			def := o.Default
			if def == "" {
				def = "-"
			}
			doc := escapeMDX(o.Doc)
			if doc == "" {
				doc = "-"
			}
			if o.Type == registry.ArgEnum && len(o.Enum) > 0 {
				doc += fmt.Sprintf(" Values: `%s`.", strings.Join(o.Enum, "`, `"))
			}
			fmt.Fprintf(&b, "| `%s` | `%s` | `%s` | %s |\n", o.Name, o.Type, def, doc)
		}
		b.WriteString("\n")
	}

	// DesugarsTo
	if op.DesugarsTo != "" {
		b.WriteString("## Desugars To\n\n")
		fmt.Fprintf(&b, "```\n%s\n```\n\n", op.DesugarsTo)
	}

	// Examples
	if len(op.Examples) > 0 {
		b.WriteString("## Examples\n\n")
		for _, ex := range op.Examples {
			fmt.Fprintf(&b, "```\n%s\n```\n\n", ex)
		}
	}

	// Spec link
	b.WriteString("---\n\n")
	b.WriteString("*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*\n")

	return b.String()
}

// functions page

func generateFunctionsPage(root string) error {
	path := filepath.Join(root, "docs", "site", "docs", "lynxflow", "functions.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	fns := registry.Functions()
	cats := groupFunctionsByCategory(fns)

	var b strings.Builder
	b.WriteString("---\ntitle: \"Scalar Functions\"\nsidebar_label: \"Scalar Functions\"\n---\n\n")
	b.WriteString("# Scalar Functions\n\n")
	b.WriteString("All scalar functions available in LynxFlow expressions. ")
	b.WriteString("Functions marked **null_on_failure** return `null` when the input is invalid; ")
	b.WriteString("those with a **strict variant** (`name!`) raise a query error instead.\n\n")

	for _, cat := range cats {
		fmt.Fprintf(&b, "## %s\n\n", titleCase(cat.name))
		b.WriteString("| Function | Params | Result | Fallibility | Strict | Description |\n")
		b.WriteString("|----------|--------|--------|-------------|--------|-------------|\n")
		for _, fn := range cat.fns {
			params := renderParams(fn.Params)
			strict := "-"
			if fn.StrictVariant {
				strict = fmt.Sprintf("`%s!`", fn.Name)
			}
			doc := escapeMDX(fn.Doc)
			if doc == "" {
				doc = "-"
			}
			fmt.Fprintf(&b, "| `%s` | %s | `%s` | %s | %s | %s |\n",
				fn.Name, params, fn.Result, fn.Fallibility, strict, doc)
		}
		b.WriteString("\n")
	}

	b.WriteString("---\n\n")
	b.WriteString("*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/functions.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full specification.*\n")

	return os.WriteFile(path, []byte(b.String()), 0o644)
}

type funcCategory struct {
	name string
	fns  []registry.Function
}

func groupFunctionsByCategory(fns []registry.Function) []funcCategory {
	m := map[string][]registry.Function{}
	var order []string
	for _, fn := range fns {
		if _, ok := m[fn.Category]; !ok {
			order = append(order, fn.Category)
		}
		m[fn.Category] = append(m[fn.Category], fn)
	}
	var cats []funcCategory
	for _, name := range order {
		cats = append(cats, funcCategory{name: name, fns: m[name]})
	}
	return cats
}

func renderParams(params []registry.Param) string {
	if len(params) == 0 {
		return "()"
	}
	var parts []string
	for _, p := range params {
		s := fmt.Sprintf("%s: %s", p.Name, p.Type)
		if p.Optional {
			s += "?"
		}
		if p.Variadic {
			s += "..."
		}
		parts = append(parts, s)
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

// aggregates page

func generateAggregatesPage(root string) error {
	path := filepath.Join(root, "docs", "site", "docs", "lynxflow", "aggregates.md")

	aggs := registry.Aggregates()

	// Split into standard and window-only
	var standard, window []registry.Aggregate
	for _, ag := range aggs {
		if ag.WindowOnly {
			window = append(window, ag)
		} else {
			standard = append(standard, ag)
		}
	}

	var b strings.Builder
	b.WriteString("---\ntitle: \"Aggregate Functions\"\nsidebar_label: \"Aggregate Functions\"\n---\n\n")
	b.WriteString("# Aggregate Functions\n\n")
	b.WriteString("All aggregate and window functions available in `stats`, `eventstats`, and `streamstats` stages.\n\n")

	b.WriteString("## Standard Aggregates\n\n")
	b.WriteString("All standard aggregates support `where` clauses for conditional aggregation: `count(where status >= 500)`.\n\n")
	b.WriteString("| Function | Params | Result | Description |\n")
	b.WriteString("|----------|--------|--------|-------------|\n")
	for _, ag := range standard {
		params := renderAggParams(ag.Params)
		doc := escapeMDX(ag.Doc)
		if doc == "" {
			doc = "-"
		}
		fmt.Fprintf(&b, "| `%s` | %s | `%s` | %s |\n", ag.Name, params, ag.Result, doc)
	}
	b.WriteString("\n")

	if len(window) > 0 {
		b.WriteString("## Window Functions (streamstats only)\n\n")
		b.WriteString("| Function | Params | Result | Description |\n")
		b.WriteString("|----------|--------|--------|-------------|\n")
		for _, ag := range window {
			params := renderAggParams(ag.Params)
			doc := escapeMDX(ag.Doc)
			if doc == "" {
				doc = "-"
			}
			fmt.Fprintf(&b, "| `%s` | %s | `%s` | %s |\n", ag.Name, params, ag.Result, doc)
		}
		b.WriteString("\n")
	}

	b.WriteString("---\n\n")
	b.WriteString("*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/aggregates.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full specification.*\n")

	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func renderAggParams(params []registry.Param) string {
	if len(params) == 0 {
		return "()"
	}
	var parts []string
	for _, p := range params {
		s := fmt.Sprintf("%s: %s", p.Name, p.Type)
		if p.Optional {
			s += "?"
		}
		if p.Variadic {
			s += "..."
		}
		parts = append(parts, s)
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

// EBNF generation

func generateEBNF(root string) error {
	path := filepath.Join(root, "docs", "grammar", "lynxflow.ebnf")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	var b strings.Builder
	b.WriteString("(* LynxFlow v2 Grammar — generated from pkg/lynxflow/registry *)\n")
	b.WriteString("(* Expression grammar: RFC-002 §4.2 (normative) *)\n")
	b.WriteString("(* Stage productions: registry-derived *)\n\n")

	// Expression grammar from RFC-002 §4.2 — embedded as a static block
	b.WriteString("(* ─── Expression grammar (RFC-002 §4.2) ─── *)\n\n")
	b.WriteString(`expr        ::= or_expr ;
or_expr     ::= and_expr ('or' and_expr)* ;
and_expr    ::= not_expr ('and' not_expr)* ;
not_expr    ::= 'not' not_expr | cmp_expr ;
cmp_expr    ::= coal_expr (cmp_op coal_expr
              | 'in' coal_expr
              | 'between' coal_expr 'and' coal_expr)? ;
cmp_op      ::= '==' | '!=' | '<' | '<=' | '>' | '>=' ;
coal_expr   ::= add_expr ('??' add_expr)* ;
add_expr    ::= mul_expr (('+' | '-') mul_expr)* ;
mul_expr    ::= unary_expr (('*' | '/' | '%') unary_expr)* ;
unary_expr  ::= '-' unary_expr | postfix ;
postfix     ::= primary (call_args | '.' ident | '?.' ident | '[' expr ']')* ;
call_args   ::= '(' [arg (',' arg)*] ')' ;
arg         ::= expr | 'where' expr ;
primary     ::= literal | ident | backtick_ident | '(' expr ')' | lambda
              | array_lit | object_lit ;
lambda      ::= ident '->' expr ;
array_lit   ::= '[' [expr (',' expr)*] ']' ;
object_lit  ::= '{' [obj_entry (',' obj_entry)*] '}' ;
obj_entry   ::= (ident | string) ':' expr ;
`)
	b.WriteString("\n")

	// Top-level pipeline
	b.WriteString("(* ─── Pipeline ─── *)\n\n")
	b.WriteString("program     ::= { cte_def } pipeline ;\n")
	b.WriteString("cte_def     ::= 'let' '$' ident '=' pipeline ';' ;\n")
	b.WriteString("pipeline    ::= stage { '|' stage } ;\n")
	b.WriteString("stage       ::= " + stageAlternation() + " ;\n\n")

	// One production per operator
	b.WriteString("(* ─── Stage productions (registry-generated) ─── *)\n\n")

	ops := registry.Operators()
	sort.Slice(ops, func(i, j int) bool { return ops[i].Name < ops[j].Name })
	for _, op := range ops {
		b.WriteString(renderStageProduction(op))
	}

	// Common non-terminals
	b.WriteString("\n(* ─── Common non-terminals ─── *)\n\n")
	b.WriteString("ident       ::= (letter | '_') { letter | digit | '_' | '-' | '.' } ;\n")
	b.WriteString("field       ::= ident | backtick_ident ;\n")
	b.WriteString("field_list  ::= field { ',' field } ;\n")
	b.WriteString("sort_list   ::= sort_key { ',' sort_key } ;\n")
	b.WriteString("sort_key    ::= ['+' | '-'] field ;\n")
	b.WriteString("agg_list    ::= agg_call { ',' agg_call } ;\n")
	b.WriteString("agg_call    ::= ident '(' [expr_list] [',']? ['where' expr]? ')' ['as' ident] ;\n")
	b.WriteString("assign_list ::= assignment { ',' assignment } ;\n")
	b.WriteString("assignment  ::= ident '=' expr ;\n")
	b.WriteString("sub_pipeline ::= '$' ident | '[' pipeline ']' ;\n")
	b.WriteString("duration    ::= number duration_unit ;\n")
	b.WriteString("duration_unit ::= 'ns' | 'us' | 'ms' | 's' | 'm' | 'h' | 'd' | 'w' ;\n")
	b.WriteString("literal     ::= string | number | 'true' | 'false' | 'null' ;\n")

	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func stageAlternation() string {
	ops := registry.Operators()
	names := make([]string, len(ops))
	for i, op := range ops {
		names[i] = op.Name + "_stage"
	}
	sort.Strings(names)
	return strings.Join(names, "\n              | ")
}

func renderStageProduction(op registry.Operator) string {
	var b strings.Builder
	name := op.Name + "_stage"
	fmt.Fprintf(&b, "%-16s ::= '%s'", name, op.Name)

	for _, p := range op.Positionals {
		nt := argTypeToNonTerminal(p.Type)
		if p.Required {
			b.WriteString(" " + nt)
		} else {
			fmt.Fprintf(&b, " [%s]", nt)
		}
		if p.Variadic {
			b.WriteString("...")
		}
	}
	for _, o := range op.Options {
		nt := argTypeToNonTerminal(o.Type)
		fmt.Fprintf(&b, " ['%s' '=' %s]", o.Name, nt)
	}
	b.WriteString(" ;\n")
	return b.String()
}

func argTypeToNonTerminal(t registry.ArgType) string {
	switch t {
	case registry.ArgExpr:
		return "expr"
	case registry.ArgPredicate:
		return "expr"
	case registry.ArgField:
		return "field"
	case registry.ArgFieldList:
		return "field_list"
	case registry.ArgFieldPatterns:
		return "field_patterns"
	case registry.ArgAssignList:
		return "assign_list"
	case registry.ArgAggList:
		return "agg_list"
	case registry.ArgSortList:
		return "sort_list"
	case registry.ArgInt:
		return "number"
	case registry.ArgString:
		return "string"
	case registry.ArgDuration:
		return "duration"
	case registry.ArgFormat:
		return "format_spec"
	case registry.ArgCaptures:
		return "captures"
	case registry.ArgSubPipeline:
		return "sub_pipeline"
	case registry.ArgEnum:
		return "ident"
	case registry.ArgBool:
		return "'true' | 'false'"
	default:
		return string(t)
	}
}

// escapeMDX replaces angle brackets with HTML entities so Docusaurus MDX does
// not interpret them as JSX tags.
func escapeMDX(s string) string {
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "{", "&#123;")
	s = strings.ReplaceAll(s, "}", "&#125;")
	return s
}

// titleCase uppercases the first rune of s.
func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// findRepoRoot walks up from cwd until it finds go.mod.
func findRepoRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "docgen: cannot determine cwd: %v\n", err)
		os.Exit(1)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			fmt.Fprintf(os.Stderr, "docgen: cannot find repo root (go.mod)\n")
			os.Exit(1)
		}
		dir = parent
	}
}
