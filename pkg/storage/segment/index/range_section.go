package index

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"math"

	bsi "github.com/RoaringBitmap/roaring/BitSliceIndexing"
)

const (
	// RangeBitmapMagic identifies a per-row-group range bitmap section.
	RangeBitmapMagic = "LSRB"
	// RangeSectionHeaderSize is the fixed LSRB section header size.
	RangeSectionHeaderSize = 16
	rangeBSILayoutBase2    = 0
)

// RangeSectionEncoder serializes a row group's range BSI columns.
type RangeSectionEncoder struct {
	w     io.Writer
	start int64
	n     uint16
	buf   bytes.Buffer
}

// NewRangeSectionEncoder creates an encoder that writes to w at startOffset.
func NewRangeSectionEncoder(w io.Writer, startOffset int64) *RangeSectionEncoder {
	return &RangeSectionEncoder{w: w, start: startOffset}
}

// AddColumn appends one column BSI to the section.
func (e *RangeSectionEncoder) AddColumn(name string, kind uint8, minValue, maxValue int64, idx *bsi.BSI) error {
	if idx == nil {
		return fmt.Errorf("index: encode range BSI column %q: nil BSI", name)
	}
	if e.n == math.MaxUint16 {
		return fmt.Errorf("index: encode range BSI column %q: too many columns", name)
	}
	nameBytes := []byte(name)
	if len(nameBytes) > math.MaxUint16 {
		return fmt.Errorf("index: encode range BSI column %q: name too long", name)
	}
	bitCount := idx.BitCount()
	if bitCount > math.MaxUint8 {
		return fmt.Errorf("index: encode range BSI column %q: bit count %d exceeds uint8", name, bitCount)
	}

	frames, err := idx.MarshalBinary()
	if err != nil {
		return fmt.Errorf("index: marshal range BSI column %q: %w", name, err)
	}
	var payload bytes.Buffer
	for _, frame := range frames {
		if len(frame) > math.MaxUint32 {
			return fmt.Errorf("index: encode range BSI column %q: frame too large", name)
		}
		var lenBuf [4]byte
		binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(frame)))
		payload.Write(lenBuf[:])
		payload.Write(frame)
	}
	if payload.Len() > math.MaxUint32 {
		return fmt.Errorf("index: encode range BSI column %q: payload too large", name)
	}

	var entry bytes.Buffer
	var lenBuf [2]byte
	binary.LittleEndian.PutUint16(lenBuf[:], uint16(len(nameBytes)))
	entry.Write(lenBuf[:])
	entry.Write(nameBytes)
	entry.WriteByte(rangeBSILayoutBase2)
	entry.WriteByte(byte(bitCount))
	var numBuf [8]byte
	binary.LittleEndian.PutUint64(numBuf[:], uint64(minValue))
	entry.Write(numBuf[:])
	binary.LittleEndian.PutUint64(numBuf[:], uint64(maxValue))
	entry.Write(numBuf[:])
	entry.WriteByte(kind)
	var payloadLen [4]byte
	binary.LittleEndian.PutUint32(payloadLen[:], uint32(payload.Len()))
	entry.Write(payloadLen[:])
	entry.Write(payload.Bytes())

	crc := crc32.ChecksumIEEE(entry.Bytes())
	var crcBuf [4]byte
	binary.LittleEndian.PutUint32(crcBuf[:], crc)
	entry.Write(crcBuf[:])

	e.buf.Write(entry.Bytes())
	e.n++
	return nil
}

// Finalize writes the section and returns its file offset and byte length.
func (e *RangeSectionEncoder) Finalize() (offset, length int64, err error) {
	section := make([]byte, 0, RangeSectionHeaderSize+e.buf.Len())
	section = append(section, RangeBitmapMagic...)
	section = append(section, 0, 0, 0, 0)
	section = binary.LittleEndian.AppendUint16(section, e.n)
	section = append(section, 0, 0, 0, 0, 0, 0)
	section = append(section, e.buf.Bytes()...)

	if _, err := e.w.Write(section); err != nil {
		return e.start, int64(len(section)), fmt.Errorf("index: write range BSI section: %w", err)
	}
	return e.start, int64(len(section)), nil
}
