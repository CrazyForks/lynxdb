package sigmaqueries

import (
	"embed"
	"encoding/json"
	"fmt"
)

//go:embed compat_manifest.json
var compatManifestFS embed.FS

// CompatManifest describes the rsigma compatibility corpus embedded in LynxDB.
type CompatManifest struct {
	RsigmaVersion string          `json:"rsigma_version"`
	LynxDBVersion string          `json:"lynxdb_version"`
	Fixtures      []CompatFixture `json:"fixtures"`
}

// CompatFixture describes one fixture of the rsigma compatibility corpus.
// The executable goldens are the .lynxflow files under testdata/golden; this
// manifest entry carries the Sigma rule metadata and the expected match count.
type CompatFixture struct {
	Name   string   `json:"name"`
	RuleID string   `json:"rule_id"`
	Title  string   `json:"title,omitempty"`
	Level  string   `json:"level,omitempty"`
	Tags   []string `json:"tags,omitempty"`
	// LynxFlow is the hand-maintained LynxFlow golden for this fixture —
	// the same text as the .lynxflow file under testdata/golden.
	LynxFlow           string   `json:"lynxflow"`
	Format             string   `json:"format"`
	Shapes             []string `json:"shapes"`
	ExpectedMatchCount int      `json:"expected_match_count"`
}

// EmbeddedCompatManifest returns the compatibility manifest embedded in this binary.
func EmbeddedCompatManifest() (*CompatManifest, error) {
	data, err := compatManifestFS.ReadFile("compat_manifest.json")
	if err != nil {
		return nil, fmt.Errorf("sigmaqueries: read embedded compat manifest: %w", err)
	}

	var manifest CompatManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("sigmaqueries: decode embedded compat manifest: %w", err)
	}

	return &manifest, nil
}
