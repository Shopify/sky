package server

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/skydb/sky/db"
	"github.com/skydb/sky/query"
)

func (s *Server) addQueryHandlers() {
	s.ApiHandleFunc("/tables/{name}/query", func(w http.ResponseWriter, req *http.Request, params map[string]interface{}) (interface{}, error) {
		return s.queryHandler(w, req, params)
	}).Methods("POST")
	s.ApiHandleFunc("/tables/{name}/query/codegen", func(w http.ResponseWriter, req *http.Request, params map[string]interface{}) (interface{}, error) {
		return s.queryCodegenHandler(w, req, params)
	}).Methods("POST")
}

// POST /tables/:name/query
func (s *Server) queryHandler(w http.ResponseWriter, req *http.Request, params map[string]interface{}) (interface{}, error) {
	vars := mux.Vars(req)

	// Return an error if the table already exists.
	table, err := s.OpenTable(vars["name"])
	if err != nil {
		return nil, err
	}

	var results interface{}
	err = table.View(func(tx *db.Tx) error {
		q, err := s.parseQuery(tx, params)
		if err != nil {
			return err
		}

		prefix := req.FormValue("prefix")
		if prefix != "" {
			q.Prefix = prefix
		}

		if req.FormValue("minTimestamp") != "" {
			q.MinTimestamp, err = db.ParseTime(req.FormValue("minTimestamp"))
			if err != nil {
				return err
			}
		}

		if req.FormValue("maxTimestamp") != "" {
			q.MaxTimestamp, err = db.ParseTime(req.FormValue("maxTimestamp"))
			if err != nil {
				return err
			}
		}

		results, err = s.RunQuery(tx, q)
		return err
	})
	return results, err
}

// POST /tables/:name/query/codegen
func (s *Server) queryCodegenHandler(w http.ResponseWriter, req *http.Request, params map[string]interface{}) (interface{}, error) {
	vars := mux.Vars(req)

	// Return an error if the table already exists.
	table, err := s.OpenTable(vars["name"])
	if err != nil {
		return nil, err
	}

	var source interface{}
	err = table.View(func(tx *db.Tx) error {
		q, err := s.parseQuery(tx, params)
		if err != nil {
			return err
		}

		// Create an engine to retrieve full header.
		engine, err := query.NewExecutionEngine(q)
		if err != nil {
			return err
		}
		source = engine.FullSource()
		engine.Destroy()

		return nil
	})

	// Generate the query source code.
	return source, &TextPlainContentTypeError{}
}

func (s *Server) parseQuery(tx *db.Tx, params map[string]interface{}) (*query.Query, error) {
	var err error

	// Use raw post data as query if it's not JSON.
	raw := params["RAW_POST_DATA"]

	// DEPRECATED: Allow query to be passed in as root param.
	if raw == nil {
		raw = params["query"]
	}
	if raw == nil {
		raw = params
	}

	// Parse query if passed as a string. Otherwise deserialize JSON.
	var q *query.Query
	if str, ok := raw.(string); ok {
		if q, err = query.NewParser().ParseString(str); err != nil {
			return nil, err
		}
		if _, ok := params["prefix"]; ok {
			q.Prefix = params["prefix"].(string)
		}
	} else if obj, ok := raw.(map[string]interface{}); ok {
		q = query.NewQuery()
		if err = q.Deserialize(obj); err != nil {
			return nil, err
		}
	}

	q.Tx = tx

	return q, nil
}
