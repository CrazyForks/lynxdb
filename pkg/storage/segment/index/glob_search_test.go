package index

import (
	"fmt"
	"testing"
)

func buildSerialized(t *testing.T, build func(*InvertedIndex)) *SerializedIndex {
	t.Helper()
	idx := NewInvertedIndex()
	build(idx)
	data, err := idx.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	si, err := DecodeInvertedIndex(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	return si
}

func TestSearchGlobMatchesTerms(t *testing.T) {
	si := buildSerialized(t, func(idx *InvertedIndex) {
		idx.Add(1, "Accepted password for user from 10.0.0.1")
		idx.Add(2, "Failed password for invalid user admin")
		idx.Add(3, "session opened for root")
	})

	for _, tc := range []struct {
		pattern string
		want    []uint32
	}{
		{"us*r", []uint32{1, 2}},    // token "user"
		{"*ssword", []uint32{1, 2}}, // leading star, token "password"
		{"sess*", []uint32{3}},
		{"r??t", []uint32{3}},
		{"acc*ed", []uint32{1}},
		{"zz*", nil},
	} {
		bm, ok, err := si.SearchGlob(tc.pattern)
		if err != nil {
			t.Fatalf("SearchGlob(%q): %v", tc.pattern, err)
		}
		if !ok {
			t.Fatalf("SearchGlob(%q): expected ok=true", tc.pattern)
		}
		got := bm.ToArray()
		if fmt.Sprint(got) != fmt.Sprint(tc.want) && !(len(got) == 0 && len(tc.want) == 0) {
			t.Errorf("SearchGlob(%q) = %v, want %v", tc.pattern, got, tc.want)
		}
	}
}

// Composite field\x00value keys must never satisfy a token glob: '*' could
// otherwise match across the separator and produce false postings.
func TestSearchGlobSkipsCompositeKeys(t *testing.T) {
	si := buildSerialized(t, func(idx *InvertedIndex) {
		idx.AddField(7, "user", "root") // key "user\x00root"
		idx.Add(8, "nothing relevant here")
	})

	// "us*t" would match "user\x00root" if composite keys leaked through.
	bm, ok, err := si.SearchGlob("us*t")
	if err != nil || !ok {
		t.Fatalf("SearchGlob: ok=%v err=%v", ok, err)
	}
	if bm.GetCardinality() != 0 {
		t.Fatalf("composite key leaked into glob expansion: %v", bm.ToArray())
	}
}

// Empty index (no FST) is a proof of no match, not a fallback.
func TestSearchGlobEmptyIndex(t *testing.T) {
	si := buildSerialized(t, func(idx *InvertedIndex) {})
	bm, ok, err := si.SearchGlob("us*r")
	if err != nil {
		t.Fatalf("SearchGlob: %v", err)
	}
	if !ok || bm.GetCardinality() != 0 {
		t.Fatalf("empty index: want ok=true empty bitmap, got ok=%v card=%d", ok, bm.GetCardinality())
	}
}

// Oversized expansions abandon pruning (ok=false) instead of paying an
// unbounded OR of posting lists.
func TestSearchGlobExpansionCap(t *testing.T) {
	si := buildSerialized(t, func(idx *InvertedIndex) {
		for i := 0; i < maxGlobTermExpansion+10; i++ {
			idx.Add(uint32(i), fmt.Sprintf("tok%d", i))
		}
	})
	_, ok, err := si.SearchGlob("tok*")
	if err != nil {
		t.Fatalf("SearchGlob: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false when expansion exceeds the cap")
	}
}

// Invalid patterns surface as errors (ok=false) — callers degrade to scan.
func TestSearchGlobInvalidPattern(t *testing.T) {
	si := buildSerialized(t, func(idx *InvertedIndex) {
		idx.Add(1, "hello world")
	})
	_, ok, err := si.SearchGlob("[ab")
	if err == nil || ok {
		t.Fatalf("want error and ok=false for invalid pattern, got ok=%v err=%v", ok, err)
	}
}

func TestPrefixSuccessor(t *testing.T) {
	for prefix, want := range map[string]string{
		"us":     "ut",
		"a":      "b",
		"to\xff": "tp", // trailing 0xff trimmed, previous byte incremented
	} {
		got := prefixSuccessor(prefix)
		if string(got) != want {
			t.Errorf("prefixSuccessor(%q) = %q, want %q", prefix, got, want)
		}
	}
	if got := prefixSuccessor("\xff\xff"); got != nil {
		t.Errorf("prefixSuccessor(all-0xff) = %q, want nil", got)
	}
}
