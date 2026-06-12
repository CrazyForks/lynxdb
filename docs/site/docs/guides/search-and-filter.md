---
title: Search and Filter Logs
description: How to search and filter logs in LynxDB using full-text search, field-value filters, boolean operators, wildcards, and the WHERE operator.
---

# Search and Filter Logs

LynxDB provides multiple ways to find the events you need: full-text search across raw log text, field-value filters, boolean operators, wildcards, and the `where` operator for typed expressions. This guide shows how to use each technique.

## Full-text search

The simplest way to find events is to search for text that appears anywhere in the raw log line. Search terms attach directly to the [`from`](/docs/lynxflow/operators/from) stage.

### Search for a keyword

```bash
lynxdb query 'from main "connection refused"'
```

A quoted phrase in the `from` stage scans the `_raw` field of every event. LynxDB uses an FST-based inverted index with bloom filters, so full-text search is fast even over millions of events.

### Search for multiple terms

Bare words separated by spaces are ANDed together:

```bash
lynxdb query 'from main timeout redis'
```

This returns events containing both "timeout" and "redis".

### Search mid-pipeline

Search sugar only works in the `from` stage. Later in the pipeline, use the full-text functions `has()`, `contains()`, `matches()`, or `glob()` inside `where`:

```bash
lynxdb query 'from main | where has(_raw, "refused")'
```

---

## Field-value filters

If your events have structured fields (JSON logs, or fields extracted at ingest time), filter directly on field values in the `from` stage.

### Exact match

```bash
lynxdb query 'from main level=error'
lynxdb query 'from main _source=nginx'
lynxdb query 'from main host="web-01"'
```

:::note
The `key=value` form is search sugar that only works in the `from` stage -- there, `=` means "match". Everywhere else in the pipeline, `==` compares and `=` binds.
:::

### Numeric comparison

```bash
lynxdb query 'from main status>=500'
lynxdb query 'from main duration_ms>1000'
lynxdb query 'from main status!=200'
```

### Combine multiple filters

Multiple field-value pairs are ANDed together:

```bash
lynxdb query 'from main _source=nginx status>=500'
```

This returns events where `_source` is "nginx" AND `status` is 500 or above.

---

## Boolean operators

Use `AND`, `OR`, and `NOT` for more complex filter logic. These work in both the `from`-stage search sugar and the [`where`](/docs/lynxflow/operators/where) operator.

### In the search expression

```bash
lynxdb query 'from main level=error OR level=warn'
lynxdb query 'from main _source=nginx NOT status=200'
lynxdb query 'from main (level=error OR level=warn) _source=nginx'
```

### In a WHERE clause

The `where` operator supports full boolean logic with typed expressions:

```bash
lynxdb query 'where level == "error" or level == "warn"'
lynxdb query 'where status >= 500 and _source == "nginx"'
lynxdb query 'where not (status >= 200 and status < 300)'
```

---

## Wildcards

Use a trailing `*` as a wildcard in `from`-stage field-value filters:

```bash
# Match any host starting with "web-"
lynxdb query 'from main host=web-*'
```

For leading or inner wildcards, use the `glob()` function inside `where`:

```bash
# Match any source ending in "-gateway"
lynxdb query 'where glob(_source, "*-gateway")'

# Match paths containing "api"
lynxdb query 'where glob(path, "*api*")'
```

For more complex pattern matching, use the `matches()` function with a regex:

```bash
lynxdb query 'where matches(path, r"^/api/v\d+")'
```

See the [functions reference](/docs/lynxflow/functions) for details on `glob()` and `matches()`.

---

## The WHERE operator

[`where`](/docs/lynxflow/operators/where) is the primary filtering operator in LynxFlow pipelines. It evaluates a typed boolean expression and keeps only events where the expression is true. Remember: `==` compares, `=` binds.

### Basic filtering

```bash
lynxdb query 'where level == "error"'
lynxdb query 'where status >= 500'
lynxdb query 'where duration_ms > 1000 and _source == "nginx"'
```

### Using functions in WHERE

```bash
# Case-insensitive match
lynxdb query 'where lower(level) == "error"'

# Regular expression match
lynxdb query 'where matches(message, r"timeout|refused")'

# Null checks
lynxdb query 'where exists(user_id)'
lynxdb query 'where is_null(response_code)'
```

### Filtering after aggregation

`where` can appear anywhere in the pipeline, including after `stats`:

```bash
lynxdb query 'from main _source=nginx | stats count() as count by uri | where count > 100'
```

Result:

| uri | count |
|-----|-------|
| /api/v2/users | 1423 |
| /api/v1/health | 891 |
| /api/v1/login | 456 |

---

## The FROM stage

Use [`from`](/docs/lynxflow/operators/from) to query a specific index:

```bash
lynxdb query 'from production | where level == "error" | stats count() by service'
```

When you omit `from`, LynxDB queries the default `main` index -- `where level == "error"` is equivalent to `from main | where level == "error"`.

---

## Time range filters

Narrow your search to a specific time window.

### Relative time

```bash
lynxdb query 'from main level=error' --since 1h
lynxdb query 'from main level=error' --since 15m
lynxdb query 'from main level=error' --since 7d
```

You can also attach the time range to the `from` stage with brackets:

```bash
lynxdb query 'from main[-1h] level=error'
```

### Absolute time

```bash
lynxdb query 'from main level=error' \
  --from 2026-01-15T00:00:00Z \
  --to 2026-01-15T23:59:59Z
```

See the [`from` reference](/docs/lynxflow/operators/from) for all supported time range formats.

---

## Limiting results

### Head and tail

Use [`head`](/docs/lynxflow/operators/head) to return only the first N results:

```bash
lynxdb query 'from main _source=nginx status>=500 | head 10'
```

Result:

| _time | status | uri | duration_ms |
|-------|--------|-----|-------------|
| 2026-01-15T14:23:01Z | 502 | /api/v2/users | 3421 |
| 2026-01-15T14:22:58Z | 500 | /api/v1/health | 1205 |

Use [`tail`](/docs/lynxflow/operators/tail) for the last N:

```bash
lynxdb query 'from main _source=nginx | tail 5'
```

### Dedup

Remove duplicate events based on a field with [`dedup`](/docs/lynxflow/operators/dedup):

```bash
lynxdb query 'from main level=error | dedup host'
```

---

## Selecting and renaming fields

### KEEP -- pick specific columns

```bash
lynxdb query 'from main level=error | keep _time, _source, message'
```

Result:

| _time | _source | message |
|-------|---------|---------|
| 2026-01-15T14:23:01Z | nginx | upstream timed out |
| 2026-01-15T14:22:55Z | api-gw | connection refused |

### DROP -- exclude fields

```bash
lynxdb query 'from main level=error | drop _raw'
```

### RENAME -- change field names

```bash
lynxdb query 'from main | stats count() as count by source | rename count as total_events'
```

See the [`keep`](/docs/lynxflow/operators/keep), [`drop`](/docs/lynxflow/operators/drop), and [`rename`](/docs/lynxflow/operators/rename) operator references.

---

## Searching local files (no server)

All the search techniques above work in pipe mode and file mode:

```bash
# Search a local file
lynxdb query --file access.log 'where status >= 500 | head 20'

# Search stdin
cat /var/log/syslog | lynxdb query 'where level == "ERROR" | stats count() by service'

# Search multiple files with glob
lynxdb query --file '/var/log/nginx/*.log' 'from main status>=500 | top 10 uri'
```

See the [pipe mode guide](/docs/getting-started/pipe-mode) for details.

---

## Quick-access shortcuts

LynxDB provides shortcut commands for common search patterns:

```bash
# Quick count
lynxdb count 'where level == "error"' --since 1h

# Peek at data shape
lynxdb sample 5 'where _source == "nginx"'

# See field catalog
lynxdb fields status --values
```

See the [CLI shortcuts reference](/docs/cli/shortcuts) for the full list.

---

## Next steps

- [Run aggregations](/docs/guides/aggregations) -- compute statistics from your filtered events
- [Extract fields at query time](/docs/guides/field-extraction) -- parse unstructured logs with `parse` and `extend`
- [LynxFlow search syntax](/docs/lynxflow/operators/from) -- full reference for `from`-stage search expressions
- [WHERE operator](/docs/lynxflow/operators/where) -- complete `where` syntax and examples
