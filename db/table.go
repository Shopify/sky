package db

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/boltdb/bolt"
	"github.com/skydb/sky/hash"
	"github.com/ugorji/go/codec"
)

// FactorCacheSize is the number of factors that are stored in the LRU cache.
// This cache size is per-property.
const FactorCacheSize = 1000

var (
	// ErrObjectIDRequired is returned inserting, deleting, or retrieving
	// event data without specifying an object identifier.
	ErrObjectIDRequired = errors.New("object id required")
)

// NewTable returns a reference to a new table.
func NewTable(name, path string) *Table {
	return &Table{
		name: name,
		path: path,
	}
}

// Statistics about the table
type TableStats struct {
	// Page count statistics
	BranchPages    int `json:"branchPages"`
	BranchOverflow int `json:"branchOverflow"`
	LeafPages      int `json:"leafPages"`
	LeafOverflow   int `json:"leafOverflow"`

	// Tree statistics
	KeyCount int `json:"keyCount"`
	Depth    int `json:"depth"`

	// Page size utilization
	BranchAllocated int `json:"branchAlloc"`
	BranchInUse     int `json:"branchInuse"`
	LeafAllocated   int `json:"leafAlloc"`
	LeafInUse       int `json:"leafInuse"`
}

// Table represents a collection of objects.
type Table struct {
	sync.Mutex

	db             *bolt.DB
	name           string
	path           string
	caches         map[int]*cache
	properties     map[string]*Property
	propertiesByID map[int]*Property
	stat           Stat

	shardCount     int
	maxPermanentID int
	maxTransientID int
}

// Gather stats from bolt and return the stats as a map so the internals of bolt.BucketStats
// are not exposed to the caller
func (t *Table) Stats() (*TableStats, error) {
	stats := new(TableStats)

	err := t.db.View(func(tx *bolt.Tx) error {
		var s bolt.BucketStats
		tx.ForEach(func(name []byte, b *bolt.Bucket) error {
			s.Add(b.Stats())
			return nil
		})

		stats.BranchPages = s.BranchPageN
		stats.BranchOverflow = s.BranchOverflowN
		stats.LeafPages = s.LeafPageN
		stats.LeafOverflow = s.LeafOverflowN
		stats.KeyCount = s.KeyN
		stats.Depth = s.Depth
		stats.BranchAllocated = s.BranchAlloc
		stats.BranchInUse = s.BranchInuse
		stats.LeafAllocated = s.LeafAlloc
		stats.LeafInUse = s.LeafInuse

		return nil
	})

	if err != nil {
		return nil, err
	}

	return stats, nil
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

func (t *Table) Create() error {
	t.Lock()
	defer t.Unlock()

	// Set initial shard count.
	if t.shardCount == 0 {
		t.shardCount = runtime.NumCPU()
	}

	// Open the table.
	if err := t.open(); err != nil {
		return err
	}

	// Save initial table state.
	err := t.Update(func(tx *Tx) error {
		return tx.PutMeta()
	})
	if err != nil {
		return err
	}

	return nil
}

// Open opens and initializes the table.
func (t *Table) Open() error {
	t.Lock()
	defer t.Unlock()
	return t.open()
}

func (t *Table) open() error {
	if t.db != nil {
		return nil
	}

	// Create Bolt database.
	db, err := bolt.Open(t.path, 0600)
	if err != nil {
		return fmt.Errorf("table open: %s", err)
	}
	t.db = db

	// Initialize schema.
	err = t.Update(func(tx *Tx) error {
		// Create meta bucket.
		b, err := tx.CreateBucketIfNotExists([]byte("meta"))
		if err != nil {
			return fmt.Errorf("meta: %s", err)
		}

		// Read meta data into table.
		value := b.Get([]byte("meta"))
		if len(value) > 0 {
			if err := t.unmarshal(value); err != nil {
				return err
			}
		}

		// Create shard buckets.
		for i := 0; i < t.shardCount; i++ {
			if _, err := tx.CreateBucketIfNotExists(shardDBName(i)); err != nil {
				return fmt.Errorf("shard: %s", err)
			}
		}

		// Create factor buckets.
		for _, p := range t.properties {
			if p.DataType != Factor {
				continue
			}
			if _, err := tx.CreateBucketIfNotExists(factorDBName(p.ID)); err != nil {
				return fmt.Errorf("factor: %s", err)
			}
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
	t.close()
	if err := os.RemoveAll(t.path); err != nil {
		return fmt.Errorf("remove all error: %s", err)
	}

	return nil
}

// opened returned whether the table is currently open.
func (t *Table) opened() bool {
	return t.db != nil
}

func (t *Table) Close() {
	t.Lock()
	defer t.Unlock()
	t.close()
}

func (t *Table) close() {
	if t.db != nil {
		t.db.Close()
	}
}

// View executes a function in the context of a read-only transaction.
func (t *Table) View(fn func(*Tx) error) error {
	return t.db.View(func(tx *bolt.Tx) error {
		return fn(&Tx{tx, t})
	})
}

// Update executes a function in the context of a writable transaction.
func (t *Table) Update(fn func(*Tx) error) error {
	return t.db.Update(func(tx *bolt.Tx) error {
		return fn(&Tx{tx, t})
	})
}

// MaxTransientID returns the largest transient property identifier.
func (t *Table) MaxTransientID() int {
	return t.maxTransientID
}

// MaxPermanentID returns the largest transient property identifier.
func (t *Table) MaxPermanentID() int {
	return t.maxPermanentID
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

// shardIndex returns the appropriate shard for a given object id.
func (t *Table) shardIndex(id string) int {
	return int(hash.Local(id)) % t.shardCount
}

// shardDBName returns the name of the shard table.
func shardDBName(index int) []byte {
	return []byte(fmt.Sprintf("shards/%d", index))
}

// factorDBName returns the name of the factor table for a property.
func factorDBName(propertyID int) []byte {
	return []byte(fmt.Sprintf("factors/%d", propertyID))
}

// factorKey returns the value-to-index key.
func factorKey(value string) []byte {
	return []byte(fmt.Sprintf(">%s", value))
}

// reverseFactorKey returns the index-to-value key.
func reverseFactorKey(index int) []byte {
	return []byte(fmt.Sprintf("<%d", index))
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
	Data      map[string]interface{} `json:"data"`
	Timestamp time.Time              `json:"timestamp"`
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
