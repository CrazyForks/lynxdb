---
title: chart
description: Compute chart-style aggregations over one or two split fields.
---

# chart

Compute aggregations over events for chart-style tables.

## Syntax

```spl
| chart <agg-function> [AS <alias>] [BY <row-field>]
| chart <agg-function> [AS <alias>] OVER <row-field> BY <column-field>
| chart <agg-function> [AS <alias>] BY <row-field>, <column-field>
```

## Examples

```spl
-- Count by one field
| chart count by host

-- Pivot one aggregate by a split field
| chart count over host by status

-- Equivalent split syntax
| chart avg(duration_ms) by host,status
```

## Notes

- `chart <agg> by <field>` behaves like `stats <agg> by <field>`.
- With one aggregate and a column split, LynxDB pivots grouped results into chart-style columns.
- Extended Splunk chart options such as `limit`, `format`, `sep`, `cont`, and split-series filtering are not yet implemented.

## See Also

- [stats](/docs/lynx-flow/commands/stats) -- Grouped aggregation
- [xyseries](/docs/lynx-flow/commands/xyseries) -- Pivot grouped rows
- [timechart](/docs/lynx-flow/commands/timechart) -- Time-series aggregation
