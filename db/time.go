package db

import (
	"encoding/binary"
	"fmt"
	"time"
)

// SecondsBitOffset is the number of bits used to store subseconds in Sky time.
const SecondsBitOffset = 20

// ParseTime parses an ISO-8601 timestamp into Go time.
func ParseTime(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return t, fmt.Errorf("invalid timestamp: %q", s)
	}
	return t.UTC(), nil
}

// ShiftTime converts Go time into a Sky timestamp.
func ShiftTime(value time.Time) int64 {
	timestamp := value.UnixNano() / 1000
	usec := timestamp % 1000000
	sec := timestamp / 1000000
	return (sec << SecondsBitOffset) + usec
}

// ShiftTimeBytes converts Go time into a byte slice in Sky timestamp format.
func ShiftTimeBytes(value time.Time) []byte {
	var b [8]byte
	bs := b[:8]
	timestamp := ShiftTime(value)
	binary.BigEndian.PutUint64(bs, uint64(timestamp))
	return bs
}

// UnshiftTime converts a Sky timestamp into Go time.
func UnshiftTime(value int64) time.Time {
	usec := value & 0xFFFFF
	sec := value >> SecondsBitOffset
	return time.Unix(sec, usec*1000).UTC()
}

// UnshiftTimeBytes converts a byte slice containing a Sky timestamp to Go time.
func UnshiftTimeBytes(value []byte) time.Time {
	return UnshiftTime(int64(binary.BigEndian.Uint64(value)))
}
