package db

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/skydb/sky/hash"
	"github.com/szferi/gomdb"
	"github.com/ugorji/go/codec"
)

// maxKeySize is the size, in bytes, of the largest key that can be inserted.
// This is a limitation of LMDB.
const maxKeySize = 500

// FactorCacheSize is the number of factors that are stored in the LRU cache.
// This cache size is per-property.
const FactorCacheSize = 1000

var (
	// ErrObjectIDRequired is returned inserting, deleting, or retrieving
	// event data without specifying an object identifier.
	ErrObjectIDRequired = errors.New("object id required")
)

// Table represents a collection of objects.
type Table struct {
	sync.RWMutex

	db             *DB
	name           string
	path           string
	properties     map[string]*Property
	propertiesByID map[int]*Property
	env            *mdb.Env
	caches         map[int]*cache
	stat           Stat

	shardCount     int
	maxPermanentID int
	maxTransientID int

	noSync     bool
	maxDBs     uint
	maxReaders uint
}

// Name returns the name of the table.
func (t *Table) Name() string {
	return t.name
}

// Path returns the location of the table on disk.
func (t *Table) Path() string {
	return t.path
}

// ShardCount returns the number of shards in the table.
func (t *Table) ShardCount() int {
	return t.shardCount
}

// Exists returns whether the table exists.
func (t *Table) Exists() bool {
	_, err := os.Stat(t.path)
	return !os.IsNotExist(err)
}

func (t *Table) create() error {
	t.Lock()
	defer t.Unlock()

	// Create directory.
	if err := os.MkdirAll(t.path, 0700); err != nil {
		return err
	}

	// Set initial shard count.
	t.shardCount = 16
	if t.shardCount == 0 {
		t.shardCount = runtime.NumCPU()
	}

	// Open the table.
	if err := t._open(); err != nil {
		return err
	}

	// Save initial table state.
	if err := t.save(); err != nil {
		return err
	}

	return nil
}

// open opens and initializes the table.
func (t *Table) open() error {
	t.Lock()
	defer t.Unlock()
	return t._open()
}

func (t *Table) _open() error {
	if t.env != nil {
		return nil
	} else if !t.Exists() {
		return fmt.Errorf("table not found: %s", t.name)
	}

	// Initialize directory.
	if err := os.MkdirAll(t.path, 0700); err != nil {
		return fmt.Errorf("table mkdir error: %s", err)
	}

	// Create LMDB environment.
	env, err := mdb.NewEnv()
	assert(err == nil, "table env error: %v", err)

	// LMDB environment settings.
	err = env.SetMaxDBs(mdb.DBI(t.maxDBs))
	assert(err == nil, "max dbs (%d) error: %s", t.maxDBs, err)
	err = env.SetMaxReaders(t.maxReaders)
	assert(err == nil, "max readers (%d) error: %s", t.maxReaders, err)
	err = env.SetMapSize(1 << 38)
	assert(err == nil, "map size error: %s", err)

	// Set LMDB flags.
	options := uint(mdb.NOTLS)
	if t.noSync {
		options |= mdb.NOSYNC
	}

	// Open the LMDB environment.
	if err := env.Open(t.path, options, 0600); err != nil {
		env.Close()
		return fmt.Errorf("lmdb open error: " + err.Error())
	}
	t.env = env

	// Create standard tables.
	err = t.txn(0, func(txn *transaction) error {
		err := txn.dbi("meta", 0)
		assert(err == nil, "meta dbi error: %v", err)

		// Initialize shards.
		for i := 0; i < t.shardCount; i++ {
			err := txn.dbi(shardDBName(i), mdb.DUPSORT)
			assert(err == nil, "shard dbi error: %v", err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Load the metadata.
	if err := t.load(); err != nil {
		t._close()
		return err
	}

	// Initialize factor databases.
	err = t.txn(0, func(txn *transaction) error {
		for _, p := range t.properties {
			if p.DataType != Factor {
				continue
			}
			err := txn.dbi(factorDBName(p.ID), 0)
			assert(err == nil, "factor dbi error: %v", err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Initialize the factor caches.
	t.caches = make(map[int]*cache)
	for _, p := range t.properties {
		if p.DataType == Factor {
			t.caches[p.ID] = newCache(FactorCacheSize)
		}
	}

	return nil
}

// drop closes and removes the table.
func (t *Table) drop() error {
	t.Lock()
	defer t.Unlock()

	// Close table and delete everything.
	t._close()
	if err := os.RemoveAll(t.path); err != nil {
		return fmt.Errorf("remove all error: %s", err)
	}

	return nil
}

// opened returned whether the table is currently open.
func (t *Table) opened() bool {
	return t.env != nil
}

func (t *Table) close() {
	t.Lock()
	defer t.Unlock()
	t._close()
}

func (t *Table) _close() {
	if t.env != nil {
		t.env.Close()
		t.env = nil
	}
}

func (t *Table) load() error {
	return t.txn(mdb.RDONLY, func(txn *transaction) error {
		value, err := txn.get("meta", []byte("meta"))
		if err != nil {
			return fmt.Errorf("table meta error: %s", err)
		} else if len(value) == 0 {
			return nil
		}
		// warnf("unmarshal: %s", string(value))
		if err := t.unmarshal(value); err != nil {
			return err
		}
		return nil
	})
}

func (t *Table) save() error {
	return t.txn(0, func(txn *transaction) error {
		value, err := t.marshal()
		// warnf("marshal: %s", string(value))
		assert(err == nil, "table marshal error: %v", err)
		return txn.put("meta", []byte("meta"), value)
	})
}

// Properties retrieves a map of properties by property name.
func (t *Table) Properties() (map[string]*Property, error) {
	t.Lock()
	defer t.Unlock()
	if !t.opened() {
		return nil, fmt.Errorf("table not open: %s", t.name)
	}
	return t.properties, nil
}

// Properties retrieves a map of properties by property identifier.
func (t *Table) PropertiesByID() (map[int]*Property, error) {
	t.Lock()
	defer t.Unlock()
	if !t.opened() {
		return nil, fmt.Errorf("table not open: %s", t.name)
	}
	return t.propertiesByID, nil
}

// Property returns a single property from the table with the given name.
func (t *Table) Property(name string) (*Property, error) {
	t.Lock()
	defer t.Unlock()
	if !t.opened() {
		return nil, fmt.Errorf("table not open: %s", t.name)
	}
	return t.properties[name], nil
}

// PropertyByID returns a single property from the table by id.
func (t *Table) PropertyByID(id int) (*Property, error) {
	t.Lock()
	defer t.Unlock()
	if !t.opened() {
		return nil, fmt.Errorf("table not open: %s", t.name)
	}
	return t.propertiesByID[id], nil
}

// CreateProperty creates a new property on the table.
func (t *Table) CreateProperty(name string, dataType string, transient bool) (*Property, error) {
	t.Lock()
	defer t.Unlock()
	if !t.opened() {
		return nil, fmt.Errorf("table not open: %s", t.name)
	}

	// Don't allow duplicate names.
	if t.properties[name] != nil {
		return nil, fmt.Errorf("property already exists: %s", name)
	}

	// Create and validate property.
	p := &Property{
		table:     t,
		Name:      name,
		Transient: transient,
		DataType:  dataType,
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}

	// Retrieve the next property id.
	if transient {
		t.maxTransientID--
		p.ID = t.maxTransientID
	} else {
		t.maxPermanentID++
		p.ID = t.maxPermanentID
	}

	// Initialize factor database.
	if p.DataType == Factor {
		err := t.txn(0, func(txn *transaction) error {
			err := txn.dbi(factorDBName(p.ID), 0)
			assert(err == nil, "factor dbi error: %v", err)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	// Add it to the collection.
	properties, propertiesByID := t.properties, t.propertiesByID
	t.copyProperties()
	t.properties[name] = p
	t.propertiesByID[p.ID] = p

	if err := t.save(); err != nil {
		t.properties = properties
		t.propertiesByID = propertiesByID
		return nil, err
	}

	// Initialize the cache.
	if p.DataType == Factor {
		t.caches[p.ID] = newCache(FactorCacheSize)
	}

	return p, nil
}

// RenameProperty updates the name of a property.
func (t *Table) RenameProperty(oldName, newName string) (*Property, error) {
	t.Lock()
	defer t.Unlock()
	if !t.opened() {
		return nil, fmt.Errorf("table not open: %s", t.name)
	} else if t.properties[oldName] == nil {
		return nil, fmt.Errorf("property not found: %s", oldName)
	} else if t.properties[newName] != nil {
		return nil, fmt.Errorf("property already exists: %s", newName)
	}

	properties := t.properties
	t.copyProperties()
	p := t.properties[oldName].Clone()
	p.Name = newName
	delete(t.properties, oldName)
	t.properties[newName] = p

	if err := t.save(); err != nil {
		t.properties = properties
		return nil, err
	}
	return p, nil
}

// DeleteProperty removes a single property from the table.
func (t *Table) DeleteProperty(name string) error {
	t.Lock()
	defer t.Unlock()
	if !t.opened() {
		return fmt.Errorf("table not open: %s", t.name)
	}

	p := t.properties[name]
	if p == nil {
		return fmt.Errorf("property not found: %s", name)
	}

	properties, propertiesByID := t.properties, t.propertiesByID
	t.copyProperties()
	delete(t.properties, name)
	delete(t.propertiesByID, p.ID)

	if err := t.save(); err != nil {
		t.properties = properties
		t.propertiesByID = propertiesByID
		return err
	}
	return nil
}

// copyProperties creates a new map and copies all existing properties.
func (t *Table) copyProperties() {
	properties := make(map[string]*Property)
	for k, v := range t.properties {
		properties[k] = v
	}
	t.properties = properties

	propertiesByID := make(map[int]*Property)
	for k, v := range t.propertiesByID {
		propertiesByID[k] = v
	}
	t.propertiesByID = propertiesByID
}

// GetEvent returns a single event for an object at a given timestamp.
func (t *Table) GetEvent(id string, timestamp time.Time) (*Event, error) {
	t.Lock()
	defer t.Unlock()
	if !t.opened() {
		return nil, fmt.Errorf("table not open: %s", t.name)
	}

	rawEvent, err := t.getRawEvent(id, shiftTime(timestamp))
	if err != nil {
		return nil, err
	} else if rawEvent == nil {
		return nil, nil
	}

	return t.toEvent(rawEvent)
}

// GetEvents returns all events for an object in chronological order.
func (t *Table) GetEvents(id string) ([]*Event, error) {
	t.Lock()
	defer t.Unlock()
	if !t.opened() {
		return nil, fmt.Errorf("table not open: %s", t.name)
	}

	// Retrieve raw events.
	rawEvents, err := t.getRawEvents(id)
	if err != nil {
		return nil, err
	}

	// Convert to regular events and return.
	var events []*Event
	for _, rawEvent := range rawEvents {
		event, err := t.toEvent(rawEvent)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

func (t *Table) getRawEvent(id string, timestamp int64) (*rawEvent, error) {
	if id == "" {
		return nil, ErrObjectIDRequired
	}

	var stat = bench()
	var buf bytes.Buffer
	binary.Write(&buf, binary.BigEndian, timestamp)
	prefix := buf.Bytes()

	// Retrieve event bytes from the database.
	var b []byte
	err := t.txn(mdb.RDONLY, func(txn *transaction) error {
		var err error
		b, err = txn.getAt(shardDBName(t.shardIndex(id)), []byte(id), prefix)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("get raw event error: %s", err)
	} else if b == nil {
		return nil, nil
	}
	stat.count++
	stat.apply(&t.stat.Event.Fetch.Count, &t.stat.Event.Fetch.Duration)

	// Unmarshal bytes into a raw event.
	stat = bench()
	e := &rawEvent{}
	if err := e.unmarshal(b); err != nil {
		return nil, err
	}
	stat.count++
	stat.apply(&t.stat.Event.Unmarshal.Count, &t.stat.Event.Unmarshal.Duration)

	return e, nil
}

func (t *Table) getRawEvents(id string) ([]*rawEvent, error) {
	if id == "" {
		return nil, ErrObjectIDRequired
	}

	// Retrieve all bytes from the database.
	var stat = bench()
	var slices [][]byte
	err := t.txn(mdb.RDONLY, func(txn *transaction) error {
		var err error
		slices, err = txn.getAll(shardDBName(t.shardIndex(id)), []byte(id))
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("get raw events error: %s", err)
	} else if slices == nil {
		return nil, nil
	}
	stat.count += len(slices)
	stat.apply(&t.stat.Event.Fetch.Count, &t.stat.Event.Fetch.Duration)

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
	stat.apply(&t.stat.Event.Unmarshal.Count, &t.stat.Event.Unmarshal.Duration)
	return events, nil
}

// InsertEvent inserts an event for an object.
func (t *Table) InsertEvent(id string, event *Event) error {
	t.Lock()
	defer t.Unlock()
	if !t.opened() {
		return fmt.Errorf("table not open: %s", t.name)
	}
	return t.insertEvent(id, event)
}

// InsertEvents inserts multiple events for an object.
func (t *Table) InsertEvents(id string, events []*Event) error {
	t.Lock()
	defer t.Unlock()
	if !t.opened() {
		return fmt.Errorf("table not open: %s", t.name)
	}
	for _, event := range events {
		if err := t.insertEvent(id, event); err != nil {
			return err
		}
	}
	return nil
}

// InsertObjects inserts multiple sets of events for different objects.
func (t *Table) InsertObjects(objects map[string][]*Event) error {
	t.Lock()
	defer t.Unlock()
	if !t.opened() {
		return fmt.Errorf("table not open: %s", t.name)
	}
	for id, events := range objects {
		for _, event := range events {
			if err := t.insertEvent(id, event); err != nil {
				return fmt.Errorf("insert objects error: %s", err)
			}
		}
	}
	return nil
}

func (t *Table) insertEvent(id string, e *Event) error {
	if id == "" {
		return ErrObjectIDRequired
	}
	// Convert to raw event.
	rawEvent, err := t.toRawEvent(e)
	if err != nil {
		return fmt.Errorf("insert event error: %s", err)
	}

	// Retrieve existing event for object at the same moment and merge.
	current, err := t.getRawEvent(id, rawEvent.timestamp)
	if current != nil {
		data := current.data
		for k, v := range rawEvent.data {
			data[k] = v
		}
		rawEvent.data = data
	}

	// Marshal raw event into byte slice.
	var stat = bench()
	b, err := rawEvent.marshal()
	if err != nil {
		return err
	}
	stat.count++
	stat.apply(&t.stat.Event.Marshal.Count, &t.stat.Event.Marshal.Duration)

	// Create the timestamp prefix.
	stat = bench()
	var buf bytes.Buffer
	binary.Write(&buf, binary.BigEndian, rawEvent.timestamp)
	prefix := buf.Bytes()

	// Insert event into appropriate shard.
	err = t.txn(0, func(txn *transaction) error {
		if err := txn.putAt(shardDBName(t.shardIndex(id)), []byte(id), prefix, b); err != nil {
			return fmt.Errorf("insert event put error: %s", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("insert event txn error: %s", err)
	}
	stat.count++
	stat.apply(&t.stat.Event.Insert.Count, &t.stat.Event.Insert.Duration)
	return nil
}

// DeleteEvent removes a single event for an object at a given timestamp.
func (t *Table) DeleteEvent(id string, timestamp time.Time) error {
	t.Lock()
	defer t.Unlock()
	if !t.opened() {
		return fmt.Errorf("table not open: %s", t.name)
	}

	// Create the timestamp prefix.
	var stat = bench()
	var buf bytes.Buffer
	binary.Write(&buf, binary.BigEndian, shiftTime(timestamp))
	prefix := buf.Bytes()

	// Delete the event.
	err := t.txn(0, func(txn *transaction) error {
		return txn.delAt(shardDBName(t.shardIndex(id)), []byte(id), prefix)
	})
	if err != nil {
		return fmt.Errorf("delete event txn error: %s", err)
	}
	stat.count++
	stat.apply(&t.stat.Event.Delete.Count, &t.stat.Event.Delete.Duration)
	return err
}

// DeleteEvents removes all events for an object.
func (t *Table) DeleteEvents(id string) error {
	t.Lock()
	defer t.Unlock()
	if !t.opened() {
		return fmt.Errorf("table not open: %s", t.name)
	}

	// Delete all events.
	err := t.txn(0, func(txn *transaction) error {
		return txn.del(shardDBName(t.shardIndex(id)), []byte(id))
	})
	if err != nil {
		return fmt.Errorf("delete events txn error: %s", err)
	}
	return nil
}

// Merge combines two existing objects together.
func (t *Table) Merge(destId, srcId string) error {
	panic("not implemented: Table.Merge()")
	return nil
}

// ForEach executes a function once for each shard in the table.
// A different cursor is passed in for each function invocation.
//
// IMPORTANT: The function is responsible for closing the cursor.
func (t *Table) ForEach(fn func(c *Cursor)) error {
	if !t.opened() {
		return fmt.Errorf("table not open: %s", t.name)
	}
	for i := 0; i < t.shardCount; i++ {
		txn, err := t.env.BeginTxn(nil, mdb.RDONLY)
		if err != nil {
			return fmt.Errorf("foreach txn error: %s", err)
		}

		shardDBName := shardDBName(i)
		dbi, err := txn.DBIOpen(&shardDBName, 0)
		if err != nil {
			return fmt.Errorf("foreach dbi error: %s", err)
		}

		c, err := txn.CursorOpen(dbi)
		if err != nil {
			return fmt.Errorf("foreach cursor error: %s", err)
		}

		fn(&Cursor{c})
	}

	return nil
}

// Keys returns all object keys in the table in sorted order.
func (t *Table) Keys() ([]string, error) {
	var keys = make([]string, 0)
	err := t.ForEach(func(c *Cursor) {
		for k, _, err := c.Get(nil, mdb.NEXT_NODUP); err != mdb.NotFound; k, _, err = c.Get(nil, mdb.NEXT_NODUP) {
			keys = append(keys, string(k))
		}
	})
	if err != nil {
		return nil, fmt.Errorf("keys error: %s", err)
	}
	sort.Strings(keys)
	return keys, nil
}

// Factorize converts a factor property value to its integer index representation.
func (t *Table) Factorize(propertyID int, value string) (int, error) {
	t.Lock()
	defer t.Unlock()
	return t.factorize(propertyID, value, false)
}

// factorize converts a factor property value to its integer index representation.
// Returns an error if the factor could not be found and createIfNotExists is false.
func (t *Table) factorize(propertyID int, value string, createIfNotExists bool) (int, error) {
	// Blank is always zero.
	if value == "" {
		return 0, nil
	}

	// Check the LRU first.
	if sequence, ok := t.caches[propertyID].getValue(value); ok {
		t.stat.Event.Factorize.CacheHit.Count++
		return sequence, nil
	}

	// Find an existing factor for the value.
	var stat = bench()
	stat.count++
	var val int
	err := t.txn(mdb.RDONLY, func(txn *transaction) error {
		data, err := txn.get(factorDBName(propertyID), factorKey(value))
		if err != nil {
			return fmt.Errorf("factorize txn get error: %s", err)
		} else if data != nil {
			val = int(binary.BigEndian.Uint64(data))
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("factorize txn error: %s", err)
	} else if val != 0 {
		stat.apply(&t.stat.Event.Factorize.FetchHit.Count, &t.stat.Event.Factorize.FetchHit.Duration)
		t.caches[propertyID].add(value, val)
		return val, nil
	}
	stat.apply(&t.stat.Event.Factorize.FetchMiss.Count, &t.stat.Event.Factorize.FetchMiss.Duration)

	// Create a new factor if requested.
	if createIfNotExists {
		return t.addFactor(propertyID, value)
	}

	return 0, nil
}

func (t *Table) addFactor(propertyID int, value string) (int, error) {
	var index int
	var stat = bench()
	err := t.txn(0, func(txn *transaction) error {
		// Look up next sequence index.
		data, err := txn.get(factorDBName(propertyID), []byte("+"))
		if err != nil {
			return fmt.Errorf("add factor txn get error: %s", err)
		} else if data == nil {
			data = make([]byte, 8)
		}

		// Read identifier and increment.
		index = int(binary.BigEndian.Uint64(data))
		index += 1

		// Save incremented index.
		binary.BigEndian.PutUint64(data, uint64(index))
		if err = txn.put(factorDBName(propertyID), []byte("+"), data); err != nil {
			return fmt.Errorf("add factor txn get error: %s", err)
		}

		// Truncate the value so it fits in our max key size.
		value = truncateFactor(value)

		// Store the value-to-index lookup.
		binary.BigEndian.PutUint64(data[:], uint64(index))
		if err := txn.put(factorDBName(propertyID), factorKey(value), data); err != nil {
			return fmt.Errorf("add factor txn put error: %s", err)
		}

		// Save the index-to-value lookup.
		if err := txn.put(factorDBName(propertyID), reverseFactorKey(index), []byte(value)); err != nil {
			return fmt.Errorf("add factor put reverse error: %s", err)
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("add factor error: %s", err)
	}

	// Add to cache.
	t.caches[propertyID].add(value, index)

	stat.count++
	stat.apply(&t.stat.Event.Factorize.Create.Count, &t.stat.Event.Factorize.Create.Duration)

	return index, nil
}

// Defactorize converts a factor index to its actual value.
func (t *Table) Defactorize(propertyID int, index int) (string, error) {
	t.Lock()
	defer t.Unlock()
	return t.defactorize(propertyID, index)
}

// defactorize converts a factor index to its string value.
func (t *Table) defactorize(propertyID int, index int) (string, error) {
	// Blank is always zero.
	if index == 0 {
		return "", nil
	}

	// Check the cache first.
	if key, ok := t.caches[propertyID].getKey(index); ok {
		t.stat.Event.Defactorize.CacheHit.Count++
		return key, nil
	}

	var stat = bench()
	stat.count++
	var data []byte
	err := t.txn(mdb.RDONLY, func(txn *transaction) error {
		var err error
		data, err = txn.get(factorDBName(propertyID), reverseFactorKey(index))
		return err
	})
	if err != nil {
		return "", fmt.Errorf("defactorize error: %s", err)
	} else if data == nil {
		stat.apply(&t.stat.Event.Defactorize.FetchMiss.Count, &t.stat.Event.Defactorize.FetchMiss.Duration)
		return "", fmt.Errorf("factor not found: %d: %d", propertyID, index)
	}
	stat.apply(&t.stat.Event.Defactorize.FetchHit.Count, &t.stat.Event.Defactorize.FetchHit.Duration)

	// Add to cache.
	t.caches[propertyID].add(string(data), index)

	return string(data), nil
}

// shardIndex returns the appropriate shard for a given object id.
func (t *Table) shardIndex(id string) int {
	return int(hash.Local(id)) % t.shardCount
}

// shardDBName returns the name of the shard table.
func shardDBName(index int) string {
	return fmt.Sprintf("shards/%d", index)
}

// factorDBName returns the name of the factor table for a property.
func factorDBName(propertyID int) string {
	return fmt.Sprintf("factors/%d", propertyID)
}

// factorKey returns the value-to-index key.
func factorKey(value string) []byte {
	return []byte(fmt.Sprintf(">%s", truncateFactor(value)))
}

// reverseFactorKey returns the index-to-value key.
func reverseFactorKey(index int) []byte {
	return []byte(fmt.Sprintf("<%d", index))
}

// truncateFactor returns the value that can be saved to the factorizer because
// of LMDB key size restrictions.
func truncateFactor(value string) string {
	if len(value) > maxKeySize {
		return value[0:maxKeySize]
	}
	return value
}

// Stat returns statistics for the table's underlying LMDB environment.
func (t *Table) Stat() (*Stat, error) {
	stat, err := t.env.Stat()
	if err != nil {
		return nil, fmt.Errorf("txn stat error: %s", err)
	}
	info, err := t.env.Info()
	if err != nil {
		return nil, err
	}
	s := &Stat{}
	*s = t.stat
	s.Entries = stat.Entries
	s.Size = info.MapSize
	s.Depth = stat.Depth
	s.Transactions.Last = info.LastTxnID
	s.Readers.Max = info.MaxReaders
	s.Readers.Current = info.NumReaders
	s.Pages.Last = info.LastPNO
	s.Pages.Size = stat.PSize
	s.Pages.Branch = stat.BranchPages
	s.Pages.Leaf = stat.LeafPages
	s.Pages.Overflow = stat.OverflowPages
	return s, nil
}

// marshal encodes the table into a byte slice.
func (t *Table) marshal() ([]byte, error) {
	var msg = tableRawMessage{Name: t.name, ShardCount: t.shardCount, MaxPermanentID: t.maxPermanentID, MaxTransientID: t.maxTransientID}
	for _, p := range t.properties {
		msg.Properties = append(msg.Properties, p)
	}
	return json.Marshal(msg)
}

// unmarshal decodes a byte slice into a table.
func (t *Table) unmarshal(data []byte) error {
	var msg tableRawMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return err
	}
	t.name = msg.Name
	t.maxPermanentID = msg.MaxPermanentID
	t.maxTransientID = msg.MaxTransientID
	t.shardCount = msg.ShardCount

	t.properties = make(map[string]*Property)
	t.propertiesByID = make(map[int]*Property)
	for _, p := range msg.Properties {
		p.table = t
		t.properties[p.Name] = p
		t.propertiesByID[p.ID] = p
	}

	return nil
}

// txn executes a function within the context of a LMDB transaction.
func (t *Table) txn(flags uint, fn func(*transaction) error) error {
	txn, err := t.env.BeginTxn(nil, flags)
	if err != nil {
		return fmt.Errorf("txn error: %s", err)
	}
	if err := fn(&transaction{txn}); err != nil {
		txn.Abort()
		return err
	}
	if err := txn.Commit(); err != nil {
		return fmt.Errorf("txn commit error: %s", err)
	}
	return nil
}

// toRawEvent returns a raw event representation of this event.
func (t *Table) toRawEvent(e *Event) (*rawEvent, error) {
	rawEvent := &rawEvent{
		timestamp: shiftTime(e.Timestamp),
		data:      make(map[int]interface{}),
	}

	// Map data by property id instead of name.
	for k, v := range e.Data {
		p := t.properties[k]
		if p == nil {
			return nil, fmt.Errorf("property not found: %s", k)
		}

		// Cast the value to the appropriate type.
		v = p.Cast(v)

		// Factorize value, if needed.
		if p.DataType == Factor {
			var err error
			v, err = t.factorize(p.ID, v.(string), true)
			if err != nil {
				return nil, err
			}
		}

		rawEvent.data[p.ID] = v
	}

	return rawEvent, nil
}

// toEvent returns an normal event representation of this raw event.
func (t *Table) toEvent(e *rawEvent) (*Event, error) {
	event := &Event{
		Timestamp: unshiftTime(e.timestamp),
		Data:      make(map[string]interface{}),
	}

	// Map data by name instead of property id.
	for k, v := range e.data {
		p := t.propertiesByID[k]

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
				v, err = t.defactorize(p.ID, int(intValue))
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

type tableRawMessage struct {
	Name           string      `json:"name"`
	ShardCount     int         `json:"shardCount"`
	MaxPermanentID int         `json:"maxPermanentID"`
	MaxTransientID int         `json:"maxTransientID"`
	Properties     []*Property `json:"properties"`
}

// Event represents the state for an object at a given point in time.
type Event struct {
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
}

// rawEvent represents an internal event structure.
type rawEvent struct {
	timestamp int64
	data      map[int]interface{}
}

// marshal encodes the raw event as a byte slice.
func (e *rawEvent) marshal() ([]byte, error) {
	var buf bytes.Buffer
	err := binary.Write(&buf, binary.BigEndian, e.timestamp)
	assert(err == nil, "timestamp marshal error: %v", err)

	var handle codec.MsgpackHandle
	handle.RawToString = true
	if err := codec.NewEncoder(&buf, &handle).Encode(e.data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// unmarshal decodes a raw event from a byte slice.
func (e *rawEvent) unmarshal(b []byte) error {
	var buf = bytes.NewBuffer(b)
	err := binary.Read(buf, binary.BigEndian, &e.timestamp)
	assert(err == nil, "timestamp unmarshal error: %v", err)

	e.data = make(map[int]interface{})
	var handle codec.MsgpackHandle
	handle.RawToString = true
	if err := codec.NewDecoder(buf, &handle).Decode(&e.data); err != nil {
		return err
	}
	e.normalize()

	return nil
}

// normalize promotes all values of the raw event to appropriate types.
func (e *rawEvent) normalize() {
	for k, v := range e.data {
		e.data[k] = promote(v)
	}
}

// stat represents a simple counter and timer.
type stat struct {
	count int
	time  time.Time
}

// since returns the elapsed time since the stat began.
func (s *stat) since() time.Duration {
	return time.Since(s.time)
}

// apply increments the count and duration based on the stat.
func (s *stat) apply(count *int, duration *time.Duration) {
	*count += s.count
	*duration += time.Since(s.time)
}

// bench begins a timed stat counter.
func bench() stat {
	return stat{0, time.Now()}
}

// Stat represents statistics for a single table.
type Stat struct {
	Entries      uint64 `json:"entries"` // Number of data items
	Size         uint64 `json:"size"`    // Size of the data memory map
	Depth        uint   `json:"depth"`   // Depth (height) of the B-tree
	Transactions struct {
		Last uint64 `json:"last"` // ID of the last committed transaction
	} `json:"transactions"`
	Readers struct {
		Max     uint `json:"max"`     // maximum number of threads for the environment
		Current uint `json:"current"` // maximum number of threads used in the environment
	} `json:"readers"`
	Pages struct {
		Last     uint64 `json:"last"`     // ID of the last used page
		Size     uint   `json:"size"`     // Size of a database page. This is currently the same for all databases.
		Branch   uint64 `json:"branch"`   // Number of internal (non-leaf) pages
		Leaf     uint64 `json:"leaf"`     // Number of leaf pages
		Overflow uint64 `json:"overflow"` // Number of overflow pages
	} `json:"pages"`
	Event struct {
		Fetch struct {
			Count    int           `json:"count"`
			Duration time.Duration `json:"duration"`
		} `json:"fetch"`
		Insert struct {
			Count    int           `json:"count"`
			Duration time.Duration `json:"duration"`
		} `json:"insert"`
		Delete struct {
			Count    int           `json:"count"`
			Duration time.Duration `json:"duration"`
		} `json:"delete"`
		Factorize struct {
			CacheHit struct {
				Count int `json:"count"`
			} `json:"cacheHit"`
			FetchHit struct {
				Count    int           `json:"count"`
				Duration time.Duration `json:"duration"`
			} `json:"fetchHit"`
			FetchMiss struct {
				Count    int           `json:"count"`
				Duration time.Duration `json:"duration"`
			} `json:"fetchMiss"`
			Create struct {
				Count    int           `json:"count"`
				Duration time.Duration `json:"duration"`
			} `json:"create"`
		} `json:"factorize"`
		Defactorize struct {
			CacheHit struct {
				Count int `json:"count"`
			} `json:"cacheHit"`
			FetchHit struct {
				Count    int           `json:"count"`
				Duration time.Duration `json:"duration"`
			} `json:"fetchHit"`
			FetchMiss struct {
				Count    int           `json:"count"`
				Duration time.Duration `json:"duration"`
			} `json:"fetchMiss"`
		} `json:"defactorize"`
		Marshal struct {
			Count    int           `json:"count"`
			Duration time.Duration `json:"duration"`
		} `json:"marshal"`
		Unmarshal struct {
			Count    int           `json:"count"`
			Duration time.Duration `json:"duration"`
		} `json:"unmarshal"`
	} `json:"event"`
}

// Diff calculates the difference between a stat object and another.
func (s *Stat) Diff(other *Stat) *Stat {
	diff := &Stat{}
	diff.Entries = s.Entries - other.Entries
	diff.Size = s.Size - other.Size
	diff.Depth = s.Depth - other.Depth
	diff.Transactions.Last = s.Transactions.Last - other.Transactions.Last
	diff.Readers.Max = s.Readers.Max - other.Readers.Max
	diff.Readers.Current = s.Readers.Current - other.Readers.Current
	diff.Pages.Last = s.Pages.Last - other.Pages.Last
	diff.Pages.Size = s.Pages.Size - other.Pages.Size
	diff.Pages.Branch = s.Pages.Branch - other.Pages.Branch
	diff.Pages.Leaf = s.Pages.Leaf - other.Pages.Leaf
	diff.Pages.Overflow = s.Pages.Overflow - other.Pages.Overflow
	diff.Event.Fetch.Count = s.Event.Fetch.Count - other.Event.Fetch.Count
	diff.Event.Fetch.Duration = s.Event.Fetch.Duration - other.Event.Fetch.Duration
	diff.Event.Insert.Count = s.Event.Insert.Count - other.Event.Insert.Count
	diff.Event.Insert.Duration = s.Event.Insert.Duration - other.Event.Insert.Duration
	diff.Event.Delete.Count = s.Event.Delete.Count - other.Event.Delete.Count
	diff.Event.Delete.Duration = s.Event.Delete.Duration - other.Event.Delete.Duration
	diff.Event.Factorize.CacheHit.Count = s.Event.Factorize.CacheHit.Count - other.Event.Factorize.CacheHit.Count
	diff.Event.Factorize.FetchHit.Count = s.Event.Factorize.FetchHit.Count - other.Event.Factorize.FetchHit.Count
	diff.Event.Factorize.FetchHit.Duration = s.Event.Factorize.FetchHit.Duration - other.Event.Factorize.FetchHit.Duration
	diff.Event.Factorize.FetchMiss.Count = s.Event.Factorize.FetchMiss.Count - other.Event.Factorize.FetchMiss.Count
	diff.Event.Factorize.FetchMiss.Duration = s.Event.Factorize.FetchMiss.Duration - other.Event.Factorize.FetchMiss.Duration
	diff.Event.Factorize.Create.Count = s.Event.Factorize.Create.Count - other.Event.Factorize.Create.Count
	diff.Event.Factorize.Create.Duration = s.Event.Factorize.Create.Duration - other.Event.Factorize.Create.Duration
	diff.Event.Defactorize.CacheHit.Count = s.Event.Defactorize.CacheHit.Count - other.Event.Defactorize.CacheHit.Count
	diff.Event.Defactorize.FetchHit.Count = s.Event.Defactorize.FetchHit.Count - other.Event.Defactorize.FetchHit.Count
	diff.Event.Defactorize.FetchHit.Duration = s.Event.Defactorize.FetchHit.Duration - other.Event.Defactorize.FetchHit.Duration
	diff.Event.Defactorize.FetchMiss.Count = s.Event.Defactorize.FetchMiss.Count - other.Event.Defactorize.FetchMiss.Count
	diff.Event.Defactorize.FetchMiss.Duration = s.Event.Defactorize.FetchMiss.Duration - other.Event.Defactorize.FetchMiss.Duration
	diff.Event.Marshal.Count = s.Event.Marshal.Count - other.Event.Marshal.Count
	diff.Event.Marshal.Duration = s.Event.Marshal.Duration - other.Event.Marshal.Duration
	diff.Event.Unmarshal.Count = s.Event.Unmarshal.Count - other.Event.Unmarshal.Count
	diff.Event.Unmarshal.Duration = s.Event.Unmarshal.Duration - other.Event.Unmarshal.Duration
	return diff
}

type Cursor struct {
	*mdb.Cursor
}

func (c *Cursor) Close() {
	txn := c.Txn()
	c.Cursor.Close()
	txn.Commit()
}
