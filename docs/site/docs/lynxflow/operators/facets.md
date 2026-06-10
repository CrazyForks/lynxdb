---
title: "facets"
sidebar_label: "facets"
---

# facets

**Class:** `sugar` &middot; **Streaming:** accumulating

Top values per requested field in one result (_facet/_value/count columns).

## Signature

```
| facets <fields> [limit=<int>]
```

## Positional Arguments

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `fields` | `field_list` | Yes | - |

## Options

| Name | Type | Default | Description |
|------|------|---------|-------------|
| `limit` | `int` | `10` | - |

## Desugars To

```
union of per-field `stats count() as count by <f> | sort -count | head <limit> | extend _facet = "<f>", _value = string(<f>) | keep _facet, _value, count`
```

## Examples

```
facets service, host limit=5
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
