package server

import (
	"fmt"
	"os"
	"testing"
)

// Ensure that we can retrieve a list of all available tables on the server.
func TestServerGetTables(t *testing.T) {
	runTestServer(func(s *Server) {
		setupTestTable("foo")
		setupTestTable("bar")

		resp, err := sendTestHttpRequest("GET", "http://localhost:8586/tables", "application/json", ``)
		if err != nil {
			t.Fatalf("Unable to get tables: %v", err)
		}
		assertResponse(t, resp, 200, `[{"name":"bar"},{"name":"foo"}]`+"\n", "GET /tables failed.")
	})
}

// Ensure that we can retrieve a single table on the server.
func TestServerGetTable(t *testing.T) {
	runTestServer(func(s *Server) {
		// Make and open one table.
		resp, _ := sendTestHttpRequest("POST", "http://localhost:8586/tables", "application/json", `{"name":"foo"}`)
		resp.Body.Close()
		resp, err := sendTestHttpRequest("GET", "http://localhost:8586/tables/foo", "application/json", ``)
		if err != nil {
			t.Fatalf("Unable to get table: %v", err)
		}
		assertResponse(t, resp, 200, `{"name":"foo"}`+"\n", "GET /table failed.")
	})
}

// Ensure that we can create a new table through the server.
func TestServerCreateTable(t *testing.T) {
	runTestServer(func(s *Server) {
		resp, err := sendTestHttpRequest("POST", "http://localhost:8586/tables", "application/json", `{"name":"foo"}`)
		if err != nil {
			t.Fatalf("Unable to create table: %v", err)
		}
		assertResponse(t, resp, 200, `{"name":"foo"}`+"\n", "POST /tables failed.")
		if _, err := os.Stat(fmt.Sprintf("%v/foo", s.Path())); os.IsNotExist(err) {
			t.Fatalf("POST /tables did not create table.")
		}
	})
}

// Ensure that we can create a new table through the server.
func TestServerCreateTableWithProperties(t *testing.T) {
	runTestServer(func(s *Server) {
		properties := `{"id":-3,"name":"location_country","transient":true,"dataType":"factor"},` +
			`{"id":-2,"name":"referrer_host","transient":true,"dataType":"string"},` +
			`{"id":-1,"name":"referrer_source","transient":true,"dataType":"factor"},` +
			`{"id":1,"name":"session_token","transient":false,"dataType":"string"},` +
			`{"id":2,"name":"shop_id","transient":false,"dataType":"integer"}`
		definition := fmt.Sprintf(`{"name":"foo", "properties": [%s]}`, properties)
		resp, err := sendTestHttpRequest("POST", "http://localhost:8586/tables", "application/json", definition)
		if err != nil {
			t.Fatalf("Unable to create table: %v", err)
		}
		assertResponse(t, resp, 200, `{"name":"foo"}`+"\n", "POST /tables failed.")
		if _, err := os.Stat(fmt.Sprintf("%v/foo", s.Path())); os.IsNotExist(err) {
			t.Fatalf("POST /tables did not create table.")
		}
		resp, err = sendTestHttpRequest("GET", "http://localhost:8586/tables/foo/properties", "application/json", "")
		expected := `{"id":-3,"name":"referrer_source","transient":true,"dataType":"factor"},` +
			`{"id":-2,"name":"referrer_host","transient":true,"dataType":"string"},` +
			`{"id":-1,"name":"location_country","transient":true,"dataType":"factor"},` +
			`{"id":1,"name":"session_token","transient":false,"dataType":"string"},` +
			`{"id":2,"name":"shop_id","transient":false,"dataType":"integer"}`
		assertResponse(t, resp, 200, "["+expected+"]\n", "POST /tables failed.")
	})
}

// Ensure that we can delete a table through the server.
func TestServerDeleteTable(t *testing.T) {
	runTestServer(func(s *Server) {
		// Create table.
		resp, err := sendTestHttpRequest("POST", "http://localhost:8586/tables", "application/json", `{"name":"foo"}`)
		if err != nil {
			t.Fatalf("Unable to create table: %v", err)
		}
		assertResponse(t, resp, 200, `{"name":"foo"}`+"\n", "POST /tables failed.")
		if _, err := os.Stat(fmt.Sprintf("%v/foo", s.Path())); os.IsNotExist(err) {
			t.Fatalf("POST /tables did not create table.")
		}

		// Delete table.
		resp, _ = sendTestHttpRequest("DELETE", "http://localhost:8586/tables/foo", "application/json", ``)
		assertResponse(t, resp, 200, "", "DELETE /tables/:name failed.")
		if _, err := os.Stat(fmt.Sprintf("%v/foo", s.Path())); !os.IsNotExist(err) {
			t.Fatalf("DELETE /tables/:name did not delete table.")
		}
	})
}

// Ensure that we can retrieve a list of object keys from the server for a table.
func TestServerTableKeys(t *testing.T) {
	runTestServer(func(s *Server) {
		setupTestTable("foo")
		setupTestProperty("foo", "value", true, "integer")
		setupTestData(t, "foo", [][]string{
			[]string{"a0", "2012-01-01T00:00:00Z", `{"data":{"value":1}}`},
			[]string{"a1", "2012-01-01T00:00:00Z", `{"data":{"value":2}}`},
			[]string{"a1", "2012-01-01T00:00:01Z", `{"data":{"value":3}}`},
			[]string{"a2", "2012-01-01T00:00:00Z", `{"data":{"value":4}}`},
			[]string{"a2", "2012-01-01T00:00:01Z", `{"data":{"value":4}}`},
			[]string{"a3", "2012-01-01T00:00:00Z", `{"data":{"value":5}}`},
		})

		setupTestTable("bar")
		setupTestProperty("bar", "value", true, "integer")
		setupTestData(t, "bar", [][]string{
			[]string{"b0", "2012-01-01T00:00:00Z", `{"data":{"value":1}}`},
			[]string{"b1", "2012-01-01T00:00:00Z", `{"data":{"value":2}}`},
			[]string{"b1", "2012-01-01T00:00:01Z", `{"data":{"value":3}}`},
			[]string{"b2", "2012-01-01T00:00:00Z", `{"data":{"value":4}}`},
			[]string{"b3", "2012-01-01T00:00:00Z", `{"data":{"value":5}}`},
		})

		resp, _ := sendTestHttpRequest("GET", "http://localhost:8586/tables/foo/keys", "application/json", "")
		assertResponse(t, resp, 200, `["a0","a1","a2","a3"]`+"\n", "POST /tables/:name/keys failed.")
	})
}

func TestServerTableStats(t *testing.T) {
	runTestServer(func(s *Server) {
		setupTestTable("foo")
		setupTestProperty("foo", "value", true, "integer")
		setupTestData(t, "foo", [][]string{
			[]string{"a0", "2012-01-01T00:00:00Z", `{"data":{"value":1}}`},
		})

		resp, _ := sendTestHttpRequest("GET", "http://localhost:8586/tables/foo/stats", "application/json", "")
		assertResponse(t, resp, 200, `{"branchPages":0,"branchOverflow":0,"leafPages":18,"leafOverflow":0,"keyCount":3,"depth":2,"branchAlloc":0,"branchInuse":0,"leafAlloc":73728,"leafInuse":284}`+"\n", "GET /tables/:name/stats failed.")
	})
}
