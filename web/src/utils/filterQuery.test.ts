import { describe, it, expect } from "vitest";
import {
  appendFilter,
  _quoteField as quoteField,
  _isNumericLiteral as isNumericLiteral,
} from "./filterQuery";

// ---------------------------------------------------------------------------
// quoteField (backtick logic)
// ---------------------------------------------------------------------------

describe("quoteField", () => {
  it("passes a plain identifier through", () => {
    expect(quoteField("level")).toBe("level");
  });

  it("backtick-quotes a hard keyword (case-insensitive check)", () => {
    expect(quoteField("not")).toBe("`not`");
    expect(quoteField("true")).toBe("`true`");
    expect(quoteField("null")).toBe("`null`");
    expect(quoteField("in")).toBe("`in`");
    expect(quoteField("between")).toBe("`between`");
    expect(quoteField("and")).toBe("`and`");
    expect(quoteField("or")).toBe("`or`");
    expect(quoteField("false")).toBe("`false`");
  });

  it("backtick-quotes identifiers with non-alphanum characters", () => {
    expect(quoteField("field-with-dash")).toBe("`field-with-dash`");
    expect(quoteField("weird field")).toBe("`weird field`");
    expect(quoteField("a.b")).toBe("`a.b`");
    expect(quoteField("123bad")).toBe("`123bad`");
  });

  it("does not backtick-quote soft keywords like 'stats' or 'where'", () => {
    expect(quoteField("stats")).toBe("stats");
    expect(quoteField("where")).toBe("where");
    expect(quoteField("sort")).toBe("sort");
  });
});

// ---------------------------------------------------------------------------
// isNumericLiteral
// ---------------------------------------------------------------------------

describe("isNumericLiteral", () => {
  it("recognises integers", () => {
    expect(isNumericLiteral("42")).toBe(true);
    expect(isNumericLiteral("0")).toBe(true);
    expect(isNumericLiteral("-7")).toBe(true);
  });

  it("recognises floats", () => {
    expect(isNumericLiteral("3.14")).toBe(true);
    expect(isNumericLiteral(".5")).toBe(true);
    expect(isNumericLiteral("1e-6")).toBe(true);
  });

  it("recognises hex", () => {
    expect(isNumericLiteral("0x2a")).toBe(true);
  });

  it("rejects non-numeric strings", () => {
    expect(isNumericLiteral("abc")).toBe(false);
    expect(isNumericLiteral("")).toBe(false);
    expect(isNumericLiteral("12abc")).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// appendFilter — the primary export
// ---------------------------------------------------------------------------

describe("appendFilter", () => {
  it("creates a leading clause for an empty query", () => {
    expect(appendFilter("", "level", "ERROR", false)).toBe(
      '| where level == "ERROR"',
    );
  });

  it("appends to an existing query and trims it", () => {
    expect(appendFilter("  from main  ", "level", "ERROR", false)).toBe(
      'from main | where level == "ERROR"',
    );
  });

  it("uses != when excluding", () => {
    expect(appendFilter("from main", "status", "500", true)).toBe(
      "from main | where status != 500",
    );
  });

  it("escapes embedded double quotes in string values", () => {
    expect(appendFilter("", "msg", 'he said "hi"', false)).toBe(
      '| where msg == "he said \\"hi\\""',
    );
  });

  it("escapes embedded backslashes in string values", () => {
    expect(appendFilter("", "path", "C:\\Users\\foo", false)).toBe(
      '| where path == "C:\\\\Users\\\\foo"',
    );
  });

  it("returns the query unchanged for a null value", () => {
    expect(
      appendFilter("from main", "f", null as unknown as string, false),
    ).toBe("from main");
  });

  it("emits numeric values without quotes (integer)", () => {
    expect(appendFilter("from main", "status", "500", false)).toBe(
      "from main | where status == 500",
    );
  });

  it("emits numeric values without quotes (float)", () => {
    expect(appendFilter("", "duration_ms", "3.14", false)).toBe(
      "| where duration_ms == 3.14",
    );
  });

  it("emits numeric values without quotes (hex)", () => {
    expect(appendFilter("", "code", "0x2a", false)).toBe(
      "| where code == 0x2a",
    );
  });

  it("quotes string values that look like non-numbers", () => {
    expect(appendFilter("", "host", "web-01", false)).toBe(
      '| where host == "web-01"',
    );
  });

  it("backtick-quotes a field name that is a hard keyword", () => {
    expect(appendFilter("", "not", "yes", false)).toBe(
      '| where `not` == "yes"',
    );
    expect(appendFilter("", "true", "1", false)).toBe(
      "| where `true` == 1",
    );
  });

  it("backtick-quotes a field name with special characters", () => {
    expect(appendFilter("", "http.status", "200", false)).toBe(
      "| where `http.status` == 200",
    );
  });
});
