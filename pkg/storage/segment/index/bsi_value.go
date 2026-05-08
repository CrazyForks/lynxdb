package index

import "math"

const (
	// RangeBSIValueInt stores signed integer values.
	RangeBSIValueInt uint8 = iota
	// RangeBSIValueFloat64Bits stores order-preserving float64 bit encodings.
	RangeBSIValueFloat64Bits
	// RangeBSIValueTimestampNS stores Unix nanoseconds.
	RangeBSIValueTimestampNS
	// RangeBSIValueBool stores false as 0 and true as 1.
	RangeBSIValueBool
)

// FloatToOrderedInt64 maps a float64 to an int64 whose ordering matches
// float ordering for all non-NaN values.
func FloatToOrderedInt64(f float64) int64 {
	bits := math.Float64bits(f)
	if bits&(1<<63) != 0 {
		return int64(^bits)
	}
	return int64(bits ^ (1 << 63))
}

// OrderedInt64ToFloat reverses FloatToOrderedInt64.
func OrderedInt64ToFloat(v int64) float64 {
	bits := uint64(v)
	if bits&(1<<63) == 0 {
		return math.Float64frombits(^bits)
	}
	return math.Float64frombits(bits ^ (1 << 63))
}

// BoolToInt64 maps a boolean to the integer domain used by Range BSI.
func BoolToInt64(v bool) int64 {
	if v {
		return 1
	}
	return 0
}
