package index

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"testing"
)

type rangeEntryOffsetsForTest struct {
	name         string
	nameOffset   int
	layoutOffset int
	payloadStart int
	payloadLen   int
	crcOffset    int
	entryStart   int
	entryEnd     int
}

func TestUnit_RangeSectionDecoder_PayloadMutation_ReturnsCorruptError(t *testing.T) {
	section := buildThreeColumnRangeSection(t)
	offsets := rangeEntryOffsetsByName(t, section)
	middle := offsets["middle"]
	if middle.payloadLen == 0 {
		t.Fatal("middle payload is empty")
	}
	mutated := append([]byte(nil), section...)
	mutated[middle.payloadStart+middle.payloadLen/2] ^= 0xff

	if _, err := DecodeRangeSection(mutated); !errors.Is(err, ErrRangeSectionCorrupt) {
		t.Fatalf("DecodeRangeSection err = %v, want ErrRangeSectionCorrupt", err)
	}
	if _, err := DecodeRangeSectionEntry(mutated, "middle"); !errors.Is(err, ErrRangeSectionCorrupt) {
		t.Fatalf("DecodeRangeSectionEntry(middle) err = %v, want ErrRangeSectionCorrupt", err)
	}
}

func TestUnit_RangeSectionDecoder_NameMutation_ReturnsCorruptError(t *testing.T) {
	section := buildThreeColumnRangeSection(t)
	offsets := rangeEntryOffsetsByName(t, section)
	second := offsets["middle"]
	mutated := append([]byte(nil), section...)
	mutated[second.nameOffset] ^= 0x20

	if _, err := DecodeRangeSection(mutated); !errors.Is(err, ErrRangeSectionCorrupt) {
		t.Fatalf("DecodeRangeSection err = %v, want ErrRangeSectionCorrupt", err)
	}
}

func TestUnit_RangeSectionDecoder_TruncatedCRC_ReturnsCorruptError(t *testing.T) {
	section := buildThreeColumnRangeSection(t)
	truncated := append([]byte(nil), section[:len(section)-4]...)

	if _, err := DecodeRangeSection(truncated); !errors.Is(err, ErrRangeSectionCorrupt) {
		t.Fatalf("DecodeRangeSection err = %v, want ErrRangeSectionCorrupt", err)
	}
	if _, err := DecodeRangeSectionEntry(truncated, "last"); !errors.Is(err, ErrRangeSectionCorrupt) {
		t.Fatalf("DecodeRangeSectionEntry(last) err = %v, want ErrRangeSectionCorrupt", err)
	}
}

func TestUnit_RangeSectionDecoder_UnsupportedLayout_ReturnsUnsupportedLayoutError(t *testing.T) {
	section := buildThreeColumnRangeSection(t)
	offsets := rangeEntryOffsetsByName(t, section)
	first := offsets["first"]
	mutated := append([]byte(nil), section...)
	mutated[first.layoutOffset] = 1
	rewriteRangeEntryCRCForTest(mutated, first)

	if _, err := DecodeRangeSection(mutated); !errors.Is(err, ErrUnsupportedRangeLayout) {
		t.Fatalf("DecodeRangeSection err = %v, want ErrUnsupportedRangeLayout", err)
	}
	if _, err := DecodeRangeSectionEntry(mutated, "first"); !errors.Is(err, ErrUnsupportedRangeLayout) {
		t.Fatalf("DecodeRangeSectionEntry(first) err = %v, want ErrUnsupportedRangeLayout", err)
	}
}

func buildThreeColumnRangeSection(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer
	enc := NewRangeSectionEncoder(&buf, 0)
	add := func(name string, minValue, maxValue int64, rows map[uint32]int64) {
		t.Helper()
		builder := NewBSIBuilder(RangeBSIValueInt, minValue, maxValue)
		for rowID, raw := range rows {
			builder.Set(rowID, raw)
		}
		if err := enc.AddColumn(name, RangeBSIValueInt, minValue, maxValue, builder.Build()); err != nil {
			t.Fatalf("AddColumn(%q): %v", name, err)
		}
	}
	add("first", 10, 40, map[uint32]int64{0: 10, 1: 25, 2: 40})
	add("middle", 100, 200, map[uint32]int64{0: 100, 2: 150, 4: 200})
	add("last", 1000, 2000, map[uint32]int64{1: 1000, 3: 1500, 5: 2000})
	if _, _, err := enc.Finalize(); err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	return append([]byte(nil), buf.Bytes()...)
}

func rangeEntryOffsetsByName(t *testing.T, section []byte) map[string]rangeEntryOffsetsForTest {
	t.Helper()
	count, pos, err := decodeRangeSectionHeader(section)
	if err != nil {
		t.Fatalf("decodeRangeSectionHeader: %v", err)
	}
	offsets := make(map[string]rangeEntryOffsetsForTest, count)
	for i := uint16(0); i < count; i++ {
		entryStart := pos
		if pos+2 > len(section) {
			t.Fatalf("entry %d truncated before name length", i)
		}
		nameLen := int(binary.LittleEndian.Uint16(section[pos : pos+2]))
		pos += 2
		nameOffset := pos
		if pos+nameLen+1+1+8+8+1+4 > len(section) {
			t.Fatalf("entry %d truncated before payload", i)
		}
		name := string(section[pos : pos+nameLen])
		pos += nameLen
		layoutOffset := pos
		pos += 1 + 1 + 8 + 8 + 1
		payloadLen := int(binary.LittleEndian.Uint32(section[pos : pos+4]))
		pos += 4
		payloadStart := pos
		pos += payloadLen
		if pos+4 > len(section) {
			t.Fatalf("entry %d truncated before crc", i)
		}
		crcOffset := pos
		offsets[name] = rangeEntryOffsetsForTest{
			name:         name,
			nameOffset:   nameOffset,
			layoutOffset: layoutOffset,
			payloadStart: payloadStart,
			payloadLen:   payloadLen,
			crcOffset:    crcOffset,
			entryStart:   entryStart,
			entryEnd:     pos,
		}
		pos += 4
	}
	if pos != len(section) {
		t.Fatalf("section has %d trailing bytes", len(section)-pos)
	}
	return offsets
}

func rewriteRangeEntryCRCForTest(section []byte, entry rangeEntryOffsetsForTest) {
	crc := crc32.ChecksumIEEE(section[entry.entryStart:entry.entryEnd])
	binary.LittleEndian.PutUint32(section[entry.crcOffset:entry.crcOffset+4], crc)
}
