// Package sigmaqueries provides helpers and compatibility tests for LynxFlow
// queries produced by `rsigma convert -t lynxdb`. rsigma is an external tool;
// nothing in this package calls or embeds it.
//
// The golden corpus under testdata/golden consists of .lynxflow fixtures plus
// the source Sigma rules (.yml) and reference match sets (.matches.json). The
// embedded compat_manifest.json carries per-fixture metadata; its "spl2" query
// field is the legacy rsigma SPL2 output, retained only for its
// expected_match_count bookkeeping (SPL2 itself was removed in RFC-002
// Phase 10).
package sigmaqueries
