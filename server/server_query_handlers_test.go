package server

import (
	"encoding/json"
	"testing"
)

// Ensure error when no prefix.
func TestServerQueryFailsWithNoPrefix(t *testing.T) {
	runTestServer(func(s *Server) {
		setupTestTable("foo")
		setupTestProperty("foo", "fruit", true, "string")
		setupTestProperty("foo", "num", true, "integer")
		setupTestData(t, "foo", [][]string{
			[]string{"a0", "2012-01-01T00:00:00Z", `{"data":{"fruit":"apple"}}`},
			[]string{"a1", "2012-01-01T00:00:00Z", `{"data":{"fruit":"grape"}}`},
			[]string{"a1", "2012-01-01T00:00:01Z", `{"data":{"num":12}}`},
			[]string{"a2", "2012-01-01T00:00:00Z", `{"data":{"fruit":"orange"}}`},
			[]string{"a3", "2012-01-01T00:00:00Z", `{"data":{"fruit":"apple"}}`},
		})

		setupTestTable("bar")
		setupTestProperty("bar", "fruit", true, "string")
		setupTestData(t, "bar", [][]string{
			[]string{"a0", "2012-01-01T00:00:00Z", `{"data":{"fruit":"grape"}}`},
		})

		// Run query.
		query := `{"query":{"statements":"SELECT count()"}}`
		resp, _ := sendTestHttpRequest("POST", "http://localhost:8586/tables/foo/query", "application/json", query)
		assertResponse(t, resp, 500, `{"message":"Prefix is required on queries."}`+"\n", "POST /tables/:name/query failed.")
	})
}

// Ensure that we can query the server for a count of events.
func TestServerSimpleCountQuery(t *testing.T) {
	runTestServer(func(s *Server) {
		setupTestTable("foo")
		setupTestProperty("foo", "fruit", true, "string")
		setupTestProperty("foo", "num", true, "integer")
		setupTestData(t, "foo", [][]string{
			[]string{"a0", "2012-01-01T00:00:00Z", `{"data":{"fruit":"apple"}}`},
			[]string{"a1", "2012-01-01T00:00:00Z", `{"data":{"fruit":"grape"}}`},
			[]string{"a1", "2012-01-01T00:00:01Z", `{"data":{"num":12}}`},
			[]string{"a2", "2012-01-01T00:00:00Z", `{"data":{"fruit":"orange"}}`},
			[]string{"a3", "2012-01-01T00:00:00Z", `{"data":{"fruit":"apple"}}`},
		})

		setupTestTable("bar")
		setupTestProperty("bar", "fruit", true, "string")
		setupTestData(t, "bar", [][]string{
			[]string{"a0", "2012-01-01T00:00:00Z", `{"data":{"fruit":"grape"}}`},
		})

		// Run query.
		query := `{"query":{"statements":"SELECT count()"}}`
		resp, _ := sendTestHttpRequest("POST", "http://localhost:8586/tables/foo/query?prefix=a", "application/json", query)
		assertResponse(t, resp, 200, `{"count":5}`+"\n", "POST /tables/:name/query failed.")
		resp, _ = sendTestHttpRequest("POST", "http://localhost:8586/tables/bar/query?prefix=a", "application/json", query)
		assertResponse(t, resp, 200, `{"count":1}`+"\n", "POST /tables/:name/query failed.")
	})
}

// Ensure that we can query the server for a count of events.
func TestServerSimpleCountQueryWithPrefixInBody(t *testing.T) {
	runTestServer(func(s *Server) {
		setupTestTable("foo")
		setupTestProperty("foo", "fruit", true, "string")
		setupTestProperty("foo", "num", true, "integer")
		setupTestData(t, "foo", [][]string{
			[]string{"a0", "2012-01-01T00:00:00Z", `{"data":{"fruit":"apple"}}`},
			[]string{"a1", "2012-01-01T00:00:00Z", `{"data":{"fruit":"grape"}}`},
			[]string{"a1", "2012-01-01T00:00:01Z", `{"data":{"num":12}}`},
			[]string{"a2", "2012-01-01T00:00:00Z", `{"data":{"fruit":"orange"}}`},
			[]string{"a3", "2012-01-01T00:00:00Z", `{"data":{"fruit":"apple"}}`},
			[]string{"b2", "2012-01-01T00:00:00Z", `{"data":{"fruit":"orange"}}`},
			[]string{"b3", "2012-01-01T00:00:00Z", `{"data":{"fruit":"apple"}}`},
		})

		setupTestTable("bar")
		setupTestProperty("bar", "fruit", true, "string")
		setupTestData(t, "bar", [][]string{
			[]string{"a0", "2012-01-01T00:00:00Z", `{"data":{"fruit":"grape"}}`},
		})

		// Run query.
		query := `{"query":"SELECT count()", "prefix": "a"}`
		resp, _ := sendTestHttpRequest("POST", "http://localhost:8586/tables/foo/query", "application/json", query)
		assertResponse(t, resp, 200, `{"count":5}`+"\n", "POST /tables/:name/query failed.")
		resp, _ = sendTestHttpRequest("POST", "http://localhost:8586/tables/bar/query", "application/json", query)
		assertResponse(t, resp, 200, `{"count":1}`+"\n", "POST /tables/:name/query failed.")
	})
}

// Ensure that we can query the server for a count of events with a single dimension.
func TestServerOneDimensionCountQuery(t *testing.T) {
	runTestServer(func(s *Server) {
		setupTestTable("foo")
		setupTestProperty("foo", "fruit", true, "string")
		setupTestProperty("foo", "num", true, "integer")
		setupTestData(t, "foo", [][]string{
			[]string{"b0", "2012-01-01T00:00:00Z", `{"data":{"fruit":"apple"}}`},
			[]string{"b1", "2012-01-01T00:00:00Z", `{"data":{"fruit":"grape"}}`},
			[]string{"b1", "2012-01-01T00:00:01Z", `{"data":{"num":12}}`},
			[]string{"b2", "2012-01-01T00:00:00Z", `{"data":{"fruit":"orange"}}`},
			[]string{"b3", "2012-01-01T00:00:00Z", `{"data":{"fruit":"apple"}}`},
		})

		// Run query.
		query := `{"query":"SELECT count() AS count GROUP BY fruit"}`
		//_codegen(t, "foo", query)
		resp, _ := sendTestHttpRequest("POST", "http://localhost:8586/tables/foo/query?prefix=b", "application/json", query)
		assertResponse(t, resp, 200, `{"fruit":{"":{"count":1},"apple":{"count":2},"grape":{"count":1},"orange":{"count":1}}}`+"\n", "POST /tables/:name/query failed.")
	})
}

// Ensure that we can query the server for multiple selections with multiple dimensions.
func TestServerMultiDimensionalQuery(t *testing.T) {
	runTestServer(func(s *Server) {
		setupTestTable("foo")
		setupTestProperty("foo", "gender", false, "string")
		setupTestProperty("foo", "state", false, "factor")
		setupTestProperty("foo", "price", true, "float")
		setupTestProperty("foo", "num", true, "integer")
		setupTestData(t, "foo", [][]string{
			[]string{"c0", "2012-01-01T00:00:00Z", `{"data":{"gender":"m", "state":"NY", "price":100}}`},
			[]string{"c0", "2012-01-01T00:00:01Z", `{"data":{"price":200}}`},
			[]string{"c0", "2012-01-01T00:00:02Z", `{"data":{"state":"CA","price":10}}`},

			[]string{"c1", "2012-01-01T00:00:00Z", `{"data":{"gender":"m", "state":"CA", "price":20}}`},
			[]string{"c1", "2012-01-01T00:00:01Z", `{"data":{"num":1000}}`},

			[]string{"c2", "2012-01-01T00:00:00Z", `{"data":{"gender":"f", "state":"NY", "price":30}}`},
		})

		// Run query.
		query := `{"query":"` +
			`SELECT count() AS count, sum(price) AS sum GROUP BY gender, state INTO \"s1\" ` +
			`SELECT min(price) AS minimum, max(price) AS maximum GROUP BY gender, state ` +
			`SELECT sum((price + num) * 2) AS sum INTO \"_\" ` +
			`SELECT avg(price) AS average INTO \"_\" ` +
			`SELECT count(DISTINCT price) AS distinct INTO \"_\" ` +
			`"}`
		//_codegen(t, "foo", query)
		resp, _ := sendTestHttpRequest("POST", "http://localhost:8586/tables/foo/query?prefix=c", "application/json", query)
		assertResponse(t, resp, 200, `{"_":{"average":60,"distinct":6,"sum":2720},"gender":{"f":{"state":{"NY":{"maximum":30,"minimum":30}}},"m":{"state":{"CA":{"maximum":20,"minimum":0},"NY":{"maximum":200,"minimum":100}}}},"s1":{"gender":{"f":{"state":{"NY":{"count":1,"sum":30}}},"m":{"state":{"CA":{"count":3,"sum":30},"NY":{"count":2,"sum":300}}}}}}`+"\n", "POST /tables/:name/query failed.")
	})
}

// Ensure that we can perform a non-sessionized funnel analysis.
func TestServerFunnelAnalysisQuery(t *testing.T) {
	runTestServer(func(s *Server) {
		setupTestTable("foo")
		setupTestProperty("foo", "action", true, "factor")
		setupTestData(t, "foo", [][]string{
			// A0[0..0]..A1[1..2] occurs twice for this object.
			[]string{"ad0", "2012-01-01T00:00:00Z", `{"data":{"action":"A0"}}`},
			[]string{"ad0", "2012-01-01T00:00:01Z", `{"data":{"action":"A1"}}`},
			[]string{"ad0", "2012-01-01T00:00:02Z", `{"data":{"action":"A2"}}`},
			[]string{"ad0", "2012-01-01T12:00:00Z", `{"data":{"action":"A0"}}`},
			[]string{"ad0", "2012-01-01T13:00:00Z", `{"data":{"action":"A0"}}`},
			[]string{"ad0", "2012-01-01T14:00:00Z", `{"data":{"action":"A1"}}`},

			// A0[0..0]..A1[1..2] occurs once for this object. (Second time matches A1[1..3]).
			[]string{"ae1", "2012-01-01T00:00:00Z", `{"data":{"action":"A0"}}`},
			[]string{"ae1", "2012-01-01T00:00:01Z", `{"data":{"action":"A0"}}`},
			[]string{"ae1", "2012-01-01T00:00:02Z", `{"data":{"action":"A1"}}`},
			[]string{"ae1", "2012-01-02T00:00:00Z", `{"data":{"action":"A0"}}`},
			[]string{"ae1", "2012-01-02T00:00:01Z", `{"data":{"action":"A0"}}`},
			[]string{"ae1", "2012-01-02T00:00:02Z", `{"data":{"action":"A0"}}`},
			[]string{"ae1", "2012-01-02T00:00:03Z", `{"data":{"action":"A1"}}`},
		})

		// Run query.
		query := `{"query":"` +
			`WHEN action == 'A0' THEN ` +
			`  WHEN action == 'A1' WITHIN 1..2 STEPS THEN ` +
			`    SELECT count() AS count GROUP BY action ` +
			`  END ` +
			`END` +
			`"}`
		resp, _ := sendTestHttpRequest("POST", "http://localhost:8586/tables/foo/query?prefix=a", "application/json", query)
		assertResponse(t, resp, 200, `{"action":{"A1":{"count":3}}}`+"\n", "POST /tables/:name/query failed.")
	})
}

// Ensure that we can factorize overlapping factors.
func TestServerFactorizeOverlappingQueries(t *testing.T) {
	runTestServer(func(s *Server) {
		setupTestTable("foo")
		setupTestProperty("foo", "action", false, "factor")
		setupTestData(t, "foo", [][]string{
			[]string{"f0", "2012-01-01T00:00:00Z", `{"data":{"action":"A0"}}`},
		})

		// Run query.
		query := `{"query":"` +
			`SELECT count() AS count1 GROUP BY action INTO \"q\" ` +
			`SELECT count() AS count2 GROUP BY action INTO \"q\" ` +
			`"}`
		//_codegen(t, "foo", query)
		resp, _ := sendTestHttpRequest("POST", "http://localhost:8586/tables/foo/query?prefix=f", "application/json", query)
		assertResponse(t, resp, 200, `{"q":{"action":{"A0":{"count1":1,"count2":1}}}}`+"\n", "POST /tables/:name/query failed.")
	})
}

// Ensure that we can perform a sessionized funnel analysis.
func TestServerSessionizedFunnelAnalysisQuery(t *testing.T) {
	runTestServer(func(s *Server) {
		setupTestTable("foo")
		setupTestProperty("foo", "action", false, "factor")
		setupTestData(t, "foo", [][]string{
			// A0[0..0]..A1[1..1] occurs once for this object. The second one is broken across sessions.
			[]string{"f0", "1970-01-01T00:00:01Z", `{"data":{"action":"A0"}}`},
			[]string{"f0", "1970-01-01T01:59:59Z", `{"data":{"action":"A1"}}`},
			[]string{"f0", "1970-01-02T00:00:00Z", `{"data":{"action":"A0"}}`},
			[]string{"f0", "1970-01-02T02:00:00Z", `{"data":{"action":"A1"}}`},
		})

		// Run query.
		query := `{
			"statements":"` +
			`FOR EACH SESSION DELIMITED BY 2 HOURS ` +
			`  FOR EACH EVENT ` +
			`    WHEN action == \"A0\" THEN ` +
			`      WHEN action == \"A1\" WITHIN 1..1 STEPS THEN ` +
			`        SELECT count() AS count GROUP BY action ` +
			`      END ` +
			`    END ` +
			`  END ` +
			`END` +
			`"}`
		// _codegen(t, "foo", query)
		resp, _ := sendTestHttpRequest("POST", "http://localhost:8586/tables/foo/query?prefix=f", "application/json", query)
		assertResponse(t, resp, 200, `{"action":{"A1":{"count":1}}}`+"\n", "POST /tables/:name/query failed.")
	})
}

// Ensure that we can access system variables like eos and eof.
func TestServerSystemVariables(t *testing.T) {
	runTestServer(func(s *Server) {
		setupTestTable("foo")
		setupTestProperty("foo", "action", false, "factor")
		setupTestData(t, "foo", [][]string{
			[]string{"f0", "1970-01-01T00:00:01Z", `{"data":{"action":"A0"}}`},
			[]string{"f0", "1970-01-01T01:59:59Z", `{"data":{"action":"A1"}}`},
			[]string{"f0", "1970-01-02T00:00:00Z", `{"data":{"action":"A0"}}`},
			[]string{"f0", "1970-01-02T02:00:00Z", `{"data":{"action":"A1"}}`},

			[]string{"f1", "1970-01-01T02:00:00Z", `{"data":{"action":"A0"}}`},
		})

		query := `
			FOR EACH SESSION DELIMITED BY 2 HOURS
			  FOR EACH EVENT
				SELECT count() AS count GROUP BY action, @@eos, @@eof
			  END
			END
		`
		q, _ := json.Marshal(query)
		resp, _ := sendTestHttpRequest("POST", "http://localhost:8586/tables/foo/query?prefix=f", "application/json", `{"query":`+string(q)+`}`)
		assertResponse(t, resp, 200, `{"action":{"A0":{"eos":{"false":{"eof":{"false":{"count":1}}},"true":{"eof":{"false":{"count":1},"true":{"count":1}}}}},"A1":{"eos":{"true":{"eof":{"false":{"count":1},"true":{"count":1}}}}}}}`+"\n", "POST /tables/:name/query failed.")
	})
}

// Ensure that we can utilitize the timestamp in the query.
func TestServerTimestampQuery(t *testing.T) {
	runTestServer(func(s *Server) {
		setupTestTable("foo")
		setupTestProperty("foo", "action", true, "factor")
		setupTestData(t, "foo", [][]string{
			[]string{"00", "1970-01-01T00:00:00Z", `{"data":{"action":"A0"}}`},
			[]string{"00", "1970-01-01T00:00:02Z", `{"data":{"action":"A1"}}`},
			[]string{"00", "1970-01-01T00:00:04Z", `{"data":{"action":"A2"}}`},
			[]string{"00", "1970-01-01T00:00:06Z", `{"data":{"action":"A3"}}`},
			[]string{"00", "1970-01-01T00:01:00Z", `{"data":{"action":"A4"}}`},
			[]string{"01", "1970-01-01T00:00:02Z", `{"data":{"action":"A5"}}`},
			[]string{"02", "1970-01-01T00:00:02Z", `{"data":{"action":"A5"}}`},
		})

		// Run query.
		query := `{"query":"` +
			`WHEN timestamp >= 2 && timestamp < 6 THEN ` +
			`  SELECT count() AS count, sum(timestamp) AS tsSum GROUP BY action ` +
			`END` +
			`"}`
		resp, _ := sendTestHttpRequest("POST", "http://localhost:8586/tables/foo/query?prefix=0", "application/json", query)
		assertResponse(t, resp, 200, `{"action":{"A1":{"count":1,"tsSum":2},"A2":{"count":1,"tsSum":4},"A5":{"count":2,"tsSum":4}}}`+"\n", "POST /tables/:name/query failed.")
	})
}

// Ensure that we can use declared variables.
func TestServerFSMQuery(t *testing.T) {
	runTestServer(func(s *Server) {
		setupTestTable("foo")
		setupTestProperty("foo", "action", true, "factor")
		setupTestData(t, "foo", [][]string{
			[]string{"00", "1970-01-01T00:00:00Z", `{"data":{"action":"home"}}`},
			[]string{"00", "1970-01-01T00:00:02Z", `{"data":{"action":"signup"}}`},
			[]string{"00", "1970-01-01T00:00:03Z", `{"data":{"action":"signed_up"}}`},
			[]string{"00", "1970-01-01T00:00:04Z", `{"data":{"action":"pricing"}}`},
			[]string{"00", "1970-01-02T00:00:00Z", `{"data":{"action":"cancel"}}`},
			[]string{"00", "1970-01-03T00:00:00Z", `{"data":{"action":"home"}}`},

			[]string{"01", "1970-01-01T00:00:00Z", `{"data":{"action":"home"}}`},
			[]string{"01", "1970-01-01T00:00:02Z", `{"data":{"action":"cancel"}}`},
		})

		// Run query.
		query := `
			DECLARE state AS INTEGER
			WHEN state == 0 THEN
				SET state = 1
				SELECT count() AS count INTO "visited"
			END
			WHEN state == 1 && action == "signed_up" THEN
				SET state = 2
				SELECT count() AS count INTO "registered"
			END
			WHEN state == 2 && action == "cancel" THEN
				SET state = 3
				SELECT count() AS count INTO "cancelled"
			END
		`
		q, _ := json.Marshal(query)
		resp, _ := sendTestHttpRequest("POST", "http://localhost:8586/tables/foo/query?prefix=0", "application/json", `{"query":`+string(q)+`}`)
		assertResponse(t, resp, 200, `{"cancelled":{"count":1},"registered":{"count":1},"visited":{"count":2}}`+"\n", "POST /tables/:name/query failed.")
	})
}

// Ensure that we can use variables associated with factor properties.
func TestServerFactorVariableQuery(t *testing.T) {
	runTestServer(func(s *Server) {
		setupTestTable("foo")
		setupTestProperty("foo", "action", true, "factor")
		setupTestData(t, "foo", [][]string{
			[]string{"00", "1970-01-01T00:00:00Z", `{"data":{"action":"x"}}`},
			[]string{"00", "1970-01-01T00:00:02Z", `{"data":{"action":"y"}}`},
			[]string{"00", "1970-01-01T00:00:03Z", `{"data":{"action":"z"}}`},

			[]string{"01", "1970-01-01T00:00:00Z", `{"data":{"action":"y"}}`},
			[]string{"01", "1970-01-01T00:00:02Z", `{"data":{"action":"z"}}`},
		})

		// Run query.
		query := `
			DECLARE prev_action AS FACTOR(action)
			WHEN prev_action != "x" THEN
				SELECT count() AS count GROUP BY prev_action, action
			END
			SET prev_action = action
		`
		q, _ := json.Marshal(query)
		resp, _ := sendTestHttpRequest("POST", "http://localhost:8586/tables/foo/query?prefix=0", "application/json", `{"query":`+string(q)+`}`)
		assertResponse(t, resp, 200, `{"prev_action":{"":{"action":{"x":{"count":1},"y":{"count":1}}},"y":{"action":{"z":{"count":2}}}}}`+"\n", "POST /tables/:name/query failed.")
	})
}

// Ensure that we can use time loops.
func TestServerTimeLoopQuery(t *testing.T) {
	runTestServer(func(s *Server) {
		setupTestTable("foo")
		setupTestProperty("foo", "action", true, "factor")
		setupTestProperty("foo", "val", true, "integer")
		setupTestData(t, "foo", [][]string{
			[]string{"00", "1980-01-01T00:00:00Z", `{"data":{"action":"home","val":10}}`},
			[]string{"00", "1980-01-03T00:00:00Z", `{"data":{"action":"signup","val":20}}`},
			[]string{"00", "1980-01-03T23:59:59Z", `{"data":{"val":30}}`},
			[]string{"00", "1980-01-04T00:00:00Z", `{"data":{"action":"register","val":40}}`},
		})

		// Run query.
		query := `
			FOR i EVERY 1 DAY WITHIN 7 DAYS
				FOR EACH EVENT
					SELECT sum(val) GROUP BY i
				END
			END
		`
		q, _ := json.Marshal(query)
		// _codegen(t, "foo", `{"query":`+string(q)+`}`)
		resp, _ := sendTestHttpRequest("POST", "http://localhost:8586/tables/foo/query?prefix=0", "application/json", `{"query":`+string(q)+`}`)
		assertResponse(t, resp, 200, `{"i":{"1":{"sum_val":10},"3":{"sum_val":60},"4":{"sum_val":70}}}`+"\n", "POST /tables/:name/query failed.")
	})
}

// Ensure that we can query the server for a histogram of values.
// func TestServerHistogramQuery(t *testing.T) {
// 	runTestServer(func(s *Server) {
// 		var id = "5"
// 		setupTestTable("foo")
// 		setupTestProperty("foo", "val", true, "integer")
// 		setupTestData(t, "foo", [][]string{
// 			[]string{"00", "2012-01-01T00:00:00Z", `{"data":{"val":3}}`}, // Different servlet.

// 			[]string{id, "2012-01-01T00:00:00Z", `{"data":{"val":1}}`},
// 			[]string{id, "2012-01-01T00:00:01Z", `{"data":{"val":2}}`},
// 			[]string{id, "2012-01-01T00:00:02Z", `{"data":{"val":0}}`},
// 			[]string{id, "2012-01-01T00:00:03Z", `{"data":{"val":3}}`},
// 			[]string{id, "2012-01-01T00:00:04Z", `{"data":{"val":4}}`},
// 			[]string{id, "2012-01-01T00:00:05Z", `{"data":{"val":4}}`},

// 			[]string{"02", "2012-01-01T00:00:00Z", `{"data":{"val":-1}}`},  // Out of range
// 			[]string{"02", "2012-01-01T00:00:01Z", `{"data":{"val":100}}`}, // Out of range
// 		})

// 		// Run query.
// 		query := `{"query":"SELECT histogram(val) AS hist"}`
// 		resp, _ := sendTestHttpRequest("POST", "http://localhost:8586/tables/foo/query?prefix=0", "application/json", query)
// 		assertResponse(t, resp, 200, `{"hist":{"__histogram__":true,"bins":{"0":3,"1":1,"2":5},"count":3,"max":4,"min":0,"width":1.3333333333333333}}`+"\n", "POST /tables/:name/query failed.")
// 	})
// }

// Ensure that we can can filter by prefix.
func TestServerPrefixQuery(t *testing.T) {
	runTestServer(func(s *Server) {
		setupTestTable("foo")
		setupTestProperty("foo", "price", true, "integer")
		setupTestData(t, "foo", [][]string{
			[]string{"0010a", "2012-01-01T00:00:00Z", `{"data":{"price":100}}`},
			[]string{"0010b", "2012-01-01T00:00:00Z", `{"data":{"price":200}}`},
			[]string{"0010b", "2012-01-01T00:00:01Z", `{}`},
			[]string{"0020a", "2012-01-01T00:00:00Z", `{"data":{"price":30}}`},
			[]string{"0030a", "2012-01-01T00:00:00Z", `{"data":{"price":40}}`},
		})

		// Run query.
		query := `{"query":{
			"prefix":"001",
			"statements":"SELECT sum(price) AS totalPrice"
		}}`
		resp, _ := sendTestHttpRequest("POST", "http://localhost:8586/tables/foo/query", "application/json", query)
		assertResponse(t, resp, 200, `{"totalPrice":300}`+"\n", "POST /tables/:name/query failed.")
	})
}

// Ensure that we can can query with raw text.
func TestServerRawQuery(t *testing.T) {
	runTestServer(func(s *Server) {
		setupTestTable("foo")
		setupTestProperty("foo", "price", true, "integer")
		setupTestData(t, "foo", [][]string{
			[]string{"x", "2012-01-01T00:00:00Z", `{"data":{"price":100}}`},
		})
		resp, _ := sendTestHttpRequest("POST", "http://localhost:8586/tables/foo/query?prefix=x", "application/json", "SELECT count() AS count")
		assertResponse(t, resp, 200, `{"count":1}`+"\n", "POST /tables/:name/query failed.")
	})
}

// Ensure that we can stop the query execution within an object.
func TestServerExitQuery(t *testing.T) {
	runTestServer(func(s *Server) {
		setupTestTable("foo")
		setupTestProperty("foo", "value", true, "integer")
		setupTestData(t, "foo", [][]string{
			[]string{"0001", "2012-01-01T00:00:00Z", `{"data":{"value":100}}`},
			[]string{"0001", "2012-01-01T00:01:00Z", `{"data":{"value":200}}`},
			[]string{"0001", "2012-01-01T00:02:00Z", `{"data":{"value":300}}`},
			[]string{"0002", "2012-01-01T00:00:00Z", `{"data":{"value":10}}`},
			[]string{"0002", "2012-01-01T00:01:00Z", `{"data":{"value":20}}`},
		})
		query := `
			SELECT sum(value) AS total
			WHEN value == 200 || value == 10 THEN EXIT END
		`
		q, _ := json.Marshal(query)
		resp, _ := sendTestHttpRequest("POST", "http://localhost:8586/tables/foo/query?prefix=0", "application/json", `{"query":`+string(q)+`}`)
		assertResponse(t, resp, 200, `{"total":310}`+"\n", "POST /tables/:name/query failed.")
	})
}

// Ensure that we can print debug statements from the query.
// NOTE: We're not testing the output -- simply that it doesn't blow up.
func TestServerDebugQuery(t *testing.T) {
	runTestServer(func(s *Server) {
		setupTestTable("foo")
		setupTestProperty("foo", "value", true, "integer")
		setupTestData(t, "foo", [][]string{
			[]string{"x", "2012-01-01T00:00:00Z", `{"data":{"value":100}}`},
			[]string{"x", "2012-01-01T00:01:00Z", `{"data":{"value":200}}`},
		})
		query := `
			DEBUG(timestamp)
		`
		q, _ := json.Marshal(query)
		resp, _ := sendTestHttpRequest("POST", "http://localhost:8586/tables/foo/query?prefix=x", "application/json", `{"query":`+string(q)+`}`)
		assertResponse(t, resp, 200, `{}`+"\n", "POST /tables/:name/query failed.")
	})
}

// Ensure that we can select non-aggregated field with a selection.
func TestServerExportQuery(t *testing.T) {
	runTestServer(func(s *Server) {
		setupTestTable("foo")
		setupTestProperty("foo", "val", true, "integer")
		setupTestProperty("foo", "my_string", true, "string")
		setupTestProperty("foo", "my_factor", true, "factor")
		setupTestProperty("foo", "my_integer", true, "integer")
		setupTestProperty("foo", "my_float", true, "float")
		setupTestProperty("foo", "my_boolean", true, "boolean")
		setupTestData(t, "foo", [][]string{
			[]string{"0001", "2012-01-01T00:00:00Z", `{"data":{"my_string":"xxx", "my_factor":"yyy", "my_integer":100, "my_float":10.2, "my_boolean":true}}`},
			[]string{"0001", "2012-01-01T00:00:01Z", `{"data":{"val":1000}}`},
		})
		query := `
			SELECT my_factor INTO "named"
			SELECT my_factor INTO "named"
			SELECT my_factor AS factor2 GROUP BY my_integer
			SELECT count() GROUP BY my_integer
			SELECT timestamp, my_string, my_factor, my_integer, my_float, my_boolean
		`
		//_codegen(t, "foo", query)
		q, _ := json.Marshal(query)
		resp, _ := sendTestHttpRequest("POST", "http://localhost:8586/tables/foo/query?prefix=0", "application/json", `{"query":`+string(q)+`}`)
		assertResponse(t, resp, 200, `{"_":[{"my_boolean":true,"my_factor":"yyy","my_float":10.2,"my_integer":100,"my_string":"xxx","timestamp":1325376000},{"my_boolean":false,"my_factor":"","my_float":0,"my_integer":0,"my_string":"","timestamp":1325376001}],"my_integer":{"0":{"_":[{"factor2":""}],"count":1},"100":{"_":[{"factor2":"yyy"}],"count":1}},"named":{"_":[{"my_factor":"yyy"},{"my_factor":"yyy"},{"my_factor":""},{"my_factor":""}]}}`+"\n", "POST /tables/:name/query failed.")
	})
}

// Ensure that we can select into the same field but dedupe merges.
func TestServerDuplicateSelectionQuery(t *testing.T) {
	runTestServer(func(s *Server) {
		setupTestTable("foo")
		setupTestProperty("foo", "value", true, "integer")
		setupTestData(t, "foo", [][]string{[]string{"x", "2012-01-01T00:00:00Z", `{"data":{"value":100}}`}})
		query := `
			SELECT count() AS count GROUP BY value
			SELECT count() AS count GROUP BY value
			SELECT count() AS count GROUP BY value INTO "test"
		`
		q, _ := json.Marshal(query)
		resp, _ := sendTestHttpRequest("POST", "http://localhost:8586/tables/foo/query?prefix=x", "application/json", `{"query":`+string(q)+`}`)
		assertResponse(t, resp, 200, `{"test":{"value":{"100":{"count":1}}},"value":{"100":{"count":2}}}`+"\n", "POST /tables/:name/query failed.")
	})
}

// Ensure that we can run basic stats.
func TestServerStatsQuery(t *testing.T) {
	runTestServer(func(s *Server) {
		setupTestTable("foo")
		setupTestProperty("foo", "price", true, "integer")
		setupTestData(t, "foo", [][]string{
			[]string{"0010a", "2012-01-01T00:00:00Z", `{"data":{"price":100}}`},
			[]string{"0010b", "2012-01-01T00:00:00Z", `{"data":{"price":200}}`},
			[]string{"0010b", "2012-01-01T00:00:01Z", `{"data":{"price":0}}`},
			[]string{"0020a", "2012-01-01T00:00:00Z", `{"data":{"price":30}}`},
			[]string{"0030a", "2012-01-01T00:00:00Z", `{"data":{"price":40}}`},
		})

		resp, _ := sendTestHttpRequest("GET", "http://localhost:8586/tables/foo/stats?prefix=001", "application/json", "")
		assertResponse(t, resp, 200, `{"count":3}`+"\n", "POST /tables/:name/query failed.")
	})
}
