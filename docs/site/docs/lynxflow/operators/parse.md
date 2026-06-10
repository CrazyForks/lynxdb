---
title: "parse"
sidebar_label: "parse"
---

# parse

**Class:** `core` &middot; **Streaming:** row-at-a-time

Schema-on-read extraction stage (RFC-002 §7). Never deletes columns; never silently overwrites non-null fields.

## Signature

```
| parse <format> [from=<field>] [into=<captures>] [prefix=<string>] [on_error=<enum>]
```

## Positional Arguments

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `format` | `format` | Yes | json, logfmt, kv(...), pattern "...", regex r"...", a named format, or first_of(f1, f2, ...) |

## Options

| Name | Type | Default | Description |
|------|------|---------|-------------|
| `from` | `field` | `_raw` | input field |
| `into` | `captures` | `-` | typed captures: into (status as int, dur as duration) |
| `prefix` | `string` | `-` | namespace prefix for extracted fields |
| `on_error` | `enum` | `propagate` | - Values: `propagate`, `null`, `drop`, `strict`. |

## Examples

```
parse json
```

```
parse first_of(json, logfmt)
```

```
parse regex r"user=(?<user>\w+)" into (user as string)
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
