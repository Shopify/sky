package db

import (
	"testing"
	"time"

	_assert "github.com/stretchr/testify/assert"
)

// Ensure that Go time can be converted to Sky timestamps.
func TestShift(t *testing.T) {
	var timestamp time.Time
	timestamp, _ = time.Parse(time.RFC3339, "1970-01-01T00:00:00Z")
	_assert.Equal(t, ShiftTime(timestamp), int64(0))

	timestamp, _ = time.Parse(time.RFC3339, "1970-01-01T00:00:01Z")
	_assert.Equal(t, ShiftTime(timestamp), int64(0x100000))

	timestamp, _ = time.Parse(time.RFC3339, "1969-12-31T23:59:59Z")
	_assert.Equal(t, ShiftTime(timestamp), int64(-0x100000))

	timestamp, _ = time.Parse(time.RFC3339, "1970-01-01T00:00:01.5Z")
	_assert.Equal(t, ShiftTime(timestamp), int64(0x17a120))
}

// Ensure that Sky timestamps can be converted to Go time.
func TestUnshift(t *testing.T) {
	_assert.Equal(t, UnshiftTime(0).UTC().Format(time.RFC3339), "1970-01-01T00:00:00Z")
	_assert.Equal(t, UnshiftTime(0x100000).UTC().Format(time.RFC3339), "1970-01-01T00:00:01Z")
	_assert.Equal(t, UnshiftTime(-0x100000).UTC().Format(time.RFC3339), "1969-12-31T23:59:59Z")
	_assert.Equal(t, UnshiftTime(0x17a120).UTC().Format(time.RFC3339Nano), "1970-01-01T00:00:01.5Z")
}

// Ensure that timestamps can be converted between byte slices.
func TestShiftUnshiftBytes(t *testing.T) {
	timestamp, _ := time.Parse(time.RFC3339, "1970-01-01T00:00:01.5Z")
	_assert.Equal(t, UnshiftTimeBytes(ShiftTimeBytes(timestamp)).UTC().Format(time.RFC3339Nano), "1970-01-01T00:00:01.5Z")
}
