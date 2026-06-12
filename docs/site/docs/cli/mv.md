---
sidebar_position: 7
title: mv (Materialized Views)
description: Create, manage, pause, and drop materialized views with the LynxDB CLI.
---

# mv (Materialized Views)

Manage materialized views -- precomputed aggregations that accelerate repeated queries.

```
lynxdb mv <subcommand>
```

## Subcommands

| Subcommand | Description |
|------------|-------------|
| `create <name> <query>` | Create a materialized view |
| `list` | List materialized views |
| `status <name>` | Show view status and backfill progress |
| `backfill <name>` | Manually trigger a backfill |
| `pause <name>` | Pause a view pipeline |
| `resume <name>` | Resume a paused view pipeline |
| `drop <name>` | Drop a materialized view |

## mv create

Create a new materialized view from a LynxFlow aggregation query.

```
lynxdb mv create <name> <query> [--retention <duration>]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--retention` | | Retention period (e.g., `30d`, `90d`) |

### Examples

```bash
# Create a view for error counts by host
lynxdb mv create errors_by_host \
  'from main | where level == "ERROR" | stats count() by host'

# With retention
lynxdb mv create daily_summary \
  'from main | stats count() by source' --retention 90d

# Time-bucketed view for repeated aggregations
lynxdb mv create errors_5m \
  'from main | where level == "ERROR" | stats count(), avg(duration) by source, bin(_time, 5m)' \
  --retention 90d

# Cascading view (build on top of another view)
lynxdb mv create errors_1h \
  'from errors_5m | stats sum(count) as count by source, bin(_time, 1h)' \
  --retention 365d
```

The view begins backfilling automatically after creation. Queries that match the view pattern are automatically accelerated.

---

## mv list

List all materialized views.

```
lynxdb mv list
```

Supports `--format json`.

### Console Output

```
NAME            STATUS       QUERY
mv_errors_5m    active       from main level=error | stats count(), avg(dur...
mv_5xx_hourly   backfilling  from main source=nginx status>=500 | stats cou...
```

---

## mv status

Show detailed status for a specific view.

```
lynxdb mv status <name>
```

Tab-completes view names. Supports `--format json`.

### Console Output

```
Name:       mv_errors_5m
Status:     active
Query:      from main level=error | stats count(), avg(duration) by source, bin(...)
Retention:  90d
```

---

## mv drop

Drop a materialized view and its stored data.

```
lynxdb mv drop <name> [--force] [--dry-run]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--force` | `false` | Skip confirmation prompt |
| `--dry-run` | `false` | Show what would be deleted without applying |

### Examples

```bash
# Drop with confirmation prompt
lynxdb mv drop errors_by_host

# Skip confirmation
lynxdb mv drop errors_by_host --force

# Preview what would be deleted
lynxdb mv drop errors_by_host --dry-run
```

---

## mv pause

Pause a materialized view pipeline. The view stops processing new data but retains its existing computed data.

```
lynxdb mv pause <name>
```

```bash
lynxdb mv pause errors_5m
```

---

## mv resume

Resume a paused materialized view pipeline. The view catches up on data ingested while it was paused.

```
lynxdb mv resume <name>
```

```bash
lynxdb mv resume errors_5m
```

---

## mv backfill

Manually trigger a backfill for an existing materialized view.

```
lynxdb mv backfill <name>
```

```bash
lynxdb mv backfill errors_5m
```

## How Acceleration Works

When you run a query that matches a materialized view, LynxDB automatically rewrites the query to read from the view instead of scanning raw data:

```bash
# This query:
lynxdb query 'from main level=error | stats count() by source'

# Is automatically accelerated by mv_errors_5m if it matches
# Response metadata shows:
#   meta.accelerated_by: {view: mv_errors_5m, speedup: "~400x"}
```

## See Also

- [query](/docs/cli/query) for running queries that can be accelerated by views
- [Server](/docs/cli/server) for server configuration that affects view processing
