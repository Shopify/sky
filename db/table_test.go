package db_test

import (
	"errors"
	"strconv"
	"testing"
	"time"

	. "github.com/skydb/sky/db"
	"github.com/stretchr/testify/assert"
)

// Ensure that a table can create new properties.
func TestTableCreateProperty(t *testing.T) {
	withDB(func(db *DB, path string) {
		table, err := db.CreateTable("foo", 16)
		assert.NoError(t, err)
		table.Update(func(tx *Tx) error {
			// Create permanent properties.
			p, err := tx.CreateProperty("firstName", String, false)
			assert.NoError(t, err)
			assert.Equal(t, p.ID, 1)
			assert.Equal(t, p.Name, "firstName")
			assert.Equal(t, p.DataType, String)
			assert.Equal(t, p.Transient, false)

			p, err = tx.CreateProperty("lastName", Factor, false)
			assert.NoError(t, err)
			assert.Equal(t, p.ID, 2)
			assert.Equal(t, p.Name, "lastName")

			// Create transient properties.
			p, err = tx.CreateProperty("myNum", Integer, true)
			assert.NoError(t, err)
			assert.Equal(t, p.ID, -1)
			assert.Equal(t, p.Name, "myNum")

			p, err = tx.CreateProperty("myFloat", Float, true)
			assert.NoError(t, err)
			assert.Equal(t, p.ID, -2)
			assert.Equal(t, p.Name, "myFloat")

			// Create another permanent property.
			p, err = tx.CreateProperty("myBool", Float, false)
			assert.NoError(t, err)
			assert.Equal(t, p.ID, 3)
			assert.Equal(t, p.Name, "myBool")

			return nil
		})
	})
}

// Ensure that creating a property on an unopened table returns an error.
func TestTableCreatePropertyNotOpen(t *testing.T) {
	withDB(func(db *DB, path string) {
		table, _ := db.CreateTable("foo", 16)
		db.Close()
		table.Update(func(tx *Tx) error {
			p, err := tx.CreateProperty("prop", Integer, false)
			assert.Equal(t, errors.New("table not open: foo"), err)
			assert.Nil(t, p)
			return nil
		})
	})
}

// Ensure that creating a property with an existing name returns an error.
func TestTableCreatePropertyDuplicateName(t *testing.T) {
	withDB(func(db *DB, path string) {
		table, _ := db.CreateTable("foo", 16)
		table.Update(func(tx *Tx) error {
			tx.CreateProperty("prop", Integer, false)
			p, err := tx.CreateProperty("prop", Float, false)
			assert.Equal(t, err, errors.New("property already exists: prop"))
			assert.Nil(t, p)
			return nil
		})
	})
}

// Ensure that creating a property that fails validation will return the validation error.
func TestTableCreatePropertyInvalid(t *testing.T) {
	withDB(func(db *DB, path string) {
		table, _ := db.CreateTable("foo", 16)
		table.Update(func(tx *Tx) error {
			p, err := tx.CreateProperty("my•prop", Integer, false)
			assert.Equal(t, err, errors.New("invalid property name: my•prop"))
			assert.Nil(t, p)
			return nil
		})
	})
}

// Ensure that a property can be renamed.
func TestTableRenameProperty(t *testing.T) {
	withDB(func(db *DB, path string) {
		table, _ := db.CreateTable("foo", 16)
		table.Update(func(tx *Tx) error {
			tx.CreateProperty("prop", Integer, false)
			p, err := tx.RenameProperty("prop", "prop2")
			assert.NoError(t, err)
			assert.Equal(t, p.ID, 1)
			assert.Equal(t, p.Name, "prop2")
			return nil
		})
	})
}

// Ensure that renaming a property on a closed table returns an error.
func TestTableRenamePropertyNotOpen(t *testing.T) {
	withDB(func(db *DB, path string) {
		table, _ := db.CreateTable("foo", 16)
		db.Close()
		table.Update(func(tx *Tx) error {
			p, err := tx.RenameProperty("prop", "prop2")
			assert.Equal(t, err, errors.New("table not open: foo"))
			assert.Nil(t, p)
			return nil
		})
	})
}

// Ensure that a renaming a non-existent property returns an error.
func TestTableRenamePropertyNotFound(t *testing.T) {
	withDB(func(db *DB, path string) {
		table, _ := db.CreateTable("foo", 16)
		table.Update(func(tx *Tx) error {
			p, err := tx.RenameProperty("prop", "prop2")
			assert.Equal(t, err, errors.New("property not found: prop"))
			assert.Nil(t, p)
			return nil
		})
	})
}

// Ensure that a renaming a property to a name that already exists returns an error.
func TestTableRenamePropertyAlreadyExists(t *testing.T) {
	withDB(func(db *DB, path string) {
		table, _ := db.CreateTable("foo", 16)
		table.Update(func(tx *Tx) error {
			tx.CreateProperty("prop", Integer, false)
			tx.CreateProperty("prop2", Integer, false)
			p, err := tx.RenameProperty("prop", "prop2")
			assert.Equal(t, err, errors.New("property already exists: prop2"))
			assert.Nil(t, p)
			return nil
		})
	})
}

// Ensure that a table can delete properties.
func TestTableDeleteProperty(t *testing.T) {
	withDB(func(db *DB, path string) {
		table, _ := db.CreateTable("foo", 16)
		table.Update(func(tx *Tx) error {
			tx.CreateProperty("prop1", String, false)
			tx.CreateProperty("prop2", Factor, false)

			// Delete a property.
			err := tx.DeleteProperty("prop2")
			assert.NoError(t, err)

			// Retrieve properties.
			p, err := tx.Property("prop1")
			assert.NotNil(t, p)
			assert.NoError(t, err)
			p, err = tx.Property("prop2")
			assert.Nil(t, p)
			assert.NoError(t, err)
			return nil
		})

		// Close and reopen DB.
		db.Close()
		db = &DB{}
		db.Open(path)

		table, _ = db.OpenTable("foo")
		table.View(func(tx *Tx) error {
			// Check properties again.
			p, _ := tx.Property("prop1")
			assert.NotNil(t, p)
			p, _ = tx.Property("prop2")
			assert.Nil(t, p)
			return nil
		})
	})
}

// Ensure that deleting a property on a closed table returns an error.
func TestTableDeletePropertyNotOpen(t *testing.T) {
	withDB(func(db *DB, path string) {
		table, _ := db.CreateTable("foo", 16)
		db.Close()
		table.Update(func(tx *Tx) error {
			err := tx.DeleteProperty("prop2")
			assert.Equal(t, err, errors.New("table not open: foo"))
			return nil
		})
	})
}

// Ensure that deleting a non-existent property returns an error.
func TestTableDeletePropertyNotFound(t *testing.T) {
	withDB(func(db *DB, path string) {
		table, _ := db.CreateTable("foo", 16)
		table.Update(func(tx *Tx) error {
			err := tx.DeleteProperty("prop2")
			assert.Equal(t, err, errors.New("property not found: prop2"))
			return nil
		})
	})
}

// Ensure that the table can return a map of properties by name.
func TestTableProperties(t *testing.T) {
	withDB(func(db *DB, path string) {
		table, _ := db.CreateTable("foo", 16)
		table.Update(func(tx *Tx) error {
			tx.CreateProperty("prop1", String, true)
			tx.CreateProperty("prop2", Factor, false)
			p, err := tx.Properties()
			assert.NoError(t, err)
			assert.Equal(t, p["prop1"].ID, -1)
			assert.Equal(t, p["prop2"].ID, 1)
			return nil
		})
	})
}

// Ensure that retrieving the properties of a table when it's closed returns an error.
func TestTablePropertiesNotOpen(t *testing.T) {
	withDB(func(db *DB, path string) {
		table, _ := db.CreateTable("foo", 16)
		db.Close()
		table.Update(func(tx *Tx) error {
			p, err := tx.Properties()
			assert.Equal(t, err, errors.New("table not open: foo"))
			assert.Nil(t, p)
			return nil
		})
	})
}

// Ensure that the table can return a map of properties by id.
func TestTablePropertiesByID(t *testing.T) {
	withDB(func(db *DB, path string) {
		table, _ := db.CreateTable("foo", 16)
		table.Update(func(tx *Tx) error {
			tx.CreateProperty("prop1", String, true)
			tx.CreateProperty("prop2", Factor, false)
			p, err := tx.PropertiesByID()
			assert.NoError(t, err)
			assert.Equal(t, p[-1].Name, "prop1")
			assert.Equal(t, p[1].Name, "prop2")
			return nil
		})
	})
}

// Ensure that retrieving the properties of a table by id when it's closed returns an error.
func TestTablePropertiesByIDNotOpen(t *testing.T) {
	withDB(func(db *DB, path string) {
		table, _ := db.CreateTable("foo", 16)
		db.Close()
		table.Update(func(tx *Tx) error {
			p, err := tx.PropertiesByID()
			assert.Equal(t, err, errors.New("table not open: foo"))
			assert.Nil(t, p)
			return nil
		})
	})
}

// Ensure that retrieving a property from a closed table returns an error.
func TestTablePropertyNotOpen(t *testing.T) {
	withDB(func(db *DB, path string) {
		table, _ := db.CreateTable("foo", 16)
		db.Close()
		table.Update(func(tx *Tx) error {
			p, err := tx.Property("foo")
			assert.Equal(t, err, errors.New("table not open: foo"))
			assert.Nil(t, p)
			return nil
		})
	})
}

// Ensure that the table can retrieve a property by id.
func TestTablePropertyByID(t *testing.T) {
	withDB(func(db *DB, path string) {
		table, _ := db.CreateTable("foo", 16)
		table.Update(func(tx *Tx) error {
			tx.CreateProperty("prop1", String, true)
			tx.CreateProperty("prop2", Factor, false)

			p, err := tx.PropertyByID(-1)
			assert.NoError(t, err)
			assert.Equal(t, p.Name, "prop1")

			p, err = tx.PropertyByID(1)
			assert.NoError(t, err)
			assert.Equal(t, p.Name, "prop2")

			p, err = tx.PropertyByID(2)
			assert.Nil(t, p)
			assert.Nil(t, err)
			return nil
		})
	})
}

// Ensure that retrieving a property by id from a closed table returns an error.
func TestTablePropertyByIDNotOpen(t *testing.T) {
	withDB(func(db *DB, path string) {
		table, _ := db.CreateTable("foo", 16)
		db.Close()
		table.Update(func(tx *Tx) error {
			p, err := tx.PropertyByID(-1)
			assert.Equal(t, err, errors.New("table not open: foo"))
			assert.Nil(t, p)
			return nil
		})
	})
}

// Ensure that a table can create properties and persist them after a reopen.
func TestTableReopen(t *testing.T) {
	withDB(func(db *DB, path string) {
		table, _ := db.CreateTable("foo", 16)
		table.Update(func(tx *Tx) error {
			tx.CreateProperty("prop1", Integer, false)
			tx.CreateProperty("prop2", String, true)
			tx.CreateProperty("prop3", Float, false)
			tx.CreateProperty("prop4", Factor, true)
			return nil
		})

		db.Close()
		assert.NoError(t, db.Open(path))

		table, _ = db.OpenTable("foo")
		table.Update(func(tx *Tx) error {
			p, err := tx.Property("prop1")
			assert.NoError(t, err)
			assert.Equal(t, p.ID, 1)

			p, _ = tx.Property("prop2")
			assert.Equal(t, p.ID, -1)
			p, _ = tx.Property("prop3")
			assert.Equal(t, p.ID, 2)
			p, _ = tx.Property("prop4")
			assert.Equal(t, p.ID, -2)
			return nil
		})
	})
}

// Ensure that retrieving an event while the database is closed returns an error.
func TestTableGetEventNotOpen(t *testing.T) {
	withDB(func(db *DB, path string) {
		table, _ := db.CreateTable("foo", 16)
		db.Close()
		table.Update(func(tx *Tx) error {
			e, err := tx.Event("user1", mustParseTime("2000-01-01T00:00:01Z"))
			assert.Equal(t, err, errors.New("table not open: foo"))
			assert.Nil(t, e)
			return nil
		})
	})
}

// Ensure that retrieving multiple events while the database is closed returns an error.
func TestTableGetEventsNotOpen(t *testing.T) {
	withDB(func(db *DB, path string) {
		table, _ := db.CreateTable("foo", 16)
		db.Close()
		table.Update(func(tx *Tx) error {
			events, err := tx.Events("user1")
			assert.Equal(t, err, errors.New("table not open: foo"))
			assert.Nil(t, events)
			return nil
		})
	})
}

// Ensure that a table can insert an event.
func TestTableInsertEvent(t *testing.T) {
	withDB(func(db *DB, path string) {
		table, _ := db.CreateTable("foo", 16)
		table.Update(func(tx *Tx) error {
			tx.CreateProperty("prop1", Integer, false)
			tx.CreateProperty("prop2", String, true)
			err := tx.InsertEvent("user1", newEvent("2000-01-01T00:00:01Z", "prop1", 20, "prop2", "bob"))
			assert.NoError(t, err)
			err = tx.InsertEvent("user2", newEvent("2000-01-01T00:00:01Z", "prop1", 100))
			assert.NoError(t, err)
			err = tx.InsertEvent("user1", newEvent("2000-01-01T00:00:00Z", "prop2", "susy"))
			assert.NoError(t, err)

			// Find first user's first event.
			e, err := tx.Event("user1", mustParseTime("2000-01-01T00:00:01Z"))
			if assert.NoError(t, err) && assert.NotNil(t, e) {
				assert.Equal(t, e.Timestamp, mustParseTime("2000-01-01T00:00:01Z"))
				assert.Equal(t, e.Data["prop1"], int64(20))
				assert.Equal(t, e.Data["prop2"], "bob")
			}

			// Find first user's second event.
			e, err = tx.Event("user1", mustParseTime("2000-01-01T00:00:00Z"))
			if assert.NoError(t, err) && assert.NotNil(t, e) {
				assert.Equal(t, e.Timestamp, mustParseTime("2000-01-01T00:00:00Z"))
				assert.Nil(t, e.Data["prop1"])
				assert.Equal(t, e.Data["prop2"], "susy")
			}

			// Find second user's only event.
			e, err = tx.Event("user2", mustParseTime("2000-01-01T00:00:01Z"))
			if assert.NoError(t, err) && assert.NotNil(t, e) {
				assert.Equal(t, e.Timestamp, mustParseTime("2000-01-01T00:00:01Z"))
				assert.Equal(t, e.Data["prop1"], int64(100))
				assert.Nil(t, e.Data["prop2"])
			}

			// Nonexistent user shouldn't return any event.
			e, err = tx.Event("no-such-user", mustParseTime("2000-01-01T00:00:00Z"))
			assert.NoError(t, err)
			assert.Nil(t, e)

			// Nonexistent event shouldn't return any event.
			e, err = tx.Event("user1", mustParseTime("1999-01-01T00:00:00Z"))
			assert.NoError(t, err)
			assert.Nil(t, e)
			return nil
		})
	})
}

// Ensure that inserting an event into a closed table returns an error.
func TestTableInsertEventNotOpen(t *testing.T) {
	withDB(func(db *DB, path string) {
		table, _ := db.CreateTable("foo", 16)
		db.Close()
		table.Update(func(tx *Tx) error {
			err := tx.InsertEvent("user1", newEvent("2000-01-01T00:00:01Z", "prop1", 20, "prop2", "bob"))
			assert.Equal(t, err, errors.New("table not open: foo"))
			return nil
		})
	})
}

// Ensure that a table can insert multiple events.
func TestTableInsertEvents(t *testing.T) {
	withDB(func(db *DB, path string) {
		table, _ := db.CreateTable("foo", 16)
		table.Update(func(tx *Tx) error {
			tx.CreateProperty("prop1", Integer, false)
			tx.CreateProperty("prop2", String, true)
			err := tx.InsertEvents("user1", []*Event{
				newEvent("2000-01-01T00:00:01Z", "prop1", 20, "prop2", "bob"),
				newEvent("2000-01-01T00:00:00Z", "prop2", "susy"),
			})
			assert.NoError(t, err)
			err = tx.InsertEvents("user2", []*Event{
				newEvent("2000-01-01T00:00:01Z", "prop1", 100),
			})
			assert.NoError(t, err)
			err = tx.InsertEvents("user3", []*Event{})

			// Find first user's events.
			events, err := tx.Events("user1")
			if assert.NoError(t, err) && assert.Equal(t, len(events), 2) {
				assert.Equal(t, events[0].Timestamp, mustParseTime("2000-01-01T00:00:00Z"))
				assert.Nil(t, events[0].Data["prop1"])
				assert.Equal(t, events[0].Data["prop2"], "susy")

				assert.Equal(t, events[1].Timestamp, mustParseTime("2000-01-01T00:00:01Z"))
				assert.Equal(t, events[1].Data["prop1"], int64(20))
				assert.Equal(t, events[1].Data["prop2"], "bob")
			}

			// Find second user's events.
			events, err = tx.Events("user2")
			if assert.NoError(t, err) && assert.Equal(t, len(events), 1) {
				assert.Equal(t, events[0].Timestamp, mustParseTime("2000-01-01T00:00:01Z"))
				assert.Equal(t, events[0].Data["prop1"], int64(100))
				assert.Nil(t, events[0].Data["prop2"])
			}

			// Third user should have no events.
			events, err = tx.Events("user3")
			assert.NoError(t, err)
			assert.Equal(t, len(events), 0)

			// Non-existent user should have no events.
			events, err = tx.Events("no-such-user")
			assert.NoError(t, err)
			assert.Equal(t, len(events), 0)
			return nil
		})
	})
}

// Ensure that a table can insert multiple events for multiple objects.
func TestTableInsertObjects(t *testing.T) {
	t.Skip("TODO")
}

// Ensure that deleting events from a closed table returns an error.
func TestTableDeleteEventNotOpen(t *testing.T) {
	withDB(func(db *DB, path string) {
		table, _ := db.CreateTable("foo", 16)
		db.Close()
		table.Update(func(tx *Tx) error {
			err := tx.DeleteEvent("user1", mustParseTime("2000-01-01T00:00:01Z"))
			assert.Equal(t, err, errors.New("table not open: foo"))
			return nil
		})
	})
}

// Ensure that a table can delete a single event.
func TestTableDeleteEvent(t *testing.T) {
	withDB(func(db *DB, path string) {
		table, _ := db.CreateTable("foo", 16)
		table.Update(func(tx *Tx) error {
			tx.CreateProperty("prop1", Integer, false)
			tx.InsertEvents("user1", []*Event{
				newEvent("2000-01-01T00:00:00Z", "prop1", 20),
				newEvent("2000-01-01T00:00:01Z", "prop1", 30),
				newEvent("2000-01-01T00:00:02Z", "prop1", 30),
			})
			tx.InsertEvents("user2", []*Event{
				newEvent("2000-01-01T00:00:00Z", "prop1", 100),
			})

			// Delete an event from the first user.
			assert.NoError(t, tx.DeleteEvent("user1", mustParseTime("2000-01-01T00:00:00Z")))

			// Verify event is gone.
			e, _ := tx.Event("user1", mustParseTime("2000-01-01T00:00:00Z"))
			assert.Nil(t, e)
			e, _ = tx.Event("user1", mustParseTime("2000-01-01T00:00:01Z"))
			assert.NotNil(t, e)
			e, _ = tx.Event("user1", mustParseTime("2000-01-01T00:00:02Z"))
			assert.NotNil(t, e)
			e, _ = tx.Event("user2", mustParseTime("2000-01-01T00:00:00Z"))
			assert.NotNil(t, e)

			// Delete another event and verify.
			tx.DeleteEvent("user1", mustParseTime("2000-01-01T00:00:02Z"))
			e, _ = tx.Event("user1", mustParseTime("2000-01-01T00:00:02Z"))
			assert.Nil(t, e)

			// Delete another event and verify.
			tx.DeleteEvent("user1", mustParseTime("2000-01-01T00:00:01Z"))
			e, _ = tx.Event("user1", mustParseTime("2000-01-01T00:00:01Z"))
			assert.Nil(t, e)

			// Delete non-existent exent.
			err := tx.DeleteEvent("user1", mustParseTime("1999-01-01T00:00:00Z"))
			assert.NoError(t, err)
			return nil
		})
	})
}

// Ensure that deleting all events from a closed table returns an error.
func TestTableDeleteEventsNotOpen(t *testing.T) {
	withDB(func(db *DB, path string) {
		table, _ := db.CreateTable("foo", 16)
		db.Close()
		table.Update(func(tx *Tx) error {
			err := tx.DeleteEvents("user1")
			assert.Equal(t, err, errors.New("table not open: foo"))
			return nil
		})
	})
}

// Ensure that inserting events into a closed table returns an error.
func TestTableInsertEventsNotOpen(t *testing.T) {
	withDB(func(db *DB, path string) {
		table, _ := db.CreateTable("foo", 16)
		db.Close()
		table.Update(func(tx *Tx) error {
			err := tx.InsertEvents("user1", []*Event{newEvent("2000-01-01T00:00:01Z", "prop1", 20, "prop2", "bob")})
			assert.Equal(t, err, errors.New("table not open: foo"))
			return nil
		})
	})
}

// Ensure that a table can delete all events.
func TestTableDeleteEvents(t *testing.T) {
	withDB(func(db *DB, path string) {
		table, _ := db.CreateTable("foo", 16)
		table.Update(func(tx *Tx) error {
			tx.CreateProperty("prop1", Integer, false)
			tx.InsertEvents("user1", []*Event{
				newEvent("2000-01-01T00:00:00Z", "prop1", 20),
				newEvent("2000-01-01T00:00:01Z", "prop1", 30),
				newEvent("2000-01-01T00:00:02Z", "prop1", 30),
			})
			tx.InsertEvents("user2", []*Event{
				newEvent("2000-01-01T00:00:00Z", "prop1", 100),
			})

			// Delete all events for the first user.
			tx.DeleteEvents("user1")

			// Verify events are gone.
			events, err := tx.Events("user1")
			assert.NoError(t, err)
			assert.Equal(t, len(events), 0)

			events, err = tx.Events("user2")
			assert.NoError(t, err)
			assert.Equal(t, len(events), 1)
			return nil
		})
	})
}

// Ensure that a table can factorize and defactorize events correctly.
func TestTableFactorize(t *testing.T) {
	withDB(func(db *DB, path string) {
		table, _ := db.CreateTable("foo", 16)
		table.Update(func(tx *Tx) error {
			tx.CreateProperty("prop1", Factor, false)
			tx.CreateProperty("prop2", Factor, false)
			tx.CreateProperty("prop3", Factor, false)
			tx.InsertEvents("user1", []*Event{
				newEvent("2000-01-01T00:00:00Z", "prop1", "foo", "prop2", "bar", "prop3", ""),
				newEvent("2000-01-01T00:00:01Z", "prop1", "foo"),
			})

			// Verify the events.
			e, err := tx.Event("user1", mustParseTime("2000-01-01T00:00:00Z"))
			assert.NoError(t, err)
			assert.Equal(t, e.Data["prop1"], "foo")
			assert.Equal(t, e.Data["prop2"], "bar")
			assert.Equal(t, e.Data["prop3"], "")

			e, err = tx.Event("user1", mustParseTime("2000-01-01T00:00:01Z"))
			assert.NoError(t, err)
			assert.Equal(t, e.Data["prop1"], "foo")
			return nil
		})
	})
}

// Ensure that a table can factorize a large number of values beyond the cache.
func TestTableFactorizeBeyondCache(t *testing.T) {
	withDB(func(db *DB, path string) {
		table, _ := db.CreateTable("foo", 16)
		table.Update(func(tx *Tx) error {
			tx.CreateProperty("prop1", Factor, false)
			tx.CreateProperty("prop2", Factor, false)
			tx.CreateProperty("prop3", Factor, false)

			// Insert a bunch of events.
			startTime := mustParseTime("2000-01-01T00:00:00Z")
			for i := 0; i < FactorCacheSize*3; i++ {
				e := &Event{
					Timestamp: startTime.Add(time.Duration(i) * time.Second),
					Data:      map[string]interface{}{"prop1": strconv.Itoa(i), "prop2": strconv.Itoa(i % (FactorCacheSize * 1.5)), "prop3": "foo"},
				}
				tx.InsertEvent("user1", e)
			}

			// Verify factor values.
			for i := 0; i < FactorCacheSize*3; i++ {
				e, err := tx.Event("user1", startTime.Add(time.Duration(i)*time.Second))
				if assert.NoError(t, err) {
					assert.Equal(t, strconv.Itoa(i), e.Data["prop1"])
					assert.Equal(t, strconv.Itoa(i%(FactorCacheSize*1.5)), e.Data["prop2"])
					assert.Equal(t, "foo", e.Data["prop3"])
				}
			}
			return nil
		})
	})
}

func TestTableStats(t *testing.T) {
	withDB(func(db *DB, path string) {
		table, err := db.CreateTable("foo", 16)
		assert.NoError(t, err)
		table.Update(func(tx *Tx) error {
			_, err := tx.CreateProperty("prop1", Integer, false)
			assert.NoError(t, err)

			tx.InsertEvents("user1", []*Event{
				newEvent("2000-01-01T00:00:00Z", "prop1", 20),
			})

			return nil
		})

		stats, err := table.Stats()
		assert.NoError(t, err)

		assert.Equal(t, stats.KeyCount, 3)
		assert.Equal(t, stats.LeafPages, 18)
		assert.Equal(t, stats.Depth, 2)
		assert.Equal(t, stats.LeafAllocated, 73728)
		assert.Equal(t, stats.LeafInUse, 286)
	})
}

func newEvent(timestamp string, pairs ...interface{}) *Event {
	e := &Event{Timestamp: mustParseTime(timestamp), Data: make(map[string]interface{})}
	for i := 0; i < len(pairs); i += 2 {
		e.Data[pairs[i].(string)] = pairs[i+1]
	}
	return e
}

func mustParseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		panic(err)
	}
	return t.UTC()
}
