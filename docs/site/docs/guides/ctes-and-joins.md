---
title: CTEs, Joins, and Subsearches
description: How to use let-bound Common Table Expressions (CTEs), join, union, and transaction in LynxDB for cross-source correlation and complex analysis.
---

# CTEs, Joins, and Subsearches

When a single pipeline is not enough, LynxDB supports `let`-bound Common Table Expressions (CTEs), the [`join`](/docs/lynxflow/operators/join) stage, and [`union`](/docs/lynxflow/operators/union) for combining data from multiple sources or running multi-step analysis.

## Common Table Expressions (CTEs)

CTEs let you define named result sets and reference them later in the query. This is the most powerful way to build complex multi-source queries.

### CTE syntax

Define a CTE with `let $name = <pipeline>;` and reference it with `from $name`:

```spl
let $threats = from idx_backend | where threat_type in ["sqli", "path_traversal"] | keep client_ip, threat_type;
let $logins = from idx_audit | where type == "USER_LOGIN" and res == "failed" | stats count() as failures by src_ip | rename src_ip as client_ip;
from $threats | join type=inner on client_ip with $logins | keep client_ip, threat_type, failures
```

### CTE rules

- CTEs are introduced with the `let` keyword (SPL2's bare `$x = ...;` form is gone).
- CTE names start with `$` and are assigned with `=`.
- Each CTE ends with a semicolon `;`.
- CTEs are evaluated in order, from top to bottom, and can reference earlier CTEs.
- The final query (after all CTE definitions) produces the result.
- `let` exists only at query level — there is no `| let` stage (use [`extend`](/docs/lynxflow/operators/extend) for per-row bindings).

### Example: security correlation

Find IPs that triggered threat detection AND had failed logins:

```bash
lynxdb query '
  let $threats = from main
    | where threat_type in ["sqli", "path_traversal"]
    | keep client_ip, threat_type;
  let $failed_logins = from main
    | where type == "USER_LOGIN" and res == "failed"
    | stats count() as failures by src_ip
    | rename src_ip as client_ip;
  from $threats
    | join type=inner on client_ip with $failed_logins
    | where failures > 5
    | keep client_ip, threat_type, failures
    | sort -failures'
```

Note the `rename src_ip as client_ip` — join keys must carry the same name on both sides.

### Example: compare two time periods

Bracket time ranges on `from` make period comparisons direct:

```bash
lynxdb query '
  let $current = from main[-1h]
    | where level == "error"
    | stats count() as current_errors by source;
  let $previous = from main[-2h..-1h]
    | where level == "error"
    | stats count() as previous_errors by source;
  from $current
    | join type=outer on source with $previous
    | extend change_pct = round((current_errors - previous_errors) / previous_errors * 100, 1)
    | keep source, current_errors, previous_errors, change_pct
    | sort -change_pct'
```

---

## join

The [`join`](/docs/lynxflow/operators/join) stage combines events from two datasets based on shared fields.

### Inner join

Keep only events that have a match in both datasets:

```bash
lynxdb query 'from nginx
  | join type=inner on client_ip with [
      from main | where source == "auth" and type == "login"
      | keep client_ip, user_id
    ]
  | keep client_ip, user_id, uri, status'
```

### Left join

Keep all events from the left side, with null values for unmatched right-side fields (this is what SPL2's `type=outer` did):

```bash
lynxdb query 'from nginx
  | join type=left on client_ip with [
      from main | where source == "geo"
      | keep client_ip, country, city
    ]
  | keep client_ip, uri, country, city'
```

### Join syntax

```
| join [type=inner|left|outer] on <field>[, <field>] with ($cte | [<pipeline>])
```

| Parameter | Description |
|-----------|-------------|
| `type` | `inner` (only matches, the default — a plain inner join, never innerunique), `left` (keep all from primary), or `outer` (full outer join) |
| `on` | The field(s) to join on (must exist in both datasets) |
| `with` | The secondary dataset: a `$cte` reference or a `[<pipeline>]` in square brackets |

### Join on multiple fields

When you need to join on multiple fields, list them separated by commas:

```bash
lynxdb query 'from nginx
  | join type=inner on host, timestamp with [
      from metrics
      | keep host, timestamp, cpu_pct, mem_pct
    ]
  | keep host, timestamp, uri, cpu_pct, mem_pct'
```

---

## union

The [`union`](/docs/lynxflow/operators/union) stage appends rows from one or more sub-pipelines to the current result set. It replaces both `APPEND` and `MULTISEARCH` from SPL2. Schemas merge by name, with null-padding for columns that only exist on one side:

```bash
lynxdb query 'from nginx status>=500 | stats count() as errors by uri
  | union [
      from nginx | stats count() as total by uri
    ]'
```

### Use case: combine different aggregations

When you need several aggregations that cannot be combined in a single `stats`:

```bash
lynxdb query 'from main | stats count() as total_events
  | union [
      from main | where level == "error" | stats count() as total_errors
    ]
  | union [
      from nginx | where status >= 500 | stats count() as nginx_5xx
    ]'
```

### Use case: run several independent analyses (formerly MULTISEARCH)

Tag each branch with `extend` so the merged rows stay distinguishable:

```bash
lynxdb query 'from nginx status>=500 | stats count() as errors, avg(duration_ms) as avg_lat | extend service = "nginx"
  | union [from `api-gateway` level=error | stats count() as errors, avg(duration_ms) as avg_lat | extend service = "api-gw"]
  | union [from postgres duration_ms>1000 | stats count() as errors, avg(duration_ms) as avg_lat | extend service = "postgres"]
  | keep service, errors, avg_lat
  | sort -errors'
```

A single `union` also accepts multiple sub-pipelines separated by commas: `union [<pipeline>], [<pipeline>]`.

---

## transaction

The [`transaction`](/docs/lynxflow/operators/transaction) stage groups events into transactions (sequences of related events) keyed by shared field values:

```bash
lynxdb query 'from `api-gateway`
  | transaction session_id startswith=has(_raw, "request started") endswith=has(_raw, "request completed")
  | keep session_id, duration, eventcount'
```

Each transaction row carries the grouping key plus a `duration` and an `eventcount` field. `maxspan=<duration>` caps how long a transaction may run.

Transactions are useful for:

- Grouping request start/end events into sessions
- Computing end-to-end latency across multiple log lines
- Finding incomplete transactions (missing end event)

For time-gap-based session grouping, see the [`sessionize`](/docs/lynxflow/operators/sessionize) helper:

```bash
lynxdb query 'from main | sessionize maxpause=30m by user_id'
```

---

## Practical patterns

### Find users hitting rate limits AND generating errors

```bash
lynxdb query '
  let $rate_limited = from main
    | where source == "api-gateway" and status == 429
    | stats count() as rate_limit_hits by user_id;
  let $errors = from main
    | where source == "api-gateway" and status >= 500
    | stats count() as error_count by user_id;
  from $rate_limited
    | join type=inner on user_id with $errors
    | where rate_limit_hits > 10 and error_count > 5
    | keep user_id, rate_limit_hits, error_count
    | sort -rate_limit_hits'
```

### Enrich nginx logs with geo data

```bash
lynxdb query 'from nginx[-1h] status>=500
  | join type=left on client_ip with [
      from main | where source == "geoip"
      | dedup client_ip
      | keep client_ip, country, city
    ]
  | stats count() as count by country, city
  | sort -count
  | head 20'
```

### Compare error rates across services

```bash
lynxdb query 'from main | where source == "nginx" | stats count() as total, count(where status >= 500) as errors | extend service = "nginx"
  | union [from main | where source == "api-gateway" | stats count() as total, count(where level == "error") as errors | extend service = "api-gw"]
  | union [from main | where source == "postgres" | stats count() as total, count(where level == "error") as errors | extend service = "postgres"]
  | extend error_rate = round(errors / total * 100, 2)
  | keep service, total, errors, error_rate
  | sort -error_rate'
```

---

## Performance considerations

- **join**: The right side (`with [...]` or `$cte`) is loaded into memory for the hash join. Keep it small by filtering and aggregating before joining. Avoid joining two large unfiltered datasets.
- **CTEs**: Each CTE is evaluated independently. Use filters and aggregations in CTEs to reduce intermediate result sizes.
- **union**: Rows stream through; the cost is the sum of the sub-pipelines. Keep each branch as narrow as possible (`keep` only the columns you need).

---

## Next steps

- [Search and filter logs](/docs/guides/search-and-filter) -- write effective filters for sub-pipelines
- [Run aggregations](/docs/guides/aggregations) -- build aggregations for CTE pipelines
- [join reference](/docs/lynxflow/operators/join) -- full join syntax and options
- [union reference](/docs/lynxflow/operators/union) -- full union syntax
- [transaction reference](/docs/lynxflow/operators/transaction) -- full transaction syntax
