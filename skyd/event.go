package skyd

import (
	"github.com/ugorji/go-msgpack"
	"fmt"
	"io"
	"time"
)

// An Event is a state change or action that occurs at a particular
// point in time.
type Event struct {
	Timestamp time.Time
	Action    map[int64]interface{}
	Data      map[int64]interface{}
}

// Encodes an event to MsgPack format.
func (e *Event) EncodeRaw(writer io.Writer) error {
	raw := []interface{} {ShiftTime(e.Timestamp), e.Action, e.Data}
	encoder := msgpack.NewEncoder(writer)
	err := encoder.Encode(raw)
	return err
}

// Decodes an event from MsgPack format.
func (e *Event) DecodeRaw(reader io.Reader) error {
	raw := make([]interface{}, 3)
	decoder := msgpack.NewDecoder(reader, nil)
	err := decoder.Decode(&raw)
	if err != nil {
	  return err
	}

	// Convert the timestamp to int64.
	timestamp, err := castInt64(raw[0])
	if err != nil {
	  return fmt.Errorf("Unable to parse timestamp: '%v'", raw[0])
	}
  e.Timestamp = UnshiftTime(timestamp)

  // Convert action to appropriate map.
	e.Action, err = e.decodeRawMap(raw[1].(map[interface{}]interface{}))
	if err != nil {
	  return err
	}

  // Convert data to appropriate map.
	e.Data, err = e.decodeRawMap(raw[2].(map[interface{}]interface{}))
	if err != nil {
	  return err
	}

	return nil
}

// Decodes the action map.
func (e *Event) decodeRawMap(raw map[interface{}]interface{}) (map[int64]interface{}, error) {
  m := make(map[int64]interface{})
  for k,v := range raw {
    kInt64, err := castInt64(k)
    if err != nil {
      return nil, err
    }

    vInt64, err := castInt64(v)
    if err == nil {
      m[kInt64] = vInt64
    } else {
      m[kInt64] = v
    }
  }
  return m, nil
}