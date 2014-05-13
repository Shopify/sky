package db

import (
	"encoding/binary"
	"fmt"
	"sort"
	"time"

	"github.com/boltdb/bolt"
)

// Tx represents a transaction.
type Tx struct {
	*bolt.Tx
	Table *Table
}

// PutMeta persists the meta data for the table.
func (tx *Tx) PutMeta() error {
	value, err := tx.Table.marshal()
	if err != nil {
		return fmt.Errorf("marshal: %s", err)
	}
	return tx.Bucket([]byte("meta")).Put([]byte("meta"), value)
}

// Shards returns a slice of shard buckets.
func (tx *Tx) Shards() []*bolt.Bucket {
	var buckets = make([]*bolt.Bucket, 0)
	for i := 0; i < tx.Table.shardCount; i++ {
		buckets = append(buckets, tx.Bucket(shardDBName(i)))
	}
	return buckets
}

// Properties retrieves a map of properties by property name.
func (tx *Tx) Properties() (map[string]*Property, error) {
	tx.Table.Lock()
	defer tx.Table.Unlock()
	if !tx.Table.opened() {
		return nil, fmt.Errorf("table not open: %s", tx.Table.name)
	}
	return tx.Table.properties, nil
}

// Properties retrieves a map of properties by property identifier.
func (tx *Tx) PropertiesByID() (map[int]*Property, error) {
	tx.Table.Lock()
	defer tx.Table.Unlock()
	if !tx.Table.opened() {
		return nil, fmt.Errorf("table not open: %s", tx.Table.name)
	}
	return tx.Table.propertiesByID, nil
}

// Property returns a single property from the table with the given name.
func (tx *Tx) Property(name string) (*Property, error) {
	tx.Table.Lock()
	defer tx.Table.Unlock()
	if !tx.Table.opened() {
		return nil, fmt.Errorf("table not open: %s", tx.Table.name)
	}
	return tx.Table.properties[name], nil
}

// PropertyByID returns a single property from the table by id.
func (tx *Tx) PropertyByID(id int) (*Property, error) {
	tx.Table.Lock()
	defer tx.Table.Unlock()
	if !tx.Table.opened() {
		return nil, fmt.Errorf("table not open: %s", tx.Table.name)
	}
	return tx.Table.propertiesByID[id], nil
}

// CreateProperty creates a new property on the table.
func (tx *Tx) CreateProperty(name string, dataType string, transient bool) (*Property, error) {
	tx.Table.Lock()
	defer tx.Table.Unlock()
	if !tx.Table.opened() {
		return nil, fmt.Errorf("table not open: %s", tx.Table.name)
	}

	// Don't allow duplicate names.
	if tx.Table.properties[name] != nil {
		return nil, fmt.Errorf("property already exists: %s", name)
	}

	// Create and validate property.
	p := &Property{
		table:     tx.Table,
		Name:      name,
		Transient: transient,
		DataType:  dataType,
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}

	// Retrieve the next property id.
	if transient {
		tx.Table.maxTransientID--
		p.ID = tx.Table.maxTransientID
	} else {
		tx.Table.maxPermanentID++
		p.ID = tx.Table.maxPermanentID
	}

	// Initialize factor database.
	if p.DataType == Factor {
		if _, err := tx.CreateBucketIfNotExists(factorDBName(p.ID)); err != nil {
			return nil, fmt.Errorf("factor: %s", err)
		}
	}

	// Add it to the collection.
	properties, propertiesByID := tx.Table.properties, tx.Table.propertiesByID
	tx.Table.copyProperties()
	tx.Table.properties[name] = p
	tx.Table.propertiesByID[p.ID] = p

	if err := tx.PutMeta(); err != nil {
		tx.Table.properties = properties
		tx.Table.propertiesByID = propertiesByID
		return nil, err
	}

	// Initialize the cache.
	if p.DataType == Factor {
		tx.Table.caches[p.ID] = newCache(FactorCacheSize)
	}

	return p, nil
}

// RenameProperty updates the name of a property.
func (tx *Tx) RenameProperty(oldName, newName string) (*Property, error) {
	tx.Table.Lock()
	defer tx.Table.Unlock()
	if !tx.Table.opened() {
		return nil, fmt.Errorf("table not open: %s", tx.Table.name)
	} else if tx.Table.properties[oldName] == nil {
		return nil, fmt.Errorf("property not found: %s", oldName)
	} else if tx.Table.properties[newName] != nil {
		return nil, fmt.Errorf("property already exists: %s", newName)
	}

	properties := tx.Table.properties
	tx.Table.copyProperties()
	p := tx.Table.properties[oldName].Clone()
	p.Name = newName
	delete(tx.Table.properties, oldName)
	tx.Table.properties[newName] = p

	if err := tx.PutMeta(); err != nil {
		tx.Table.properties = properties
		return nil, err
	}
	return p, nil
}

// DeleteProperty removes a single property from the table.
func (tx *Tx) DeleteProperty(name string) error {
	tx.Table.Lock()
	defer tx.Table.Unlock()
	if !tx.Table.opened() {
		return fmt.Errorf("table not open: %s", tx.Table.name)
	}

	p := tx.Table.properties[name]
	if p == nil {
		return fmt.Errorf("property not found: %s", name)
	}

	properties, propertiesByID := tx.Table.properties, tx.Table.propertiesByID
	tx.Table.copyProperties()
	delete(tx.Table.properties, name)
	delete(tx.Table.propertiesByID, p.ID)

	if err := tx.PutMeta(); err != nil {
		tx.Table.properties = properties
		tx.Table.propertiesByID = propertiesByID
		return err
	}
	return nil
}

// Event returns a single event for an object at a given timestamp.
func (tx *Tx) Event(id string, timestamp time.Time) (*Event, error) {
	tx.Table.Lock()
	defer tx.Table.Unlock()
	if !tx.Table.opened() {
		return nil, fmt.Errorf("table not open: %s", tx.Table.name)
	}

	rawEvent, err := tx.getRawEvent(id, ShiftTime(timestamp))
	if err != nil {
		return nil, err
	} else if rawEvent == nil {
		return nil, nil
	}

	return tx.toEvent(rawEvent)
}

// Events returns all events for an object in chronological order.
func (tx *Tx) Events(id string) ([]*Event, error) {
	tx.Table.Lock()
	defer tx.Table.Unlock()
	if !tx.Table.opened() {
		return nil, fmt.Errorf("table not open: %s", tx.Table.name)
	}

	// Retrieve raw events.
	rawEvents, err := tx.getRawEvents(id)
	if err != nil {
		return nil, err
	}

	// Convert to regular events and return.
	var events []*Event
	for _, rawEvent := range rawEvents {
		event, err := tx.toEvent(rawEvent)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

func (tx *Tx) getRawEvent(id string, timestamp int64) (*rawEvent, error) {
	if id == "" {
		return nil, ErrObjectIDRequired
	}

	var stat = bench()

	// Find object bucket.
	b := tx.Bucket(shardDBName(tx.Table.shardIndex(id))).Bucket([]byte(id))
	if b == nil {
		return nil, nil
	}

	// Find event.
	var key [8]byte
	binary.BigEndian.PutUint64(key[:], uint64(timestamp))

	value := b.Get(key[:])
	if value == nil {
		return nil, nil
	}
	stat.count++
	stat.apply(&tx.Table.stat.Event.Fetch.Count, &tx.Table.stat.Event.Fetch.Duration)

	// Unmarshal bytes into a raw event.
	stat = bench()
	e := &rawEvent{}
	if err := e.unmarshal(value); err != nil {
		return nil, err
	}
	stat.count++
	stat.apply(&tx.Table.stat.Event.Unmarshal.Count, &tx.Table.stat.Event.Unmarshal.Duration)

	return e, nil
}

func (tx *Tx) getRawEvents(id string) ([]*rawEvent, error) {
	if id == "" {
		return nil, ErrObjectIDRequired
	}

	// Find object's bucket.
	b := tx.Bucket(shardDBName(tx.Table.shardIndex(id))).Bucket([]byte(id))
	if b == nil {
		return nil, nil
	}

	// Loop over each event.
	var stat = bench()
	var slices [][]byte
	_ = b.ForEach(func(k, v []byte) error {
		slices = append(slices, v)
		return nil
	})
	stat.count += len(slices)
	stat.apply(&tx.Table.stat.Event.Fetch.Count, &tx.Table.stat.Event.Fetch.Duration)

	// Unmarshal each slice into a raw event.
	stat = bench()
	var events []*rawEvent
	for _, b := range slices {
		e := &rawEvent{}
		if err := e.unmarshal(b); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	stat.count += len(events)
	stat.apply(&tx.Table.stat.Event.Unmarshal.Count, &tx.Table.stat.Event.Unmarshal.Duration)
	return events, nil
}

// InsertEvent inserts an event for an object.
func (tx *Tx) InsertEvent(id string, event *Event) error {
	tx.Table.Lock()
	defer tx.Table.Unlock()
	if !tx.Table.opened() {
		return fmt.Errorf("table not open: %s", tx.Table.name)
	}
	return tx.insertEvent(id, event)
}

// InsertEvents inserts multiple events for an object.
func (tx *Tx) InsertEvents(id string, events []*Event) error {
	tx.Table.Lock()
	defer tx.Table.Unlock()
	if !tx.Table.opened() {
		return fmt.Errorf("table not open: %s", tx.Table.name)
	}
	for _, event := range events {
		if err := tx.insertEvent(id, event); err != nil {
			return err
		}
	}
	return nil
}

// InsertObjects inserts multiple sets of events for different objects.
func (tx *Tx) InsertObjects(objects map[string][]*Event) error {
	tx.Table.Lock()
	defer tx.Table.Unlock()
	if !tx.Table.opened() {
		return fmt.Errorf("table not open: %s", tx.Table.name)
	}
	for id, events := range objects {
		for _, event := range events {
			if err := tx.insertEvent(id, event); err != nil {
				return fmt.Errorf("insert objects error: %s", err)
			}
		}
	}
	return nil
}

func (tx *Tx) insertEvent(id string, e *Event) error {
	if id == "" {
		return ErrObjectIDRequired
	}

	// Convert to raw event.
	rawEvent, err := tx.toRawEvent(e)
	if err != nil {
		return fmt.Errorf("insert event error: %s", err)
	}

	// Insert event into appropriate shard.
	b, err := tx.Bucket(shardDBName(tx.Table.shardIndex(id))).CreateBucketIfNotExists([]byte(id))
	if err != nil {
		return err
	}

	// Marshal raw event into byte slice.
	var stat = bench()
	value, err := rawEvent.marshal()
	if err != nil {
		return err
	}
	stat.count++
	stat.apply(&tx.Table.stat.Event.Marshal.Count, &tx.Table.stat.Event.Marshal.Duration)

	// Insert key/value.
	stat = bench()
	var key = make([]byte, 8)
	binary.BigEndian.PutUint64(key, uint64(rawEvent.timestamp))

	if err := b.Put(key, value); err != nil {
		return err
	}

	stat.count++
	stat.apply(&tx.Table.stat.Event.Insert.Count, &tx.Table.stat.Event.Insert.Duration)
	return nil
}

// DeleteEvent removes a single event for an object at a given timestamp.
func (tx *Tx) DeleteEvent(id string, timestamp time.Time) error {
	tx.Table.Lock()
	defer tx.Table.Unlock()
	if !tx.Table.opened() {
		return fmt.Errorf("table not open: %s", tx.Table.name)
	}

	// Find object bucket.
	b := tx.Bucket(shardDBName(tx.Table.shardIndex(id))).Bucket([]byte(id))
	if b == nil {
		return nil
	}

	// Create the timestamp prefix.
	var stat = bench()
	var key = make([]byte, 8)
	binary.BigEndian.PutUint64(key, uint64(ShiftTime(timestamp)))

	// Delete the event.
	if err := b.Delete(key); err != nil {
		return err
	}

	stat.count++
	stat.apply(&tx.Table.stat.Event.Delete.Count, &tx.Table.stat.Event.Delete.Duration)
	return nil
}

// DeleteEvents removes all events for an object.
func (tx *Tx) DeleteEvents(id string) error {
	tx.Table.Lock()
	defer tx.Table.Unlock()
	if !tx.Table.opened() {
		return fmt.Errorf("table not open: %s", tx.Table.name)
	}

	// Delete object.
	return tx.Bucket(shardDBName(tx.Table.shardIndex(id))).DeleteBucket([]byte(id))
}

// Merge combines two existing objects together.
func (tx *Tx) Merge(destId, srcId string) error {
	panic("not implemented: Tx.Merge()")
	return nil
}

// Keys returns all object keys in the table in sorted order.
func (tx *Tx) Keys() []string {
	var keys = make([]string, 0)
	for i := 0; i < tx.Table.shardCount; i++ {
		_ = tx.Bucket(shardDBName(i)).ForEach(func(k, _ []byte) error {
			keys = append(keys, string(k))
			return nil
		})
	}
	sort.Strings(keys)
	return keys
}

// Factorize converts a factor property value to its integer index representation.
func (tx *Tx) Factorize(propertyID int, value string) (int, error) {
	tx.Table.Lock()
	defer tx.Table.Unlock()
	return tx.factorize(propertyID, value, false)
}

// factorize converts a factor property value to its integer index representation.
// Returns an error if the factor could not be found and createIfNotExists is false.
func (tx *Tx) factorize(propertyID int, value string, createIfNotExists bool) (int, error) {
	// Blank is always zero.
	if value == "" {
		return 0, nil
	}

	// Check the LRU first.
	if sequence, ok := tx.Table.caches[propertyID].getValue(value); ok {
		tx.Table.stat.Event.Factorize.CacheHit.Count++
		return sequence, nil
	}

	// Find an existing factor for the value.
	var stat = bench()
	stat.count++
	var val int
	if data := tx.Bucket(factorDBName(propertyID)).Get(factorKey(value)); data != nil {
		val = int(binary.BigEndian.Uint64(data))
	}
	if val != 0 {
		stat.apply(&tx.Table.stat.Event.Factorize.FetchHit.Count, &tx.Table.stat.Event.Factorize.FetchHit.Duration)
		tx.Table.caches[propertyID].add(value, val)
		return val, nil
	}
	stat.apply(&tx.Table.stat.Event.Factorize.FetchMiss.Count, &tx.Table.stat.Event.Factorize.FetchMiss.Duration)

	// Create a new factor if requested.
	if createIfNotExists {
		return tx.addFactor(propertyID, value)
	}

	return 0, nil
}

func (tx *Tx) addFactor(propertyID int, value string) (int, error) {
	var index int
	var stat = bench()

	// Look up next sequence index.
	var b = tx.Bucket(factorDBName(propertyID))
	index, err := b.NextSequence()
	if err != nil {
		return 0, err
	}

	// Store the value-to-index lookup.
	var data = make([]byte, 8)
	binary.BigEndian.PutUint64(data[:], uint64(index))
	if err := b.Put(factorKey(value), data); err != nil {
		return 0, fmt.Errorf("add factor txn put error: %s", err)
	}

	// Save the index-to-value lookup.
	if err := b.Put(reverseFactorKey(index), []byte(value)); err != nil {
		return 0, fmt.Errorf("add factor put reverse error: %s", err)
	}

	// Add to cache.
	tx.Table.caches[propertyID].add(value, index)

	stat.count++
	stat.apply(&tx.Table.stat.Event.Factorize.Create.Count, &tx.Table.stat.Event.Factorize.Create.Duration)

	return index, nil
}

// Defactorize converts a factor index to its actual value.
func (tx *Tx) Defactorize(propertyID int, index int) (string, error) {
	tx.Table.Lock()
	defer tx.Table.Unlock()
	return tx.defactorize(propertyID, index)
}

// defactorize converts a factor index to its string value.
func (tx *Tx) defactorize(propertyID int, index int) (string, error) {
	// Blank is always zero.
	if index == 0 {
		return "", nil
	}

	// Check the cache first.
	if key, ok := tx.Table.caches[propertyID].getKey(index); ok {
		tx.Table.stat.Event.Defactorize.CacheHit.Count++
		return key, nil
	}

	var stat = bench()
	stat.count++
	var data = tx.Bucket(factorDBName(propertyID)).Get(reverseFactorKey(index))
	if data == nil {
		stat.apply(&tx.Table.stat.Event.Defactorize.FetchMiss.Count, &tx.Table.stat.Event.Defactorize.FetchMiss.Duration)
		return "", fmt.Errorf("factor not found: %d: %d", propertyID, index)
	}
	stat.apply(&tx.Table.stat.Event.Defactorize.FetchHit.Count, &tx.Table.stat.Event.Defactorize.FetchHit.Duration)

	// Add to cache.
	tx.Table.caches[propertyID].add(string(data), index)

	return string(data), nil
}

// toRawEvent returns a raw event representation of this event.
func (tx *Tx) toRawEvent(e *Event) (*rawEvent, error) {
	rawEvent := &rawEvent{
		timestamp: ShiftTime(e.Timestamp),
		data:      make(map[int]interface{}),
	}

	// Map data by property id instead of name.
	for k, v := range e.Data {
		p := tx.Table.properties[k]
		if p == nil {
			return nil, fmt.Errorf("property not found: %s", k)
		}

		// Cast the value to the appropriate type.
		v = p.Cast(v)

		// Factorize value, if needed.
		if p.DataType == Factor {
			var err error
			v, err = tx.factorize(p.ID, v.(string), true)
			if err != nil {
				return nil, err
			}
		}

		rawEvent.data[p.ID] = v
	}

	return rawEvent, nil
}

// toEvent returns an normal event representation of this raw event.
func (tx *Tx) toEvent(e *rawEvent) (*Event, error) {
	event := &Event{
		Timestamp: UnshiftTime(e.timestamp),
		Data:      make(map[string]interface{}),
	}

	// Map data by name instead of property id.
	for k, v := range e.data {
		p := tx.Table.propertiesByID[k]

		// Missing properties have been deleted so just ignore.
		if p == nil {
			continue
		}

		// Cast the value to the appropriate type.
		v = promote(v)

		// Defactorize value, if needed.
		if p.DataType == Factor {
			var err error
			if intValue, ok := v.(int64); ok {
				v, err = tx.defactorize(p.ID, int(intValue))
				if err != nil {
					return nil, err
				}
			} else {
				return nil, fmt.Errorf("invalid factor value: %v", v)
			}
		}

		event.Data[p.Name] = v
	}

	return event, nil
}
