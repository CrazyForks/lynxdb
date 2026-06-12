---
sidebar_position: 5
title: Your First LynxFlow Query
description: Learn LynxFlow basics -- search, filter, aggregate, and visualize log data.
---

# Your First LynxFlow Query

LynxFlow is LynxDB's query language. It's a pipeline language inspired by Splunk's SPL -- data flows left to right through pipe (`|`) operators.

## The Pipeline Concept

Every LynxFlow query is a pipeline of operators:

```
from <dataset> [search terms] | operator1 args | operator2 args | operator3 args
```

Data starts on the left (a `from` stage with optional search terms) and flows through each operator. Each operator transforms the data and passes it to the next.

## Step 1: Search

The simplest query is a keyword search attached to the `from` stage:

```spl
from main error
```

This finds all events containing the word "error". You can also search specific fields:

```spl
from main level=error
```

Combine terms with boolean operators:

```spl
from main level=error source=nginx
from main level=error OR level=warn
from main level=error NOT source=redis
```

:::tip
If your query omits the `from` stage, LynxDB reads from the default `main` dataset. So `stats count()` is equivalent to `from main | stats count()`. Note that search-term sugar like `level=error` only works in the `from` stage -- everywhere else, use `where` with `==`.
:::

## Step 2: Filter with WHERE

Use `where` for precise filtering. Inside `where`, `==` compares (a bare `=` binds values, so it is not a comparison):

```spl
from main source=nginx | where status >= 500
from main source=nginx | where status >= 500 and duration_ms > 1000
from main source=nginx | where contains(uri, "/api/")
```

## Step 3: Aggregate with STATS

`stats` computes aggregations. Aggregate calls always take parentheses -- `count()`, not `count`:

```spl
// Count events
from main | stats count()

// Count by field
from main level=error | stats count() by source

// Multiple aggregations
from main source=nginx | stats count(), avg(duration_ms), p99(duration_ms) by uri

// With renaming
from main source=nginx | stats count() as requests, avg(duration_ms) as avg_latency by uri
```

## Step 4: Sort and Limit

Alias `count()` with `as count` when later stages reference it:

```spl
// Sort descending (prefix with -)
from main source=nginx | stats count() as count by uri | sort -count

// Take top N
from main source=nginx | stats count() as count by uri | sort -count | head 10

// Or use the TOP shortcut
from main source=nginx | top 10 uri
```

## Step 5: Select Columns

```spl
// Pick specific fields
from main level=error | keep _time, source, message

// Remove fields
from main level=error | drop _raw
```

## Step 6: Transform with EXTEND

Create computed fields:

```spl
from main source=nginx
  | stats count() as total, count(where status >= 500) as errors by uri
  | extend error_rate = round(errors / total * 100, 1)
  | where error_rate > 5
  | sort -error_rate
  | keep uri, total, errors, error_rate
```

## Step 7: Time Series with EVERY

Aggregate over time buckets:

```spl
// Error count per 5-minute bucket
from main level=error | every 5m stats count()

// Error count by source per 5 minutes
from main level=error | every 5m by source stats count()
```

`every` is sugar for grouping by a time bucket -- `every 5m stats count()` desugars to `stats count() by bin(_time, 5m)`.

## Step 8: Extract Fields with PARSE

Extract new fields from raw text using regex:

```spl
from main "connection refused"
  | parse regex r"host=(?P<host>\S+) port=(?P<port>\d+)"
  | stats count() as count by host, port
  | sort -count
```

## Putting It All Together

Here's a real-world query that finds the slowest API endpoints with high error rates:

```spl
from main source=nginx
  | stats count() as total,
          count(where status >= 500) as errors,
          avg(duration_ms) as avg_latency,
          p99(duration_ms) as p99_latency
    by uri
  | extend error_rate = round(errors / total * 100, 1)
  | where error_rate > 5 or p99_latency > 1000
  | sort -error_rate
  | keep uri, total, errors, error_rate, avg_latency, p99_latency
```

## Operator Quick Reference

| Operator | What it does | Example |
|----------|-------------|---------|
| `from` | Choose dataset + search sugar | `from main "connection refused"` |
| `where` | Filter rows | `\| where status >= 500` |
| `stats` | Aggregate | `\| stats count(), avg(x) by y` |
| `extend` | Compute fields | `\| extend rate = errors / total * 100` |
| `sort` | Order results | `\| sort -count` |
| `head` | Limit results | `\| head 10` |
| `keep` | Select columns | `\| keep uri, count` |
| `drop` | Remove fields | `\| drop _raw` |
| `parse` | Extract via regex/json/logfmt | `\| parse regex r"host=(?P<host>\S+)"` |
| `every` | Time series | `\| every 5m stats count()` |
| `top` | Top N values | `\| top 10 uri` |
| `dedup` | Remove duplicates | `\| dedup host` |

## Next Steps

- **[LynxFlow v2 Reference](/docs/lynxflow/operators/from)** -- Full language reference
- **[Searching & Filtering](/docs/guides/search-and-filter)** -- Advanced search techniques
- **[Aggregations](/docs/guides/aggregations)** -- All aggregation functions
- **[STATS operator](/docs/lynxflow/operators/stats)** -- Detailed operator reference
