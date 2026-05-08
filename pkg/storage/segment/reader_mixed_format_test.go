package segment

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestIntegration_ReaderV2_OpensV1Fixtures(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "..", "testdata", "segments", "v1*.lsg"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no v1 fixtures found")
	}

	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			assertV1SegmentOpensWithV2Reader(t, data)
		})
	}
}

func TestIntegration_ReaderV2_OpensSyntheticV1Segment(t *testing.T) {
	restore := defaultFormatMajor
	defaultFormatMajor = LSG_FORMAT_MAJOR_V1
	t.Cleanup(func() { defaultFormatMajor = restore })

	events := generateTestEvents(12)
	var buf bytes.Buffer
	w := NewWriter(&buf)
	w.SetRowGroupSize(6)
	if _, err := w.Write(events); err != nil {
		t.Fatalf("Write: %v", err)
	}

	assertV1SegmentOpensWithV2Reader(t, buf.Bytes())
}

func assertV1SegmentOpensWithV2Reader(t *testing.T, data []byte) {
	t.Helper()

	major, err := SegmentHeaderMajor(data, int64(len(data)))
	if err != nil {
		t.Fatalf("SegmentHeaderMajor: %v", err)
	}
	if major != LSG_FORMAT_MAJOR_V1 {
		t.Fatalf("header major = %d, want %d", major, LSG_FORMAT_MAJOR_V1)
	}

	r, err := OpenSegment(data)
	if err != nil {
		t.Fatalf("OpenSegment: %v", err)
	}
	if r.EventCount() == 0 {
		t.Fatal("expected non-empty V1 segment")
	}
	if len(r.footer.RowGroups) == 0 {
		t.Fatal("expected at least one row group")
	}
	for i, rg := range r.footer.RowGroups {
		if rg.PerColumnRangeOffset != 0 {
			t.Fatalf("RowGroups[%d].PerColumnRangeOffset = %d, want 0", i, rg.PerColumnRangeOffset)
		}
		if rg.PerColumnRangeLength != 0 {
			t.Fatalf("RowGroups[%d].PerColumnRangeLength = %d, want 0", i, rg.PerColumnRangeLength)
		}
	}
	for i, cat := range r.footer.Catalog {
		if cat.IndexProfile != IndexProfileDefault {
			t.Fatalf("Catalog[%d].IndexProfile = %d, want %d", i, cat.IndexProfile, IndexProfileDefault)
		}
	}

	if _, err := r.ReadEvents(); err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if r.footer.InvertedLength > 0 {
		if _, err := r.InvertedIndex(); err != nil {
			t.Fatalf("InvertedIndex: %v", err)
		}
	}
	if r.footer.PrimaryIndexLength > 0 {
		if _, err := r.PrimaryIndex(); err != nil {
			t.Fatalf("PrimaryIndex: %v", err)
		}
	}
}
