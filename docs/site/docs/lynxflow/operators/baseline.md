---
title: "baseline"
sidebar_label: "baseline"
---

# baseline

**Class:** `sugar` &middot; **Streaming:** row-at-a-time

Rolling baseline, delta, and z-score from previous rows.

## Signature

```
| baseline <field> window=<int> [by=<field_list>]
```

## Positional Arguments

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `field` | `field` | Yes | - |

## Options

| Name | Type | Default | Description |
|------|------|---------|-------------|
| `window` | `int` | `-` | - |
| `by` | `field_list` | `-` | - |

## Desugars To

```
streamstats current=false window=<n> avg(<f>) as baseline_<f>, stdev(<f>) as stdev_<f> [by <keys>] | extend delta_<f> = <f> - baseline_<f>, z_<f> = if(stdev_<f> > 0, delta_<f> / stdev_<f>, null)
```

## Examples

```
baseline error_rate window=12 by service
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
