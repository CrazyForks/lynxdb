package index

import (
	"bytes"
	"testing"
)

type rangeDecodeExpectation struct {
	name      string
	kind      uint8
	minValue  int64
	maxValue  int64
	bitCount  uint8
	rowValues map[uint64]int64
}

func TestUnit_RangeSectionDecoder_FourColumns_DecodesAllEntries(t *testing.T) {
	section, expected := buildRangeDecodeSection(t)

	entries, err := DecodeRangeSection(section)
	if err != nil {
		t.Fatalf("DecodeRangeSection: %v", err)
	}
	if len(entries) != len(expected) {
		t.Fatalf("entries len = %d, want %d", len(entries), len(expected))
	}

	for _, want := range expected {
		got := entries[want.name]
		assertDecodedRangeEntry(t, got, want)
	}
}

func TestUnit_RangeSectionDecoder_SingleColumn_DecodesMatchingEntry(t *testing.T) {
	section, expected := buildRangeDecodeSection(t)

	for _, want := range expected {
		got, err := DecodeRangeSectionEntry(section, want.name)
		if err != nil {
			t.Fatalf("DecodeRangeSectionEntry(%q): %v", want.name, err)
		}
		assertDecodedRangeEntry(t, got, want)
	}
}

func TestUnit_RangeSectionDecoder_MissingColumn_ReturnsNilEntry(t *testing.T) {
	section, _ := buildRangeDecodeSection(t)

	got, err := DecodeRangeSectionEntry(section, "does_not_exist")
	if err != nil {
		t.Fatalf("DecodeRangeSectionEntry(missing): %v", err)
	}
	if got != nil {
		t.Fatalf("entry = %+v, want nil", got)
	}
}

func TestUnit_RangeSectionDecoder_EmptySection_ReturnsEmptyMap(t *testing.T) {
	var buf bytes.Buffer
	enc := NewRangeSectionEncoder(&buf, 0)
	if _, _, err := enc.Finalize(); err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	entries, err := DecodeRangeSection(buf.Bytes())
	if err != nil {
		t.Fatalf("DecodeRangeSection(empty): %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("entries len = %d, want 0", len(entries))
	}

	entry, err := DecodeRangeSectionEntry(buf.Bytes(), "anything")
	if err != nil {
		t.Fatalf("DecodeRangeSectionEntry(empty): %v", err)
	}
	if entry != nil {
		t.Fatalf("entry = %+v, want nil", entry)
	}
}

func buildRangeDecodeSection(t *testing.T) ([]byte, []rangeDecodeExpectation) {
	t.Helper()

	var buf bytes.Buffer
	enc := NewRangeSectionEncoder(&buf, 0)
	expected := make([]rangeDecodeExpectation, 0, 4)

	addColumn := func(name string, kind uint8, minValue, maxValue int64, rawValues map[uint32]int64) {
		t.Helper()
		builder := NewBSIBuilder(kind, minValue, maxValue)
		rowValues := make(map[uint64]int64, len(rawValues))
		for rowID, raw := range rawValues {
			builder.Set(rowID, raw)
			rowValues[uint64(rowID)] = int64(uint64(raw) - uint64(minValue))
		}
		if err := enc.AddColumn(name, kind, minValue, maxValue, builder.Build()); err != nil {
			t.Fatalf("AddColumn(%q): %v", name, err)
		}
		expected = append(expected, rangeDecodeExpectation{
			name:      name,
			kind:      kind,
			minValue:  minValue,
			maxValue:  maxValue,
			bitCount:  uint8(builder.BitCount()),
			rowValues: rowValues,
		})
	}

	addColumn("status", RangeBSIValueInt, 100, 599, map[uint32]int64{
		0: 100,
		1: 204,
		3: 599,
	})
	minLatency := FloatToOrderedInt64(0.5)
	maxLatency := FloatToOrderedInt64(999.9)
	addColumn("latency", RangeBSIValueFloat64Bits, minLatency, maxLatency, map[uint32]int64{
		0: minLatency,
		2: FloatToOrderedInt64(42.25),
		4: maxLatency,
	})
	addColumn("_time", RangeBSIValueTimestampNS, 1_700_000_000_000_000_000, 1_700_000_000_000_090_000, map[uint32]int64{
		1: 1_700_000_000_000_000_000,
		2: 1_700_000_000_000_050_000,
		5: 1_700_000_000_000_090_000,
	})
	addColumn("is_error", RangeBSIValueBool, 0, 1, map[uint32]int64{
		0: 0,
		1: 1,
		6: 1,
	})

	if _, _, err := enc.Finalize(); err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	return append([]byte(nil), buf.Bytes()...), expected
}

func assertDecodedRangeEntry(t *testing.T, got *RangeSectionEntry, want rangeDecodeExpectation) {
	t.Helper()
	if got == nil {
		t.Fatalf("entry %q = nil", want.name)
	}
	if got.Name != want.name {
		t.Fatalf("entry name = %q, want %q", got.Name, want.name)
	}
	if got.Layout != 0 {
		t.Fatalf("entry %q layout = %d, want 0", want.name, got.Layout)
	}
	if got.ValueKind != want.kind {
		t.Fatalf("entry %q valueKind = %d, want %d", want.name, got.ValueKind, want.kind)
	}
	if got.MinValue != want.minValue {
		t.Fatalf("entry %q minValue = %d, want %d", want.name, got.MinValue, want.minValue)
	}
	if got.MaxValue != want.maxValue {
		t.Fatalf("entry %q maxValue = %d, want %d", want.name, got.MaxValue, want.maxValue)
	}
	if got.BitCount != want.bitCount {
		t.Fatalf("entry %q bitCount = %d, want %d", want.name, got.BitCount, want.bitCount)
	}
	if got.BSI == nil {
		t.Fatalf("entry %q BSI = nil", want.name)
	}
	for rowID, wantValue := range want.rowValues {
		gotValue, ok := got.BSI.GetValue(rowID)
		if !ok {
			t.Fatalf("entry %q row %d missing", want.name, rowID)
		}
		if gotValue != wantValue {
			t.Fatalf("entry %q row %d value = %d, want %d", want.name, rowID, gotValue, wantValue)
		}
	}
}
