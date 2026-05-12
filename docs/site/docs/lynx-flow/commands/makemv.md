---
title: makemv
description: Split a single-value field into multivalue values.
---

# makemv

Split a single-value field into multivalue values using a delimiter or tokenizer.

## Syntax

```spl
| makemv <field>
| makemv delim=<string> <field>
| makemv tokenizer=<regex> <field>
| makemv delim=<string> allowempty=<bool> <field>
```

The default delimiter is a single space.

## Examples

```spl
-- Split comma-separated tags
| makemv delim="," tags

-- Keep empty values between repeated delimiters
| makemv delim="," allowempty=true tags

-- Split using the first capture group of a regex
| makemv tokenizer="([^,]+),?" tags

-- Display the resulting values as one newline-delimited string
| makemv delim="," tags
| nomv tags
```

## Notes

- `delim` can be a multicharacter delimiter.
- `tokenizer` uses the first capturing group from each regex match.
- `allowempty=false` skips empty values. This is the default.
- `setsv` is parsed for compatibility but has no separate display effect in LynxDB's current single-value field model.

## See Also

- [nomv](/docs/lynx-flow/commands/nomv) -- Convert multivalue values to one newline-delimited value
- [mvexpand](/docs/lynx-flow/commands/mvexpand) -- Expand multivalue values into separate rows
