// Command lynxdb-compat-manifest regenerates pkg/sigmaqueries/compat_manifest.json
// from the hand-maintained .lynxflow golden fixtures and the rsigma sync
// manifest. The compat manifest is embedded into the lynxdb binary and powers
// `lynxdb sigma compat-check` plus the conformance count assertions in
// pkg/sigmaqueries tests.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type syncManifest struct {
	RsigmaVersion string                       `json:"rsigma_version"`
	Queries       []syncManifestEntry          `json:"queries"`
	Fixtures      map[string]syncManifestEntry `json:"fixtures"`
}

type syncManifestEntry struct {
	Fixture string   `json:"fixture,omitempty"`
	Line    int      `json:"line"`
	RuleID  string   `json:"rule_id"`
	Title   string   `json:"title"`
	Level   string   `json:"level"`
	Tags    []string `json:"tags"`
}

type matchesFile struct {
	MatchCount int `json:"match_count"`
}

type compatManifest struct {
	RsigmaVersion string          `json:"rsigma_version"`
	LynxDBVersion string          `json:"lynxdb_version"`
	Fixtures      []compatFixture `json:"fixtures"`
}

type compatFixture struct {
	Name               string   `json:"name"`
	RuleID             string   `json:"rule_id"`
	Title              string   `json:"title,omitempty"`
	Level              string   `json:"level,omitempty"`
	Tags               []string `json:"tags,omitempty"`
	LynxFlow           string   `json:"lynxflow"`
	Format             string   `json:"format"`
	Shapes             []string `json:"shapes"`
	ExpectedMatchCount int      `json:"expected_match_count"`
}

func main() {
	goldenDir := flag.String("golden-dir", filepath.Join("pkg", "sigmaqueries", "testdata", "golden"), "golden corpus directory")
	lynxdbVersion := flag.String("lynxdb-version", "dev", "LynxDB version to record")
	output := flag.String("output", filepath.Join("pkg", "sigmaqueries", "compat_manifest.json"), "output manifest path")
	flag.Parse()

	manifest, err := buildCompatManifest(*goldenDir, *lynxdbVersion)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err := writeCompatManifest(*output, manifest); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func buildCompatManifest(goldenDir, lynxdbVersion string) (*compatManifest, error) {
	syncData, err := readSyncManifest(filepath.Join(goldenDir, "manifest.json"))
	if err != nil {
		return nil, err
	}

	files, err := filepath.Glob(filepath.Join(goldenDir, "*.lynxflow"))
	if err != nil {
		return nil, fmt.Errorf("glob lynxflow fixtures: %w", err)
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil, fmt.Errorf("no LynxFlow fixtures under %s", goldenDir)
	}

	out := &compatManifest{
		RsigmaVersion: syncData.RsigmaVersion,
		LynxDBVersion: lynxdbVersion,
		Fixtures:      make([]compatFixture, 0, len(files)),
	}
	for _, path := range files {
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		format := fixtureFormat(name)
		baseName := fixtureBaseName(name)
		entry := syncData.Fixtures[baseName]

		query, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		matchCount, err := readMatchCount(filepath.Join(goldenDir, baseName+".matches.json"))
		if err != nil {
			return nil, err
		}

		trimmed := strings.TrimSpace(string(query))
		out.Fixtures = append(out.Fixtures, compatFixture{
			Name:               name,
			RuleID:             entry.RuleID,
			Title:              entry.Title,
			Level:              entry.Level,
			Tags:               entry.Tags,
			LynxFlow:           trimmed,
			Format:             format,
			Shapes:             detectShapes(trimmed),
			ExpectedMatchCount: matchCount,
		})
	}

	return out, nil
}

func readSyncManifest(path string) (*syncManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read sync manifest: %w", err)
	}

	var manifest syncManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("decode sync manifest: %w", err)
	}
	if manifest.Fixtures == nil {
		return nil, fmt.Errorf("sync manifest has no fixtures map")
	}

	return &manifest, nil
}

func readMatchCount(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read match fixture %s: %w", path, err)
	}

	var matches matchesFile
	if err := json.Unmarshal(data, &matches); err != nil {
		return 0, fmt.Errorf("decode match fixture %s: %w", path, err)
	}

	return matches.MatchCount, nil
}

func writeCompatManifest(path string, manifest *compatManifest) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir output dir: %w", err)
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal compat manifest: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write compat manifest: %w", err)
	}

	return nil
}

func fixtureFormat(name string) string {
	switch {
	case strings.HasSuffix(name, "_minimal"):
		return "minimal"
	case strings.HasSuffix(name, "_index"):
		return "index"
	default:
		return "default"
	}
}

func fixtureBaseName(name string) string {
	name = strings.TrimSuffix(name, "_minimal")
	name = strings.TrimSuffix(name, "_index")

	return name
}

// detectShapes classifies the LynxFlow constructs a fixture exercises. The
// labels are stable identifiers consumed by conformance reporting; they are
// derived from the query text, not parsed, so keep markers unambiguous.
func detectShapes(query string) []string {
	shapes := []string{"where.predicate"}
	if strings.Contains(query, "matches(") {
		shapes = append(shapes, "where.regex")
	}
	if strings.Contains(query, "cidr_match(") {
		shapes = append(shapes, "where.cidrmatch")
	}
	if strings.Contains(query, " in [") {
		shapes = append(shapes, "where.in")
	}
	if strings.Contains(query, " and ") {
		shapes = append(shapes, "where.boolean.and")
	}
	if strings.Contains(query, " or ") {
		shapes = append(shapes, "where.boolean.or")
	}
	if strings.Contains(query, "not ") {
		shapes = append(shapes, "where.boolean.not")
	}
	if strings.Contains(query, "has(") {
		shapes = append(shapes, "where.fulltext")
	}
	if strings.Contains(query, "contains(") || strings.Contains(query, "starts_with(") || strings.Contains(query, "ends_with(") {
		shapes = append(shapes, "where.substring")
	}
	if strings.Contains(query, "exists(") {
		shapes = append(shapes, "where.exists")
	}
	if strings.Contains(query, ">=") || strings.Contains(query, "<=") || strings.Contains(query, " > ") || strings.Contains(query, " < ") {
		shapes = append(shapes, "where.comparison")
	}

	return shapes
}
