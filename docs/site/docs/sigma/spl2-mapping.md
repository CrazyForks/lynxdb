# Sigma to SPL2 mapping (legacy)

[Back to Sigma docs](index.md)

:::warning Legacy dialect — LynxDB does not execute SPL2

This page documents the **legacy SPL2** output that rsigma v0.9.0's `lynxdb`
target emits. LynxDB executes **LynxFlow v2 only**; the SPL2 fragments below
no longer parse. Keep using this page as a migration aid: find the SPL2 shape
rsigma emitted, then write the LynxFlow equivalent from the third column. The
page URL is kept stable for existing links.

:::

Precedence differs between the two dialects. Legacy SPL2 search precedence was
`NOT > OR > AND` (OR bound tighter than AND). LynxFlow uses standard boolean
precedence: `and` binds tighter than `or`, and `not` applies to the following
predicate. rsigma inserts parentheses into its output, and the safest
migration is to keep them.

## Construct mapping

| Sigma | rsigma legacy SPL2 fragment (not executable) | LynxFlow equivalent |
|---|---|---|
| `field: value` | `field="value"` | `field == "value"` |
| `field: 42` | `field=42` | `field == 42` |
| `field: true` / `field: false` | `field=true` / `field=false` | `field == true` / `field == false` |
| `field: null` | `NOT field=*` | `not exists(field)` |
| `field\|exists: true` | `field=*` | `exists(field)` |
| `field\|contains: x` | `field=*"x"*` | `contains(field, "x")` |
| `field\|startswith: x` | `field="x"*` | `starts_with(field, "x")` |
| `field\|endswith: x` | `field=*"x"` | `ends_with(field, "x")` |
| `field\|cased: X` | `field=CASE("X")` | `contains_cs(field, "X")` |
| `field\|re: pat` | `* \| where field =~ "pat"` | `matches(field, r"pat")` |
| `field\|cidr: 10.0.0.0/8` | `* \| where cidrmatch("10.0.0.0/8", field)` | `cidr_match("10.0.0.0/8", field)` |
| `field\|gte: 10` | `field>=10` | `field >= 10` |
| `field\|gt: 10` | `field>10` | `field > 10` |
| `field\|lte: 10` | `field<=10` | `field <= 10` |
| `field\|lt: 10` | `field<10` | `field < 10` |
| `field: [a, b, c]` | `field IN ("a", "b", "c")` | `field in ["a", "b", "c"]` |
| keyword `kw` | `"kw"` | `has(_raw, "kw")` |
| `selA and not selB` | parenthesized A with `AND NOT` B | parenthesized A with `and not` B |
| logsource to custom index | `FROM security \| search ...` | `from security \| where ...` |

## Pipeline-level migration

- `FROM main \| search <predicate>` becomes `from main \| where <expression>`.
  LynxFlow has no mid-pipeline `search` stage; all filtering happens in
  `where`.
- Comparison is `==`, never `=`. In LynxFlow expressions `=` binds a value, it
  does not compare.
- Function renames: `cidrmatch` → `cidr_match`, `match` / the `=~` operator →
  `matches` (with a raw-string `r"..."` pattern), search globs
  (`field=*"x"*`) → `contains` / `starts_with` / `ends_with`, `CASE("X")` →
  `contains_cs`.
- rsigma's `format=minimal` output omits the `FROM main | search` prefix and
  leaves a bare predicate. Migrate the predicate the same way and place it in
  a `where` stage of your own pipeline, as shown in the
  [cookbook](cookbook.md).

## Worked example

What rsigma emits today (legacy SPL2, not executable):

```spl
FROM main | search (FieldA="val1" AND NOT FieldB="val2") OR FieldC="val3"
```

Hand-migrated LynxFlow (this is the `and_or_not` golden from
`pkg/sigmaqueries/testdata/golden/`):

```
from main | where (FieldA == "val1" and not FieldB == "val2") or FieldC == "val3"
```

Every construct in the table above has a conformance-tested example in the
curated corpus; see the [compatibility contract](compat.md).
