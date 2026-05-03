package segment

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCapabilityRegistryStatic(t *testing.T) {
	if CapBit_ColumnZSTD != 1 {
		t.Fatalf("CapBit_ColumnZSTD = %#x, want 0x1", CapBit_ColumnZSTD)
	}
	if LSG_REQUIRED_CAPS_KNOWN != CapBit_ColumnZSTD {
		t.Fatalf("LSG_REQUIRED_CAPS_KNOWN = %#x, want %#x", LSG_REQUIRED_CAPS_KNOWN, CapBit_ColumnZSTD)
	}
	if LSG_OPTIONAL_CAPS_KNOWN != 0 {
		t.Fatalf("LSG_OPTIONAL_CAPS_KNOWN = %#x, want 0", LSG_OPTIONAL_CAPS_KNOWN)
	}

	data, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "storage-format.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Count(text, "name: ColumnZSTD") != 1 {
		t.Fatalf("docs capability registry should contain ColumnZSTD exactly once")
	}
	if strings.Count(text, "bit: 0") != 1 {
		t.Fatalf("docs capability registry should assign bit 0 exactly once")
	}
}
