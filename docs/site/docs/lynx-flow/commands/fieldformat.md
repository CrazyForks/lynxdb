---
title: fieldformat
description: Format field values for display without changing row values.
---

# fieldformat

Validate display-only formatting for a field.

## Syntax

```spl
| fieldformat <field>=<eval-expression>
```

## Examples

```spl
-- Format a count for display
| fieldformat totalCount=tostring(totalCount, "commas")

-- Format a renamed timestamp-style field
| fieldformat "First Event"=strftime(firstTime, "%c")
```

## Notes

- `fieldformat` keeps the underlying field value unchanged. Use `eval` when exported values must change.
- Only one field/expression pair is accepted per command. Use multiple `fieldformat` commands for multiple fields.
- LynxDB currently parses the eval expression and preserves the stage, but result rows do not yet carry separate render metadata.

## See Also

- [eval](/docs/lynx-flow/commands/eval) -- Change the underlying field value
- [table](/docs/lynx-flow/commands/table) -- Select displayed fields
