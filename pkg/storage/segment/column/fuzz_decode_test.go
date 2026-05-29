package column

import (
	"encoding/binary"
	"errors"
	"testing"
)

// The column decoders parse binary on-disk data with length and count fields
// read straight from the input. These tests assert the decoders never panic on
// arbitrary bytes and reject malformed headers that claim implausible sizes
// instead of allocating for them.

func FuzzDeltaDecode(f *testing.F) {
	if b, err := NewDeltaEncoder().EncodeInt64s([]int64{1, 2, 3, -4, 5}); err == nil {
		f.Add(b)
	}
	// Malformed: header claims ~4 billion values in 13 bytes.
	bomb := make([]byte, 13)
	bomb[0] = byte(EncodingDelta)
	binary.LittleEndian.PutUint32(bomb[1:5], 0xFFFFFFFF)
	f.Add(bomb)

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = NewDeltaEncoder().DecodeInt64s(data) // must not panic
	})
}

func FuzzDictDecode(f *testing.F) {
	if b, err := NewDictEncoder().EncodeStrings([]string{"a", "bb", "a", "ccc"}); err == nil {
		f.Add(b)
	}
	bomb := make([]byte, 9)
	bomb[0] = byte(EncodingDict8)
	binary.LittleEndian.PutUint32(bomb[1:5], 0xFFFFFFFF) // count
	binary.LittleEndian.PutUint32(bomb[5:9], 0xFFFFFFFF) // dictSize
	f.Add(bomb)

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = NewDictEncoder().DecodeStrings(data) // must not panic
	})
}

func FuzzLZ4Decode(f *testing.F) {
	if b, err := NewLZ4Encoder().EncodeStrings([]string{"hello", "world", "hello"}); err == nil {
		f.Add(b)
	}
	bomb := make([]byte, 13)
	bomb[0] = byte(EncodingLZ4)
	binary.LittleEndian.PutUint32(bomb[1:5], 0xFFFFFFFF) // count
	binary.LittleEndian.PutUint32(bomb[5:9], 0xFFFFFFFF) // uncompSize
	binary.LittleEndian.PutUint32(bomb[9:13], 0)         // compSize
	f.Add(bomb)

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = NewLZ4Encoder().DecodeStrings(data) // must not panic
	})
}

func FuzzGorillaDecode(f *testing.F) {
	if b, err := NewGorillaEncoder().EncodeFloat64s([]float64{1.5, 2.5, 2.5, 100.125}); err == nil {
		f.Add(b)
	}
	f.Add(gorillaNegativeTrailingInput())

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = NewGorillaEncoder().DecodeFloat64s(data) // must not panic
	})
}

func TestDeltaDecodeRejectsHugeCount(t *testing.T) {
	data := make([]byte, 13)
	data[0] = byte(EncodingDelta)
	binary.LittleEndian.PutUint32(data[1:5], 0xFFFFFFFF)

	_, err := NewDeltaEncoder().DecodeInt64s(data)
	if !errors.Is(err, ErrCorruptData) {
		t.Fatalf("want ErrCorruptData, got %v", err)
	}
}

func TestDeltaDecodeZeroCount(t *testing.T) {
	data := make([]byte, 13)
	data[0] = byte(EncodingDelta)
	// count = 0 must return empty, not panic on result[0].
	got, err := NewDeltaEncoder().DecodeInt64s(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want empty, got %v", got)
	}
}

func TestDictDecodeRejectsHugeSizes(t *testing.T) {
	data := make([]byte, 9)
	data[0] = byte(EncodingDict8)
	binary.LittleEndian.PutUint32(data[1:5], 0xFFFFFFFF)
	binary.LittleEndian.PutUint32(data[5:9], 0xFFFFFFFF)

	_, err := NewDictEncoder().DecodeStrings(data)
	if !errors.Is(err, ErrCorruptData) {
		t.Fatalf("want ErrCorruptData, got %v", err)
	}
}

func TestLZ4DecodeRejectsHugeUncompSize(t *testing.T) {
	data := make([]byte, 13)
	data[0] = byte(EncodingLZ4)
	binary.LittleEndian.PutUint32(data[1:5], 1)          // count
	binary.LittleEndian.PutUint32(data[5:9], 0xFFFFFFFF) // uncompSize
	binary.LittleEndian.PutUint32(data[9:13], 0)         // compSize

	_, err := NewLZ4Encoder().DecodeStrings(data)
	if !errors.Is(err, ErrCorruptData) {
		t.Fatalf("want ErrCorruptData, got %v", err)
	}
}

func TestGorillaDecodeRejectsNegativeTrailing(t *testing.T) {
	_, err := NewGorillaEncoder().DecodeFloat64s(gorillaNegativeTrailingInput())
	if !errors.Is(err, ErrCorruptData) {
		t.Fatalf("want ErrCorruptData, got %v", err)
	}
}

// gorillaNegativeTrailingInput builds a valid header followed by one XOR'd value
// whose leading(60)+meaningful(10) exceeds 64, so trailing goes negative. Without
// the guard this silently yields prev instead of erroring.
func gorillaNegativeTrailingInput() []byte {
	bw := newBitWriter(8)
	bw.writeBit(1)      // control: value differs from previous
	bw.writeBits(60, 6) // leading = 60
	bw.writeBits(10, 6) // meaningful = 10 -> trailing = 64-60-10 = -6
	bw.writeBits(0, 10) // the meaningful bits
	payload := bw.bytes()

	data := make([]byte, 13)
	data[0] = byte(EncodingGorilla)
	binary.LittleEndian.PutUint32(data[1:5], 2) // count = 2

	return append(data, payload...)
}
