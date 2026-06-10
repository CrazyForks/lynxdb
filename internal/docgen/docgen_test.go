package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestGeneratedDocsUpToDate regenerates all documentation artefacts into a
// temp directory and compares them byte-for-byte against the committed output.
// If this test fails, run:
//
//	go run ./internal/docgen
func TestGeneratedDocsUpToDate(t *testing.T) {
	root := findTestRepoRoot(t)
	tmp := t.TempDir()

	// Generate into temp dir.
	if err := Generate(tmp); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// Walk the temp dir and compare every file against the committed copy.
	err := filepath.Walk(tmp, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(tmp, path)
		committed := filepath.Join(root, rel)

		got, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		want, err := os.ReadFile(committed)
		if err != nil {
			t.Errorf("missing committed file %s — run: go run ./internal/docgen", rel)
			return nil
		}
		if string(got) != string(want) {
			t.Errorf("stale file %s — run: go run ./internal/docgen", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
}

func findTestRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("cannot determine cwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("cannot find repo root (go.mod)")
		}
		dir = parent
	}
}
