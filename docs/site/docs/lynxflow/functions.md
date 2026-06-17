---
title: "Scalar Functions"
sidebar_label: "Scalar Functions"
---

# Scalar Functions

All scalar functions available in LynxFlow expressions. Functions marked **null_on_failure** return `null` when the input is invalid; those with a **strict variant** (`name!`) raise a query error instead.

## Conversion

| Function | Params | Result | Fallibility | Strict | Description |
|----------|--------|--------|-------------|--------|-------------|
| `int` | (x: any) | `int` | null_on_failure | `int!` | Cast to int; null on failure. |
| `float` | (x: any) | `float` | null_on_failure | `float!` | Cast to float; null on failure. |
| `string` | (x: any) | `string` | infallible | - | Render any value as a string. |
| `bool` | (x: any) | `bool` | null_on_failure | `bool!` | Cast to bool; null on failure. |
| `timestamp` | (x: any, layout: string?) | `timestamp` | null_on_failure | `timestamp!` | Parse RFC3339 (or layout) to timestamp; null on failure. |
| `duration` | (x: string) | `duration` | null_on_failure | `duration!` | Parse a duration string ("100ms", "5m"); numbers use n * 1ms instead. |

## Conditional

| Function | Params | Result | Fallibility | Strict | Description |
|----------|--------|--------|-------------|--------|-------------|
| `if` | (cond: bool, then: any, else: any) | `any` | infallible | - | Null condition yields null. |
| `case` | (pairs: any...) | `any` | infallible | - | case(cond1, v1, cond2, v2, ...[, default]); trailing odd argument is the default. |
| `remap` | (x: any, from: array, to: array, default: any?) | `any` | null_on_failure | - | Map x through parallel from/to arrays; default is null. |
| `coalesce` | (values: any...) | `any` | infallible | - | First non-null, non-missing argument. |
| `nullif` | (a: any, b: any) | `any` | infallible | - | Null when a == b, else a. |
| `exists` | (field: any) | `bool` | infallible | - | True when the field is present with a non-null value. |
| `is_null` | (field: any) | `bool` | infallible | - | True when present with an explicit null value. |
| `is_missing` | (field: any) | `bool` | infallible | - | True when the field was never extracted. |
| `typeof` | (x: any) | `string` | infallible | - | Type name: string, int, float, bool, timestamp, duration, array, object, null, missing. |

## String

| Function | Params | Result | Fallibility | Strict | Description |
|----------|--------|--------|-------------|--------|-------------|
| `len` | (x: any) | `int` | null_on_failure | - | Length of a string (runes) or array (elements). |
| `lower` | (s: string) | `string` | null_on_failure | - | - |
| `upper` | (s: string) | `string` | null_on_failure | - | - |
| `trim` | (s: string, chars: string?) | `string` | null_on_failure | - | - |
| `ltrim` | (s: string, chars: string?) | `string` | null_on_failure | - | - |
| `rtrim` | (s: string, chars: string?) | `string` | null_on_failure | - | - |
| `substr` | (s: string, start: int, len: int?) | `string` | null_on_failure | - | 0-based start; negative counts from end. |
| `replace` | (s: string, pattern: regex, with: string) | `string` | null_on_failure | - | Regex replace all. |
| `replace_first` | (s: string, pattern: regex, with: string) | `string` | null_on_failure | - | Regex replace first match. |
| `split` | (s: string, sep: string) | `array` | null_on_failure | - | - |
| `split_regex` | (s: string, pattern: regex) | `array` | null_on_failure | - | Split a string by regex matches. |
| `split_part` | (s: string, sep: string, n: int) | `string` | null_on_failure | - | Return the 0-based split segment, or null when out of range. |
| `index_of` | (x: any, needle: any) | `int` | null_on_failure | - | 0-based index in a string or array; null when not found. |
| `count_substr` | (s: string, sub: string) | `int` | null_on_failure | - | Count non-overlapping substring occurrences. |
| `count_matches` | (s: string, pattern: regex) | `int` | null_on_failure | - | Count regex matches. |
| `lpad` | (s: string, n: int, char?: string) | `string` | null_on_failure | - | Left-pad a string to n runes; pad defaults to space. |
| `rpad` | (s: string, n: int, char?: string) | `string` | null_on_failure | - | Right-pad a string to n runes; pad defaults to space. |
| `repeat` | (s: string, n: int) | `string` | null_on_failure | - | Repeat a string n times. |
| `translate` | (s: string, from: string, to: string) | `string` | null_on_failure | - | Replace characters from one set with characters from another. |
| `levenshtein` | (a: string, b: string, damerau: bool?) | `int` | null_on_failure | - | Edit distance between two strings. |
| `jaro_winkler` | (a: string, b: string) | `float` | null_on_failure | - | Jaro-Winkler similarity between two strings. |
| `join` | (arr: array, sep: string) | `string` | null_on_failure | - | - |
| `starts_with` | (s: string, prefix: string) | `bool` | null_on_failure | - | - |
| `ends_with` | (s: string, suffix: string) | `bool` | null_on_failure | - | - |
| `printf` | (format: string, args: any...) | `string` | null_on_failure | - | - |
| `urldecode` | (s: string) | `string` | null_on_failure | - | - |
| `urlencode` | (s: string) | `string` | null_on_failure | - | Encode a string for URL query components. |
| `regex_escape` | (s: string) | `string` | null_on_failure | - | Escape a string for literal use in a regex pattern. |
| `normalize_utf8` | (s: string, form: string?) | `string` | null_on_failure | - | Normalize Unicode text as nfc, nfd, nfkc, or nfkd. |
| `punycode_encode` | (s: string) | `string` | null_on_failure | - | Encode a domain name with IDNA punycode. |
| `punycode_decode` | (s: string) | `string` | null_on_failure | - | Decode an IDNA punycode domain name. |
| `base64_decode` | (s: string) | `string` | null_on_failure | `base64_decode!` | Decode standard base64; null on invalid input. |
| `base64_encode` | (s: string) | `string` | null_on_failure | - | Encode a string as standard base64. |
| `hex` | (s: string) | `string` | null_on_failure | - | Encode a string as lowercase hexadecimal. |
| `unhex` | (s: string) | `string` | null_on_failure | `unhex!` | Decode hexadecimal bytes; null on invalid input. |
| `url_parse` | (s: string) | `object` | null_on_failure | - | Parse a URL into &#123;scheme, host, port, path, query, fragment&#125;. |
| `url_domain` | (s: string) | `string` | null_on_failure | - | Parse a URL and return its host. |
| `url_etld1` | (s: string) | `string` | null_on_failure | - | Return the registrable domain using the public suffix list. |
| `url_tld` | (s: string) | `string` | null_on_failure | - | Return the public suffix using the public suffix list. |
| `url_path` | (s: string) | `string` | null_on_failure | - | Parse a URL and return its path. |
| `url_protocol` | (s: string) | `string` | null_on_failure | - | Parse a URL and return its scheme/protocol. |
| `url_param` | (s: string, name: string) | `string` | null_on_failure | - | Return the first URL query parameter value by name. |
| `url_strip_query` | (s: string, fragment: bool?) | `string` | null_on_failure | - | Remove the URL query string; optionally remove fragment too. |
| `path_normalize` | (s: string) | `string` | null_on_failure | - | - |
| `useragent_parse` | (s: string) | `object` | null_on_failure | - | Optional build. |

## Search

| Function | Params | Result | Fallibility | Strict | Description |
|----------|--------|--------|-------------|--------|-------------|
| `has` | (field: string, term: string) | `bool` | null_on_failure | - | Whole-token match, always case-insensitive; FST term index. Fast. |
| `contains` | (field: string, sub: string) | `bool` | null_on_failure | - | Substring, case-insensitive; bloom-assisted scan. Moderate. |
| `contains_cs` | (field: string, sub: string) | `bool` | null_on_failure | - | Case-sensitive substring. |
| `has_any` | (field: string, terms: array) | `bool` | null_on_failure | - | True when any term is present as a whole token. |
| `has_all` | (field: string, terms: array) | `bool` | null_on_failure | - | True when all terms are present as whole tokens. |
| `contains_phrase` | (field: string, phrase: string) | `bool` | null_on_failure | - | Case-insensitive phrase substring match. |
| `contains_any` | (field: string, subs: array) | `bool` | null_on_failure | - | Case-insensitive match for any substring. |
| `contains_any_cs` | (field: string, subs: array) | `bool` | null_on_failure | - | Case-sensitive match for any substring. |
| `matches_any` | (s: string, patterns: array) | `bool` | null_on_failure | - | True when any regex pattern matches. |
| `glob` | (field: string, pattern: string) | `bool` | null_on_failure | - | Glob match, case-sensitive; literal-prefix extraction when possible. |
| `has_glob` | (field: string, pattern: string) | `bool` | null_on_failure | - | Whole-token glob match (*, ?, \-escapes), always case-insensitive; FST term-dictionary expansion. Moderate. |
| `tokens` | (s: string) | `array` | null_on_failure | - | Return lowercase tokenizer-contract tokens. |

## Regex

| Function | Params | Result | Fallibility | Strict | Description |
|----------|--------|--------|-------------|--------|-------------|
| `matches` | (s: string, pattern: regex) | `bool` | null_on_failure | - | Regex match (linear-time engine). Slow tier; (?i) for case-insensitive. |
| `extract` | (s: string, pattern: regex) | `string` | null_on_failure | - | First capture group. |
| `extract_all` | (s: string, pattern: regex) | `array` | null_on_failure | - | - |
| `extract_groups` | (s: string, pattern: regex) | `object` | null_on_failure | - | Return named regex capture groups as an object. |

## Math

| Function | Params | Result | Fallibility | Strict | Description |
|----------|--------|--------|-------------|--------|-------------|
| `abs` | (x: number) | `number` | null_on_failure | - | - |
| `round` | (x: number, digits: int?) | `float` | null_on_failure | - | - |
| `floor` | (x: number) | `int` | null_on_failure | - | - |
| `ceil` | (x: number) | `int` | null_on_failure | - | - |
| `sqrt` | (x: number) | `float` | null_on_failure | - | - |
| `ln` | (x: number) | `float` | null_on_failure | - | - |
| `log` | (x: number, base: number?) | `float` | null_on_failure | - | Base 10 by default. |
| `exp` | (x: number) | `float` | null_on_failure | - | - |
| `pow` | (x: number, y: number) | `float` | null_on_failure | - | - |
| `clamp` | (x: number, lo: number, hi: number) | `number` | null_on_failure | - | - |
| `bucket` | (x: number, bounds: array) | `number` | null_on_failure | - | Snap x to the largest bound &lt;= x. |
| `sin` | (x: number) | `float` | null_on_failure | - | - |
| `cos` | (x: number) | `float` | null_on_failure | - | - |
| `tan` | (x: number) | `float` | null_on_failure | - | - |
| `asin` | (x: number) | `float` | null_on_failure | - | - |
| `acos` | (x: number) | `float` | null_on_failure | - | - |
| `atan` | (x: number) | `float` | null_on_failure | - | - |
| `atan2` | (y: number, x: number) | `float` | null_on_failure | - | - |
| `is_finite` | (x: number) | `bool` | null_on_failure | - | True for numeric values that are neither NaN nor infinite. |
| `is_nan` | (x: number) | `bool` | null_on_failure | - | True for numeric NaN values. |
| `bit_and` | (a: int, b: int) | `int` | null_on_failure | - | Bitwise AND for integers. |
| `bit_or` | (a: int, b: int) | `int` | null_on_failure | - | Bitwise OR for integers. |
| `bit_xor` | (a: int, b: int) | `int` | null_on_failure | - | Bitwise XOR for integers. |
| `bit_shl` | (a: int, n: int) | `int` | null_on_failure | - | Shift an integer left by n bits. |
| `bit_shr` | (a: int, n: int) | `int` | null_on_failure | - | Shift an integer right by n bits. |
| `greatest` | (values: any...) | `any` | null_on_failure | - | Largest scalar value; null propagates. |
| `least` | (values: any...) | `any` | null_on_failure | - | Smallest scalar value; null propagates. |

## Humanize

| Function | Params | Result | Fallibility | Strict | Description |
|----------|--------|--------|-------------|--------|-------------|
| `humanize_bytes` | (n: number) | `string` | null_on_failure | - | Format bytes with binary units (KiB, MiB, ...). |
| `humanize_count` | (n: number) | `string` | null_on_failure | - | Format counts with SI suffixes (K, M, ...). |
| `humanize_duration` | (d: duration) | `string` | null_on_failure | - | Format a duration in readable units. |
| `bar` | (x: number, lo: number, hi: number, width: int?) | `string` | null_on_failure | - | Render a fixed-width ASCII bar for x in [lo, hi]. |

## Time

| Function | Params | Result | Fallibility | Strict | Description |
|----------|--------|--------|-------------|--------|-------------|
| `now` | () | `timestamp` | infallible | - | Query start time (stable within one query). |
| `bin` | (x: any, width: any) | `any` | null_on_failure | - | Snap a timestamp to a duration boundary or a number to a numeric width; time by-lists emit as _time. |
| `strftime` | (ts: timestamp, format: string, tz?: string) | `string` | null_on_failure | - | Format a timestamp in UTC or an IANA timezone. |
| `strptime` | (s: string, format: string\|array) | `timestamp` | null_on_failure | `strptime!` | Parse with one layout or the first matching layout in an array. |
| `time_of_day` | (ts: timestamp, tz?: string) | `duration` | null_on_failure | - | Duration since midnight in UTC or an IANA timezone. |
| `day_of_week` | (ts: timestamp, tz?: string) | `int` | null_on_failure | - | 0 = Sunday; optional timezone controls the local day. |
| `from_unix` | (n: int, unit: string) | `timestamp` | null_on_failure | - | Convert Unix epoch in s, ms, us, or ns to a timestamp. |
| `to_unix` | (ts: timestamp, unit: string) | `int` | null_on_failure | - | Convert a timestamp to Unix epoch in s, ms, us, or ns. |
| `date_trunc` | (ts: timestamp, part: string, tz: string?) | `timestamp` | null_on_failure | - | Truncate a timestamp to a calendar part. |
| `date_part` | (ts: timestamp, part: string, tz: string?) | `int` | null_on_failure | - | Extract a calendar part from a timestamp. |
| `date_diff` | (a: timestamp, b: timestamp, part: string) | `int` | null_on_failure | - | Difference b - a in a calendar unit. |
| `timestamp_guess` | (s: string) | `timestamp` | null_on_failure | - | Parse a timestamp using common layouts. |

## Hash

| Function | Params | Result | Fallibility | Strict | Description |
|----------|--------|--------|-------------|--------|-------------|
| `md5` | (s: string) | `string` | null_on_failure | - | - |
| `sha1` | (s: string) | `string` | null_on_failure | - | - |
| `sha256` | (s: string) | `string` | null_on_failure | - | - |
| `xxhash64` | (s: string) | `string` | null_on_failure | - | - |

## Network

| Function | Params | Result | Fallibility | Strict | Description |
|----------|--------|--------|-------------|--------|-------------|
| `cidr_match` | (cidr: string, ip: string) | `bool` | null_on_failure | - | - |
| `ip_parse` | (s: string) | `object` | null_on_failure | - | - |
| `is_ip` | (s: string) | `bool` | infallible | - | - |
| `is_ipv4` | (s: string) | `bool` | infallible | - | - |
| `is_ipv6` | (s: string) | `bool` | infallible | - | - |
| `ip_to_int` | (ip: string) | `int` | null_on_failure | - | Convert an IPv4 string to a 32-bit integer; IPv6 returns null. |
| `ip_from_int` | (n: int) | `string` | null_on_failure | - | Convert a 32-bit integer to an IPv4 string. |
| `ipmask` | (mask: string, ip: string) | `string` | null_on_failure | - | - |

## Array

| Function | Params | Result | Fallibility | Strict | Description |
|----------|--------|--------|-------------|--------|-------------|
| `slice` | (arr: array, start: int, end: int?) | `array` | null_on_failure | - | - |
| `array_concat` | (arrays: array...) | `array` | null_on_failure | - | - |
| `array_distinct` | (arr: array) | `array` | null_on_failure | - | - |
| `array_sort` | (arr: array) | `array` | null_on_failure | - | - |
| `array_sum` | (arr: array) | `number` | null_on_failure | - | Sum numeric array elements. |
| `array_avg` | (arr: array) | `float` | null_on_failure | - | Average numeric array elements. |
| `array_min` | (arr: array) | `any` | null_on_failure | - | Minimum comparable array element. |
| `array_max` | (arr: array) | `any` | null_on_failure | - | Maximum comparable array element. |
| `array_compact` | (arr: array) | `array` | null_on_failure | - | Remove consecutive duplicate elements. |
| `array_deltas` | (arr: array) | `array` | null_on_failure | - | Adjacent differences with leading null. |
| `array_cumsum` | (arr: array) | `array` | null_on_failure | - | Cumulative sums across array elements. |
| `generate_series` | (start: int, stop: int, step?: int) | `array` | null_on_failure | - | Integer series from start to stop, excluding stop; default step is 1. |
| `zip` | (arrays: array...) | `array` | null_on_failure | - | Zip equal-length arrays into arrays of tuples. |
| `flatten` | (arr: array) | `array` | null_on_failure | - | One level. |
| `any` | (arr: array, pred: lambda) | `bool` | null_on_failure | - | any(tags, t -&gt; t.name == "vip") |
| `all` | (arr: array, pred: lambda) | `bool` | null_on_failure | - | - |
| `filter` | (arr: array, pred: lambda) | `array` | null_on_failure | - | - |
| `map` | (arr: array, fn: lambda) | `array` | null_on_failure | - | - |

## Object

| Function | Params | Result | Fallibility | Strict | Description |
|----------|--------|--------|-------------|--------|-------------|
| `keys` | (obj: object) | `array` | null_on_failure | - | - |
| `values` | (obj: object) | `array` | null_on_failure | - | - |
| `merge` | (a: object, b: object) | `object` | null_on_failure | - | Right side wins on key collision. |
| `has_key` | (obj: object, key: string) | `bool` | null_on_failure | - | - |
| `entries` | (obj: object) | `array` | null_on_failure | - | Convert an object to sorted {key, value} entries. |
| `from_entries` | (arr: array) | `object` | null_on_failure | - | Build an object from {key, value} entries. |
| `to_json` | (x: any) | `string` | null_on_failure | - | - |
| `from_json` | (s: string) | `any` | null_on_failure | `from_json!` | Null on invalid JSON, never the original string. |
| `is_json` | (s: string) | `bool` | infallible | - | True when a string contains valid JSON. |
| `json_value` | (s: string, path: string) | `any` | null_on_failure | - | Extract a scalar JSON value by path. |
| `json_query` | (s: string, path: string) | `string` | null_on_failure | - | Extract a raw JSON fragment by path. |

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/functions.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full specification.*
