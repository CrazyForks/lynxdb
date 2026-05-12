---
title: mvexpand
description: Expand multivalue fields into separate rows.
---

# mvexpand

Expand one multivalue or array field into separate rows. Other fields are copied unchanged.

## Syntax

```spl
| mvexpand <field>
| mvexpand limit=<N> <field>
| mvexpand <field> limit=<N>
| expand <field>
```

`limit=0` or an omitted limit expands all values.

## Examples

```spl
-- Expand every tag into its own row
| mvexpand tags

-- Expand only the first two values per row
| mvexpand limit=2 tags

-- SPL2 array spelling
| expand records
```

## Notes

- LynxDB treats `mvexpand`, `expand`, and `unroll` as the same row expansion operation.
- Non-array, null, and missing fields pass through unchanged.
- Only one field is expanded by `mvexpand`. Use `explode a, b` for LynxFlow zip-expansion of parallel arrays.

## See Also

- [unroll](/docs/lynx-flow/commands/unroll) -- LynxDB array expansion
- [json](/docs/lynx-flow/commands/json-cmd) -- Extract JSON fields before expanding
