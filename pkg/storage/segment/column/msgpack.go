package column

import (
	"bytes"
	"fmt"

	"github.com/vmihailenco/msgpack/v5"
)

// MsgpackCellEncoder encodes a slice of msgpack-serialized cell blobs into a
// single column chunk. The wire format reuses the LZ4 string encoder internally
// (length-prefixed cells) but wraps it with a distinct EncodingType marker so
// the reader can reconstruct typed Values instead of plain strings.
//
// Each cell is independently msgpack-encoded by the caller into a []byte before
// calling EncodeCells. This keeps the column encoder agnostic of event.Value.
type MsgpackCellEncoder struct{}

// NewMsgpackCellEncoder creates a new encoder.
func NewMsgpackCellEncoder() *MsgpackCellEncoder {
	return &MsgpackCellEncoder{}
}

// EncodeCells encodes a slice of per-cell msgpack blobs into a column chunk.
// Empty/nil cells represent null values.
//
// Wire format: [1B encoding marker][cells via LZ4-style length-prefix]
// The actual layer-1 encoding delegates to the LZ4 string encoder which
// stores count + length-prefixed strings. We pass each cell's raw bytes
// as a "string" element.
func (e *MsgpackCellEncoder) EncodeCells(cells [][]byte, encodingMarker EncodingType) ([]byte, error) {
	if len(cells) == 0 {
		return nil, ErrEmptyInput
	}
	// Convert [][]byte to []string for the LZ4 encoder.
	strs := make([]string, len(cells))
	for i, c := range cells {
		strs[i] = string(c) // null cells become ""
	}
	enc := NewLZ4Encoder()
	data, err := enc.EncodeStrings(strs)
	if err != nil {
		return nil, fmt.Errorf("column.MsgpackCellEncoder: %w", err)
	}
	// Replace the LZ4 encoding marker byte with our marker.
	data[0] = byte(encodingMarker)
	return data, nil
}

// DecodeCells decodes a column chunk back into per-cell msgpack blobs.
// Empty strings become nil cells (null).
func (e *MsgpackCellEncoder) DecodeCells(data []byte) ([][]byte, error) {
	if len(data) == 0 {
		return nil, ErrEmptyInput
	}
	// Temporarily restore the LZ4 marker for the LZ4 decoder.
	saved := data[0]
	data[0] = byte(EncodingLZ4)
	strs, err := NewLZ4Encoder().DecodeStrings(data)
	data[0] = saved // restore
	if err != nil {
		return nil, fmt.Errorf("column.MsgpackCellEncoder.DecodeCells: %w", err)
	}
	cells := make([][]byte, len(strs))
	for i, s := range strs {
		if s != "" {
			cells[i] = []byte(s)
		}
	}
	return cells, nil
}

// msgpackCellValue mirrors the spillValue struct used by the spill encoder.
// It is the recursive, type-tagged encoding for a single event.Value cell.
// The tag values match FieldType constants (append-only, stable).
//
// NOTE: This struct must stay in sync with spillValue in pipeline/spill.go.
// Both use the same wire format so that segments and spill files can be decoded
// by the same logic. The duplication is intentional: the segment package must
// not import the pipeline package.
// MsgpackCellValue is the recursive, type-tagged encoding for a single event.Value cell.
type MsgpackCellValue struct {
	Type uint8                       `msgpack:"t"`
	Str  string                      `msgpack:"s,omitempty"`
	Num  int64                       `msgpack:"n,omitempty"`
	Flt  float64                     `msgpack:"f,omitempty"`
	Arr  []MsgpackCellValue          `msgpack:"a,omitempty"`
	Obj  map[string]MsgpackCellValue `msgpack:"o,omitempty"`
}

// EncodeMsgpackCell serializes a MsgpackCellValue to bytes.
func EncodeMsgpackCell(v *MsgpackCellValue) ([]byte, error) {
	var buf bytes.Buffer
	enc := msgpack.NewEncoder(&buf)
	enc.SetSortMapKeys(true) // deterministic output
	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("column.EncodeMsgpackCell: %w", err)
	}
	return buf.Bytes(), nil
}

// DecodeMsgpackCell deserializes a MsgpackCellValue from bytes.
func DecodeMsgpackCell(data []byte) (*MsgpackCellValue, error) {
	var v MsgpackCellValue
	dec := msgpack.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("column.DecodeMsgpackCell: %w", err)
	}
	return &v, nil
}
