package server

import (
	"github.com/stretchr/testify/assert"
	"net/http"
	"testing"
	"time"
)

// Ensure that we can put an event on the server.
func TestServerUpdateEvents(t *testing.T) {
	runTestServer(func(s *Server) {
		setupTestTable("foo")
		setupTestProperty("foo", "bar", false, "factor")
		setupTestProperty("foo", "baz", true, "integer")

		// Send two new events.
		resp, _ := sendTestHttpRequest("PUT", "http://localhost:8586/tables/foo/objects/xyz/events/2012-01-01T02:00:00.123456111Z", "application/json", `{"data":{"bar":"myValue", "baz":12}}`)
		assertResponse(t, resp, 200, "", "PUT /tables/:name/objects/:objectId/events failed.")
		resp, _ = sendTestHttpRequest("PUT", "http://localhost:8586/tables/foo/objects/xyz/events/2012-01-01T03:00:00Z", "application/json", `{"data":{"bar":"myValue2"}}`)
		assertResponse(t, resp, 200, "", "PUT /tables/:name/objects/:objectId/events failed.")

		// Replace the first one.
		resp, _ = sendTestHttpRequest("PUT", "http://localhost:8586/tables/foo/objects/xyz/events/2012-01-01T02:00:00.123456222Z", "application/json", `{"data":{"bar":"myValue3", "baz":1000}}`)
		assertResponse(t, resp, 200, "", "PUT /tables/:name/objects/:objectId/events failed.")

		// Merge the second one.
		resp, _ = sendTestHttpRequest("PATCH", "http://localhost:8586/tables/foo/objects/xyz/events/2012-01-01T03:00:00Z", "application/json", `{"data":{"bar":"myValue2", "baz":20}}`)
		assertResponse(t, resp, 200, "", "PATCH /tables/:name/objects/:objectId/events failed.")

		// Check our work.
		resp, _ = sendTestHttpRequest("GET", "http://localhost:8586/tables/foo/objects/xyz/events", "application/json", "")
		assertResponse(t, resp, 200, `[{"data":{"bar":"myValue3","baz":1000},"timestamp":"2012-01-01T02:00:00.123456Z"},{"data":{"bar":"myValue2","baz":20},"timestamp":"2012-01-01T03:00:00Z"}]`+"\n", "GET /tables/:name/objects/:objectId/events failed.")

		// Grab a single event.
		resp, _ = sendTestHttpRequest("GET", "http://localhost:8586/tables/foo/objects/xyz/events/2012-01-01T03:00:00Z", "application/json", "")
		assertResponse(t, resp, 200, `{"data":{"bar":"myValue2","baz":20},"timestamp":"2012-01-01T03:00:00Z"}`+"\n", "GET /tables/:name/objects/:objectId/events/:timestamp failed.")
	})
}

// Ensure that we can delete all events for an object.
func TestServerDeleteEvent(t *testing.T) {
	runTestServer(func(s *Server) {
		setupTestTable("foo")
		setupTestProperty("foo", "bar", false, "string")

		// Send two new events.
		resp, _ := sendTestHttpRequest("PUT", "http://localhost:8586/tables/foo/objects/xyz/events/2012-01-01T02:00:00Z", "application/json", `{"data":{"bar":"myValue"}}`)
		assertResponse(t, resp, 200, "", "PUT /tables/:name/objects/:objectId/events failed.")
		resp, _ = sendTestHttpRequest("PUT", "http://localhost:8586/tables/foo/objects/xyz/events/2012-01-01T03:00:00Z", "application/json", `{"data":{"bar":"myValue2"}}`)
		assertResponse(t, resp, 200, "", "PUT /tables/:name/objects/:objectId/events failed.")

		// Delete one of the events.
		resp, _ = sendTestHttpRequest("DELETE", "http://localhost:8586/tables/foo/objects/xyz/events/2012-01-01T02:00:00Z", "application/json", "")
		assertResponse(t, resp, 200, "", "DELETE /tables/:name/objects/:objectId/events failed.")

		// Check our work.
		resp, _ = sendTestHttpRequest("GET", "http://localhost:8586/tables/foo/objects/xyz/events", "application/json", "")
		assertResponse(t, resp, 200, `[{"data":{"bar":"myValue2"},"timestamp":"2012-01-01T03:00:00Z"}]`+"\n", "GET /tables/:name/objects/:objectId/events failed.")
	})
}

// Ensure that we can delete all events for an object.
func TestServerDeleteEvents(t *testing.T) {
	runTestServer(func(s *Server) {
		setupTestTable("foo")
		setupTestProperty("foo", "bar", false, "string")

		// Send two new events.
		resp, _ := sendTestHttpRequest("PUT", "http://localhost:8586/tables/foo/objects/xyz/events/2012-01-01T02:00:00Z", "application/json", `{"data":{"bar":"myValue"}}`)
		assertResponse(t, resp, 200, "", "PUT /tables/:name/objects/:objectId/events failed.")
		resp, _ = sendTestHttpRequest("PUT", "http://localhost:8586/tables/foo/objects/xyz/events/2012-01-01T03:00:00Z", "application/json", `{"data":{"bar":"myValue2"}}`)
		assertResponse(t, resp, 200, "", "PUT /tables/:name/objects/:objectId/events failed.")

		// Delete the events.
		resp, _ = sendTestHttpRequest("DELETE", "http://localhost:8586/tables/foo/objects/xyz/events", "application/json", "")
		assertResponse(t, resp, 200, "", "DELETE /tables/:name/objects/:objectId/events failed.")

		// Check our work.
		resp, _ = sendTestHttpRequest("GET", "http://localhost:8586/tables/foo/objects/xyz/events", "application/json", "")
		assertResponse(t, resp, 200, "[]\n", "GET /tables/:name/objects/:objectId/events failed.")
	})
}

// Ensure that we can put multiple events on the server at once.
func TestServerStreamUpdateEvents(t *testing.T) {
	runTestServer(func(s *Server) {
		setupTestTable("foo")
		setupTestProperty("foo", "bar", false, "string")
		setupTestProperty("foo", "baz", true, "integer")

		// Send two new events in one request.
		resp, _ := sendTestHttpRequest("PATCH", "http://localhost:8586/tables/foo/events", "application/json", `{"id":"xyz","timestamp":"2012-01-01T02:00:00Z","data":{"bar":"myValue", "baz":12}}{"id":"xyz","timestamp":"2012-01-01T03:00:00Z","data":{"bar":"myValue2"}}`)
		assertResponse(t, resp, 200, `{"events_written":2}`, "PATCH /tables/:name/events failed.")

		// Check our work.
		resp, _ = sendTestHttpRequest("GET", "http://localhost:8586/tables/foo/objects/xyz/events", "application/json", "")
		assertResponse(t, resp, 200, `[{"data":{"bar":"myValue","baz":12},"timestamp":"2012-01-01T02:00:00Z"},{"data":{"bar":"myValue2"},"timestamp":"2012-01-01T03:00:00Z"}]`+"\n", "GET /tables/:name/objects/:objectId/events failed.")
	})
}

// Ensure that streamed events are flushed when count exceeds specified threshold.
func TestServerStreamUpdateEventsFlushesOnThreshold(t *testing.T) {
	runTestServer(func(s *Server) {
		setupTestTable("foo")
		setupTestProperty("foo", "bar", false, "string")
		setupTestProperty("foo", "baz", true, "integer")
		s.streamFlushThreshold = 2
		client, err := NewStreamingClient(t, "http://localhost:8586/tables/foo/events")
		assert.NoError(t, err)

		// Send a single event.
		client.Write(`{"id":"xyz","table":"foo","timestamp":"2012-01-01T02:00:00Z","data":{"bar":"myValue", "baz":12}}` + "\n")
		client.Flush()

		// Assert that the event was NOT flushed
		resp, err := sendTestHttpRequest("GET", "http://localhost:8586/tables/foo/objects/xyz/events", "application/json", "")
		assert.NoError(t, err)
		assertResponse(t, resp, 200, `[]`+"\n", "GET /tables/foo/events first not flushed failed.")

		// Send a second event.
		client.Write(`{"id":"xyz","table":"foo","timestamp":"2012-01-01T03:00:00Z","data":{"bar":"myValue2"}}` + "\n")
		client.Flush()

		// Give sky a small amount of time to write the events.
		time.Sleep(100 * time.Millisecond)

		// Assert that the events were flushed
		resp, err = sendTestHttpRequest("GET", "http://localhost:8586/tables/foo/objects/xyz/events", "application/json", "")
		assert.NoError(t, err)
		assertResponse(t, resp, 200, `[{"data":{"bar":"myValue","baz":12},"timestamp":"2012-01-01T02:00:00Z"},{"data":{"bar":"myValue2"},"timestamp":"2012-01-01T03:00:00Z"}]`+"\n", "GET /tables/foo/events second flushed failed.")

		// Send another event.
		client.Write(`{"id":"abc","table":"foo","timestamp":"2012-01-01T02:00:00Z","data":{"bar":"myValue", "baz":12}}` + "\n")
		client.Flush()

		// Assert that the event was NOT flushed
		resp, err = sendTestHttpRequest("GET", "http://localhost:8586/tables/foo/objects/xyz1/events", "application/json", "")
		assert.NoError(t, err)
		assertResponse(t, resp, 200, `[]`+"\n", "GET /tables/foo/events third event not flushed failed.")

		// Send a second event.
		client.Write(`{"id":"abc","table":"foo","timestamp":"2012-01-01T03:00:00Z","data":{"bar":"myValue2"}}` + "\n")
		client.Flush()

		// Give sky a small amount of time to write the events.
		time.Sleep(100 * time.Millisecond)

		// Assert that the events were flushed
		resp, err = sendTestHttpRequest("GET", "http://localhost:8586/tables/foo/objects/abc/events", "application/json", "")
		assert.NoError(t, err)
		assertResponse(t, resp, 200, `[{"data":{"bar":"myValue","baz":12},"timestamp":"2012-01-01T02:00:00Z"},{"data":{"bar":"myValue2"},"timestamp":"2012-01-01T03:00:00Z"}]`+"\n", "GET /tables/foo/events fourth flushed failed.")

		// Close streaming request.
		ret := client.Close()

		// Assert that 4 events were written during stream.
		resp = ret.(*http.Response)
		assertResponse(t, resp, 200, `{"events_written":4}`, "PATCH /events failed.")

		// Ensure events exist.
		resp, err = sendTestHttpRequest("GET", "http://localhost:8586/tables/foo/objects/xyz/events", "application/json", "")
		assert.NoError(t, err)
		assertResponse(t, resp, 200, `[{"data":{"bar":"myValue","baz":12},"timestamp":"2012-01-01T02:00:00Z"},{"data":{"bar":"myValue2"},"timestamp":"2012-01-01T03:00:00Z"}]`+"\n", "GET /tables/foo/events failed.")

		// Ensure events exist.
		resp, err = sendTestHttpRequest("GET", "http://localhost:8586/tables/foo/objects/abc/events", "application/json", "")
		assert.NoError(t, err)
		assertResponse(t, resp, 200, `[{"data":{"bar":"myValue","baz":12},"timestamp":"2012-01-01T02:00:00Z"},{"data":{"bar":"myValue2"},"timestamp":"2012-01-01T03:00:00Z"}]`+"\n", "GET /tables/foo/events failed.")
	})
}

// Ensure that streamed events are flushed when count exceeds specified threshold.
func TestServerStreamUpdateEventsWithFlushThresholdParamFlushesOnThreshold(t *testing.T) {
	runTestServer(func(s *Server) {
		setupTestTable("foo")
		setupTestProperty("foo", "bar", false, "string")
		setupTestProperty("foo", "baz", true, "integer")
		client, err := NewStreamingClient(t, "http://localhost:8586/tables/foo/events?flush-threshold=2")
		assert.NoError(t, err)

		// Send a single event.
		client.Write(`{"id":"xyz","table":"foo","timestamp":"2012-01-01T02:00:00Z","data":{"bar":"myValue", "baz":12}}` + "\n")
		client.Flush()

		// Assert that the event was NOT flushed
		resp, err := sendTestHttpRequest("GET", "http://localhost:8586/tables/foo/objects/xyz/events", "application/json", "")
		assert.NoError(t, err)
		assertResponse(t, resp, 200, `[]`+"\n", "GET /events failed.")

		// Send a second event.
		client.Write(`{"id":"xyz","table":"foo","timestamp":"2012-01-01T03:00:00Z","data":{"bar":"myValue2"}}` + "\n")
		client.Flush()

		// Give sky a small amount of time to write the events.
		time.Sleep(100 * time.Millisecond)

		// Assert that the events were flushed
		resp, err = sendTestHttpRequest("GET", "http://localhost:8586/tables/foo/objects/xyz/events", "application/json", "")
		assert.NoError(t, err)
		assertResponse(t, resp, 200, `[{"data":{"bar":"myValue","baz":12},"timestamp":"2012-01-01T02:00:00Z"},{"data":{"bar":"myValue2"},"timestamp":"2012-01-01T03:00:00Z"}]`+"\n", "GET /events failed.")

		// Send another event.
		client.Write(`{"id":"abc","table":"foo","timestamp":"2012-01-01T02:00:00Z","data":{"bar":"myValue", "baz":12}}` + "\n")
		client.Flush()

		// Assert that the event was NOT flushed
		resp, err = sendTestHttpRequest("GET", "http://localhost:8586/tables/foo/objects/xyz1/events", "application/json", "")
		assert.NoError(t, err)
		assertResponse(t, resp, 200, `[]`+"\n", "GET /events failed.")

		// Send a second event.
		client.Write(`{"id":"abc","table":"foo","timestamp":"2012-01-01T03:00:00Z","data":{"bar":"myValue2"}}` + "\n")
		client.Flush()

		// Give sky a small amount of time to write the events.
		time.Sleep(100 * time.Millisecond)

		// Assert that the events were flushed
		resp, err = sendTestHttpRequest("GET", "http://localhost:8586/tables/foo/objects/abc/events", "application/json", "")
		assert.NoError(t, err)
		assertResponse(t, resp, 200, `[{"data":{"bar":"myValue","baz":12},"timestamp":"2012-01-01T02:00:00Z"},{"data":{"bar":"myValue2"},"timestamp":"2012-01-01T03:00:00Z"}]`+"\n", "GET /events failed.")

		// Close streaming request.
		ret := client.Close()

		// Assert that 2 events were written during stream.
		resp = ret.(*http.Response)
		assertResponse(t, resp, 200, `{"events_written":4}`, "PATCH /events failed.")

		// Ensure events exist.
		resp, err = sendTestHttpRequest("GET", "http://localhost:8586/tables/foo/objects/xyz/events", "application/json", "")
		assert.NoError(t, err)
		assertResponse(t, resp, 200, `[{"data":{"bar":"myValue","baz":12},"timestamp":"2012-01-01T02:00:00Z"},{"data":{"bar":"myValue2"},"timestamp":"2012-01-01T03:00:00Z"}]`+"\n", "GET /events failed.")

		// Ensure events exist.
		resp, err = sendTestHttpRequest("GET", "http://localhost:8586/tables/foo/objects/abc/events", "application/json", "")
		assert.NoError(t, err)
		assertResponse(t, resp, 200, `[{"data":{"bar":"myValue","baz":12},"timestamp":"2012-01-01T02:00:00Z"},{"data":{"bar":"myValue2"},"timestamp":"2012-01-01T03:00:00Z"}]`+"\n", "GET /events failed.")
	})
}
