// Package sigmaqueries provides helpers and compatibility tests for LynxFlow
// queries produced by `rsigma convert -t lynxdb`. rsigma is an external tool;
// nothing in this package calls or embeds it.
//
// The golden corpus under testdata/golden consists of .lynxflow fixtures plus
// the source Sigma rules (.yml) and reference match sets (.matches.json). The
// embedded compat_manifest.json carries per-fixture metadata (rule id, shape
// labels, expected match counts) alongside the LynxFlow query text; regenerate
// it with `make compat-manifest` after changing the goldens.
package sigmaqueries
