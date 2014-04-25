package server

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/skydb/sky/core"
	"io"
	"net/http"
	"strconv"
	"time"
)

func (s *Server) addEventHandlers() {
	s.ApiHandleFunc("/tables/{name}/objects/{objectId}/events", func(w http.ResponseWriter, req *http.Request, params map[string]interface{}) (interface{}, error) {
		return s.getEventsHandler(w, req, params)
	}).Methods("GET")
	s.ApiHandleFunc("/tables/{name}/objects/{objectId}/events", func(w http.ResponseWriter, req *http.Request, params map[string]interface{}) (interface{}, error) {
		return s.deleteEventsHandler(w, req, params)
	}).Methods("DELETE")

	s.ApiHandleFunc("/tables/{name}/objects/{objectId}/events/{timestamp}", func(w http.ResponseWriter, req *http.Request, params map[string]interface{}) (interface{}, error) {
		return s.getEventHandler(w, req, params)
	}).Methods("GET")
	s.ApiHandleFunc("/tables/{name}/objects/{objectId}/events/{timestamp}", func(w http.ResponseWriter, req *http.Request, params map[string]interface{}) (interface{}, error) {
		return s.replaceEventHandler(w, req, params)
	}).Methods("PUT")
	s.ApiHandleFunc("/tables/{name}/objects/{objectId}/events/{timestamp}", func(w http.ResponseWriter, req *http.Request, params map[string]interface{}) (interface{}, error) {
		return s.updateEventHandler(w, req, params)
	}).Methods("PATCH")
	s.ApiHandleFunc("/tables/{name}/objects/{objectId}/events/{timestamp}", func(w http.ResponseWriter, req *http.Request, params map[string]interface{}) (interface{}, error) {
		return s.deleteEventHandler(w, req, params)
	}).Methods("DELETE")

	// Streaming import.
	s.router.HandleFunc("/tables/{name}/events", s.streamUpdateEventsHandler).Methods("PATCH")

	// Table agnostic streaming import.
	s.router.HandleFunc("/events", s.streamUpdateEventsHandler).Methods("PATCH")
}

// GET /tables/:name/objects/:objectId/events
func (s *Server) getEventsHandler(w http.ResponseWriter, req *http.Request, params map[string]interface{}) (interface{}, error) {
	vars := mux.Vars(req)
	t, err := s.OpenTable(vars["name"])
	if err != nil {
		return nil, err
	}

	// Retrieve raw events.
	events, err := s.db.GetEvents(t.Name, vars["objectId"])
	if err != nil {
		return nil, err
	}

	// Denormalize events.
	output := make([]map[string]interface{}, 0)
	for _, event := range events {
		f, err := s.db.Factorizer(t.Name)
		if err != nil {
			return nil, err
		}
		if err := f.DefactorizeEvent(event, t.PropertyFile()); err != nil {
			return nil, err
		}
		e, err := t.SerializeEvent(event)
		if err != nil {
			return nil, err
		}
		output = append(output, e)
	}

	return output, nil
}

// DELETE /tables/:name/objects/:objectId/events
func (s *Server) deleteEventsHandler(w http.ResponseWriter, req *http.Request, params map[string]interface{}) (ret interface{}, err error) {
	vars := mux.Vars(req)
	t, err := s.OpenTable(vars["name"])
	if err != nil {
		return nil, err
	}
	return nil, s.db.DeleteObject(t.Name, vars["objectId"])
}

// GET /tables/:name/objects/:objectId/events/:timestamp
func (s *Server) getEventHandler(w http.ResponseWriter, req *http.Request, params map[string]interface{}) (ret interface{}, err error) {
	vars := mux.Vars(req)
	t, err := s.OpenTable(vars["name"])
	if err != nil {
		return nil, err
	}

	// Parse timestamp.
	timestamp, err := time.Parse(time.RFC3339, vars["timestamp"])
	if err != nil {
		return nil, err
	}

	// Find event.
	event, err := s.db.GetEvent(t.Name, vars["objectId"], timestamp)
	if err != nil {
		return nil, err
	}
	// Return an empty event if there isn't one.
	if event == nil {
		event = core.NewEvent(vars["timestamp"], map[int64]interface{}{})
	}

	// Convert an event to a serializable object.
	f, err := s.db.Factorizer(t.Name)
	if err != nil {
		return nil, err
	}
	if err := f.DefactorizeEvent(event, t.PropertyFile()); err != nil {
		return nil, err
	}
	e, err := t.SerializeEvent(event)
	if err != nil {
		return nil, err
	}
	return e, nil
}

// PUT /tables/:name/objects/:objectId/events/:timestamp
func (s *Server) replaceEventHandler(w http.ResponseWriter, req *http.Request, params map[string]interface{}) (ret interface{}, err error) {
	vars := mux.Vars(req)
	t, err := s.OpenTable(vars["name"])
	if err != nil {
		return nil, err
	}

	params["timestamp"] = vars["timestamp"]
	event, err := t.DeserializeEvent(params)
	if err != nil {
		return nil, err
	}

	f, err := s.db.Factorizer(t.Name)
	if err != nil {
		return nil, err
	}
	if err := f.FactorizeEvent(event, t.PropertyFile(), true); err != nil {
		return nil, err
	}

	return nil, s.db.InsertEvent(t.Name, vars["objectId"], event)
}

// PATCH /tables/:name/objects/:objectId/events/:timestamp
func (s *Server) updateEventHandler(w http.ResponseWriter, req *http.Request, params map[string]interface{}) (ret interface{}, err error) {
	vars := mux.Vars(req)
	t, err := s.OpenTable(vars["name"])
	if err != nil {
		return nil, err
	}

	params["timestamp"] = vars["timestamp"]
	event, err := t.DeserializeEvent(params)
	if err != nil {
		return nil, err
	}
	f, err := s.db.Factorizer(t.Name)
	if err != nil {
		return nil, err
	}
	if err := f.FactorizeEvent(event, t.PropertyFile(), true); err != nil {
		return nil, err
	}
	return nil, s.db.InsertEvent(t.Name, vars["objectId"], event)
}

// Used by streaming handler.
type objectEvents map[string][]*core.Event

func (s *Server) flushTableEvents(table *core.Table, objects objectEvents) (int, error) {
	count, err := s.db.InsertObjects(table.Name, objects)
	if err != nil {
		return count, fmt.Errorf("Cannot put event: %v", err)
	}
	return count, nil
}

// PATCH /tables/:name/events and /events
func (s *Server) streamUpdateEventsHandler(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	t0 := time.Now()

	// Check for flush threshold/buffer passed as URL params.
	flushThreshold := s.StreamFlushThreshold
	if rawFlushThreshold := req.FormValue("flush-threshold"); rawFlushThreshold != "" {
		threshold, err := strconv.Atoi(rawFlushThreshold)
		if err == nil {
			flushThreshold = uint(threshold)
		} else {
			s.logger.Printf("ERR: invalid flush-threshold parameter: %v", err)
			fmt.Fprintf(w, `{"message":"invalid flush-threshold parameter: %v"}`, err)
			return
		}
	}

	var table *core.Table
	tableName := vars["name"]
	if tableName != "" {
		var err error
		table, err = s.OpenTable(tableName)
		if err != nil {
			s.logger.Printf("ERR %v", err)
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, `{"message":"%v"}`, err)
			return
		}
	}

	tableObjects := make(map[*core.Table]objectEvents)
	flushEventCount := uint(0)

	eventsWritten := 0
	err := func() error {
		// Stream in JSON event objects.
		decoder := json.NewDecoder(req.Body)

		// Set up events decoder listener
		events := make(chan map[string]interface{})
		eventErrors := make(chan error)
		go func(decoder *json.Decoder) {
			for {
				rawEvent := map[string]interface{}{}
				if err := decoder.Decode(&rawEvent); err == io.EOF {
					close(events)
					break
				} else if err != nil {
					eventErrors <- fmt.Errorf("Malformed json event: %v", err)
					break
				}
				events <- rawEvent
			}
		}(decoder)

	loop:
		for {

			// Read in a JSON object.
			rawEvent := map[string]interface{}{}
			select {
			case event, ok := <-events:
				if !ok {
					break loop
				} else {
					rawEvent = event
				}

			case err := <-eventErrors:
				return err

			}

			// Extract table name, if necessary.
			var eventTable *core.Table
			if table == nil {
				tableName, ok := rawEvent["table"].(string)
				if !ok {
					return fmt.Errorf("Table name required within event when using generic event stream.")
				}
				var err error
				eventTable, err = s.OpenTable(tableName)
				if err != nil {
					s.logger.Printf("ERR %v", err)
					return fmt.Errorf("Cannot open table %s: %+v", tableName, err)
				}
				delete(rawEvent, "table")
			} else {
				eventTable = table
			}

			// Extract the object identifier.
			objectId, ok := rawEvent["id"].(string)
			if !ok {
				return fmt.Errorf("Object identifier required")
			}

			// Convert to a Sky event and insert.
			event, err := eventTable.DeserializeEvent(rawEvent)
			if err != nil {
				return fmt.Errorf("Cannot deserialize: %v", err)
			}
			f, err := s.db.Factorizer(eventTable.Name)
			if err != nil {
				return fmt.Errorf("Cannot open factorizer: %v", err)
			}
			if err := f.FactorizeEvent(event, eventTable.PropertyFile(), true); err != nil {
				return fmt.Errorf("Cannot factorize: %v", err)
			}

			if _, ok := tableObjects[eventTable]; !ok {
				tableObjects[eventTable] = make(objectEvents)
			}

			// Add event to table buffer.
			tableObjects[eventTable][objectId] = append(tableObjects[eventTable][objectId], event)
			flushEventCount++

			// Flush events if exceeding threshold.
			if flushEventCount >= flushThreshold {

				flushedCount := 0
				s.logger.Printf("[STREAM] [FLUSH THRESHOLD] [FLUSHING] threshold=`%d` events=`%d`", flushThreshold, flushEventCount)
				for table, events := range tableObjects {
					count, err := s.flushTableEvents(table, events)
					if err != nil {
						return err
					}
					eventsWritten += count
					flushedCount += count
				}
				s.logger.Printf("[STREAM] [FLUSH THRESHOLD] [FLUSHED] events=`%d`", flushedCount)

				tableObjects = make(map[*core.Table]objectEvents)
				flushEventCount = 0
			}

		}

		return nil
	}()

	// Flush all events before closing the stream.
	if err == nil {
		err = func() error {
			flushedCount := 0
			s.logger.Printf("[STREAM] [END FLUSH] [FLUSHING] events=`%d`")
			for table, objects := range tableObjects {
				count, err := s.flushTableEvents(table, objects)
				if err != nil {
					return err
				}
				eventsWritten += count
				flushedCount += count
			}
			s.logger.Printf("[STREAM] [END FLUSH] [FLUSHED] events=`%d`", flushedCount)
			return nil
		}()
	}

	if err != nil {
		s.logger.Printf("ERR %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"message":"%v", "events_written":%v}`, err, eventsWritten)
		return
	}

	fmt.Fprintf(w, `{"events_written":%v}`, eventsWritten)

	s.logger.Printf("%s \"%s %s %s %d events OK\" %0.3f", req.RemoteAddr, req.Method, req.URL.Path, req.Proto, eventsWritten, time.Since(t0).Seconds())
}

// DELETE /tables/:name/objects/:objectId/events/:timestamp
func (s *Server) deleteEventHandler(w http.ResponseWriter, req *http.Request, params map[string]interface{}) (ret interface{}, err error) {
	vars := mux.Vars(req)
	t, err := s.OpenTable(vars["name"])
	if err != nil {
		return nil, err
	}

	timestamp, err := time.Parse(time.RFC3339, vars["timestamp"])
	if err != nil {
		return nil, fmt.Errorf("Unable to parse timestamp: %v", timestamp)
	}

	return nil, s.db.DeleteEvent(t.Name, vars["objectId"], timestamp)
}
