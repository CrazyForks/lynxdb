/**
 * Shared utility for building filter clauses that are appended to the
 * current LynxFlow query text.  Used by EventDetail field filter buttons and
 * (in the future) field sidebar value drill-down popovers.
 *
 * LynxFlow syntax rules (RFC-002):
 *   - `==` compares, `=` binds.
 *   - String literals use double quotes with `\"` escaping.
 *   - Identifiers that clash with hard keywords must be backtick-quoted:
 *     and, or, not, in, between, true, false, null.
 *   - Existence checks use `exists(field)` / `not exists(field)`.
 *   - Negation: `!=` for inequality.
 */

/**
 * Hard keywords that MUST be backtick-quoted when used as field names.
 * Soft keywords (stage names, `as`, `by`, etc.) do NOT need quoting in
 * expression position per RFC-002 §2.2.
 */
const HARD_KEYWORDS = new Set([
  "and",
  "or",
  "not",
  "in",
  "between",
  "true",
  "false",
  "null",
]);

/**
 * Returns true when `name` is a bare identifier per RFC-002 §2.2:
 * `[A-Za-z_][A-Za-z0-9_]*` and NOT a hard keyword.
 */
const BARE_IDENT_RE = /^[A-Za-z_][A-Za-z0-9_]*$/;

function quoteField(name: string): string {
  if (HARD_KEYWORDS.has(name.toLowerCase())) {
    return `\`${name}\``;
  }
  if (BARE_IDENT_RE.test(name)) {
    return name;
  }
  // Anything with dashes, spaces, dots-as-flat-column, etc. needs backticks.
  return `\`${name}\``;
}

/**
 * Escape a string value for embedding in a LynxFlow double-quoted literal.
 * Escapes `"` and `\` per RFC-002 §2.3 string escapes.
 */
function escapeStringValue(value: string): string {
  return value.replace(/\\/g, "\\\\").replace(/"/g, '\\"');
}

/**
 * Return true if `value` looks like a number (int or float) so we can emit
 * an unquoted numeric literal instead of a string comparison.
 */
function isNumericLiteral(value: string): boolean {
  return /^-?(?:0x[0-9a-fA-F]+|(?:\d+\.?\d*|\.\d+)(?:e[+-]?\d+)?)$/.test(
    value,
  );
}

/**
 * Append a `| where field == "value"` (or `!=`) clause to the current query.
 *
 * @param currentQuery - The existing query text (may be empty).
 * @param field        - The field name to filter on.
 * @param value        - The field value to match.
 * @param exclude      - When true, uses `!=` instead of `==`.
 * @returns The modified query string.
 */
export function appendFilter(
  currentQuery: string,
  field: string,
  value: string,
  exclude: boolean,
): string {
  if (value == null) return currentQuery;

  const qf = quoteField(field);
  const op = exclude ? "!=" : "==";

  // Use numeric literal when the value looks like a number.
  const rhs = isNumericLiteral(value)
    ? value
    : `"${escapeStringValue(value)}"`;

  const clause = `where ${qf} ${op} ${rhs}`;

  const trimmed = currentQuery.trim();
  if (!trimmed) {
    return `| ${clause}`;
  }

  return `${trimmed} | ${clause}`;
}

// Re-export helpers for testing
export { quoteField as _quoteField, isNumericLiteral as _isNumericLiteral };
