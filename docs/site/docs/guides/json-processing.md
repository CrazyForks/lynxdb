---
title: Working with JSON Logs
description: How to parse, query, and transform JSON log data using parse json, object and array access, from_json/to_json, and explode.
---

# Working with JSON Logs

LynxDB provides multiple tools for working with JSON log data, from one-stage extraction to nested object access and array manipulation. This guide covers all the approaches and when to use each one.

## Quick reference

| Approach | Use when | Example |
|----------|----------|---------|
| **`\| parse json`** | Extracting all/some fields | `\| parse json \| stats count() as count by level` |
| **Object access `f.a.b`** | Querying nested values after extraction | `\| parse json \| where response.status >= 500` |
| **`\| parse json into (...)`** | Production pipelines with typed, projected captures | `\| parse json into (status as int)` |
| **`from_json()`** | Extracting one embedded JSON value in an extend/where | `\| extend payload = from_json(body)` |
| **`\| explode`** | Exploding JSON arrays into rows | `\| parse json \| explode items` |

---

## `parse json` -- extraction

The [`parse`](/docs/lynxflow/operators/parse) stage is LynxFlow's unified schema-on-read extractor (formerly the `json` and `unpack_json` commands in SPL2). `parse json` extracts all JSON keys from `_raw` (or a specified field) into columns:

```bash
# Extract all fields
cat app.json | lynxdb query '| parse json | stats count() as count by level'

# Extract specific fields only (faster for wide objects)
cat app.json | lynxdb query '| parse json into (level, status, duration_ms) | where status >= 500'

# Extract from a non-default field
cat logs.json | lynxdb query '| parse json from message | keep level, service'
```

### Typed captures

`into (...)` both projects the fields you want and coerces their types at extraction time:

```bash
lynxdb query 'from main | parse json into (level as string, status as int)'
```

### Prefixes

Add a prefix to avoid field name collisions:

```bash
lynxdb query 'from main | parse json prefix app_ | where app_level == "error"'
```

### Merge rules

`parse` stages never delete columns and never silently overwrite an existing non-null field — on collision the existing value wins and a per-query warning counter increments. There is no `keep_original=` option because the original field is always kept.

---

## Object and array access

After `parse json`, nested values are real objects and arrays. Use dot-notation for objects and `[index]` for arrays:

```bash
echo '{"level":"error","request":{"method":"POST","duration_ms":5012}}' \
  | lynxdb query '| where request.duration_ms > 1000 | extend method = request.method | keep level, method'
```

(Pipe mode auto-detects JSON input, so the nested `request` object is already a column here. For raw text containing JSON, add `| parse json` first.)

### Multi-level nesting

```spl
// Access deeply nested values
| parse json
| where response.headers.content_type == "application/json"
| extend origin = request.headers.origin
| stats count() as count by response.status
```

### Array elements

```spl
| parse json
| extend first_tag = tags[0]
| extend num_items = len(order.items)
| where num_items > 10
```

`len()` returns the element count of an array (formerly `json_array_length`); `keys()` returns the keys of an object (formerly `json_keys`).

### How field resolution works

When LynxFlow sees `request.method` in an expression, it resolves in order:

1. **Flat column** -- a column literally named `request.method` (e.g. produced by ingest-time extraction).
2. **Object access** -- the `method` key of the object column `request`.

Backticks force interpretation 1 (`` `request.method` ``); `(request).method` or `request["method"]` force interpretation 2.

:::warning Breaking change from SPL2
There is no implicit `_raw` JSON fallback anymore. If no stage produced the field, dotted access yields `missing` — it does not silently re-parse `_raw`. Add `| parse json` to extract structure.
:::

### Safe access with `?.`

Use `?.` when intermediate objects may be absent:

```bash
lynxdb query 'from main | parse json | extend origin = request?.headers?.origin'
```

---

## Single values: `from_json()`

When one field contains an embedded JSON string and you only need a value or two, `from_json()` (formerly `json_extract`) parses it into an object you can access directly:

```bash
lynxdb query 'from main | extend payload = from_json(body) | extend user = payload?.user?.name'
```

`from_json()` returns `null` on invalid JSON — never the original string. The strict variant `from_json!()` raises a query error instead.

### Build JSON output

Construct objects with `{...}` literals and serialize with `to_json()` (formerly `json_object`):

```bash
lynxdb query 'from main
  | stats count() as count by host, level
  | extend summary = to_json({host: host, level: level, count: count})
  | keep summary'
```

---

## Handling malformed JSON

Instead of pre-validating with `json_valid()`, LynxFlow's `parse` stage has explicit `on_error` modes:

```bash
# Drop rows that fail to parse
lynxdb query 'from main | parse json on_error drop'
```

The default mode is `propagate`: rows survive with best-effort fields, and the `_error` / `_error_detail` columns record what went wrong. Inspecting failures is a first-class workflow:

```bash
lynxdb query 'from main | parse json | where exists(_error) | keep _error, _error_detail, _raw'
```

Other modes: `null` (row survives, fields null, no error columns) and `strict` (the query fails on the first offending row). See the [parse reference](/docs/lynxflow/operators/parse).

---

## `explode` -- arrays into rows

When a field contains an array, [`explode`](/docs/lynxflow/operators/explode) (formerly `unroll`) creates one row per element:

```bash
echo '{"order":"ORD-1","items":[{"sku":"A1","qty":2},{"sku":"B3","qty":1}]}' \
  | lynxdb query '| parse json | explode items | extend sku = items.sku, qty = items.qty | keep order, sku, qty'
```

Output:

```
order    sku    qty
ORD-1    A1     2
ORD-1    B3     1
```

Rows with a missing or empty array are dropped.

### Array of objects

After `explode items`, each row's `items` column holds one element. Object elements are reached with dot-notation (`items.sku`, `items.qty`). Note that `keep` only accepts flat column names — materialize nested values with `extend` first, as in the example above.

### Array of scalars

Use `as` to name the element column:

```spl
// Input: {"name": "alice", "tags": ["admin", "user"]}
| parse json | explode tags as tag
// Row 1: name=alice, tag=admin
// Row 2: name=alice, tag=user
```

### Aggregate over exploded data

```bash
lynxdb query '| parse json
  | explode items
  | stats sum(items.qty) as total_sold, dc(order_id) as orders by items.sku
  | sort -total_sold
  | head 20'
```

---

## Chaining parsers

Real-world logs often have nested formats. Chain `parse` stages to handle them:

### Docker JSON logs with embedded application log

```bash
# Docker wraps each log line in JSON: {"log":"...","stream":"stdout","time":"..."}
# The inner "log" field contains the application logfmt output
cat docker-logs.json | lynxdb query '
  | parse json
  | parse logfmt from log prefix app_
  | where app_level == "error"
  | stats count() as count by app_service'
```

### Syslog with embedded JSON

```bash
# Syslog header wraps a JSON application message
cat syslog.log | lynxdb query '
  | parse syslog
  | parse json from message prefix app_
  | stats count() as count by hostname, app_level'
```

### Nginx access log with JSON body field

```bash
cat access.log | lynxdb query '
  | parse combined
  | parse json from request_body prefix body_
  | where body_action == "purchase"
  | stats sum(body_amount) as revenue by client_ip'
```

### Mixed-format streams

When lines may be JSON or logfmt, use a fallback chain — the first format that succeeds per row wins:

```bash
cat app.log | lynxdb query '| parse first_of(json, logfmt)'
```

---

## Performance tips

1. **Use `into (...)` for wide objects.** Extracting 3 fields from a 50-key JSON object is much faster than extracting all 50, and you get typed columns for free.

2. **Use `from_json()` in extend/where for single embedded values.** When only one field holds JSON, you avoid running a full parse stage.

3. **Place extraction early in the pipeline.** Extract before `where` so the filter operates on typed fields:
   ```spl
   | parse json into (status as int) | where status >= 500   // fast: status is an int
   ```

4. **Choose an `on_error` mode deliberately.** `on_error drop` keeps malformed data out of production pipelines; the default `propagate` keeps rows and lets you audit failures via `_error`.

---

## Next steps

- [parse reference](/docs/lynxflow/operators/parse)
- [explode reference](/docs/lynxflow/operators/explode)
- [Scalar functions reference](/docs/lynxflow/functions) -- object and array function sections
- [Field Extraction Guide](/docs/guides/field-extraction) -- parse regex, extend, and schema-on-read
