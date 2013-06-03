package skyd

import (
	"errors"
	"github.com/gorilla/mux"
	"net/http"
)

func (s *Server) addPropertyHandlers() {
	s.ApiHandleFunc("/tables/{name}/properties", nil, s.getPropertiesHandler).Methods("GET")
	s.ApiHandleFunc("/tables/{name}/properties", nil, s.createPropertyHandler).Methods("POST")

	s.ApiHandleFunc("/tables/{name}/properties/{propertyName}", nil, s.getPropertyHandler).Methods("GET")
	s.ApiHandleFunc("/tables/{name}/properties/{propertyName}", nil, s.updatePropertyHandler).Methods("PATCH")
	s.ApiHandleFunc("/tables/{name}/properties/{propertyName}", nil, s.deletePropertyHandler).Methods("DELETE")
}

// GET /tables/:name/properties
func (s *Server) getPropertiesHandler(w http.ResponseWriter, req *http.Request, params interface{}) (interface{}, error) {
	vars := mux.Vars(req)

	table, err := s.OpenTable(vars["name"])
	if err != nil {
		return nil, err
	}

	return table.GetProperties()
}

// POST /tables/:name/properties
func (s *Server) createPropertyHandler(w http.ResponseWriter, req *http.Request, params interface{}) (interface{}, error) {
	vars := mux.Vars(req)
	table, err := s.OpenTable(vars["name"])
	if err != nil {
		return nil, err
	}

	p := params.(map[string]interface{})
	name, _ := p["name"].(string)
	transient, _ := p["transient"].(bool)
	dataType, _ := p["dataType"].(string)
	return table.CreateProperty(name, transient, dataType)
}

// GET /tables/:name/properties/:propertyName
func (s *Server) getPropertyHandler(w http.ResponseWriter, req *http.Request, params interface{}) (interface{}, error) {
	vars := mux.Vars(req)
	table, err := s.OpenTable(vars["name"])
	if err != nil {
		return nil, err
	}

	return table.GetPropertyByName(vars["propertyName"])
}

// PATCH /tables/:name/properties/:propertyName
func (s *Server) updatePropertyHandler(w http.ResponseWriter, req *http.Request, params interface{}) (interface{}, error) {
	vars := mux.Vars(req)
	table, err := s.OpenTable(vars["name"])
	if err != nil {
		return nil, err
	}

	// Retrieve property.
	property, err := table.GetPropertyByName(vars["propertyName"])
	if err != nil {
		return nil, err
	}
	if property == nil {
		return nil, errors.New("Property does not exist.")
	}

	// Update property and save property file.
	p := params.(map[string]interface{})
	name, _ := p["name"].(string)
	property.Name = name
	err = table.SavePropertyFile()
	if err != nil {
		return nil, err
	}

	return property, nil
}

// DELETE /tables/:name/properties/:propertyName
func (s *Server) deletePropertyHandler(w http.ResponseWriter, req *http.Request, params interface{}) (interface{}, error) {
	vars := mux.Vars(req)
	table, err := s.OpenTable(vars["name"])
	if err != nil {
		return nil, err
	}
	// Retrieve property.
	property, err := table.GetPropertyByName(vars["propertyName"])
	if err != nil {
		return nil, err
	}
	if property == nil {
		return nil, errors.New("Property does not exist.")
	}

	// Delete property and save property file.
	table.DeleteProperty(property)
	err = table.SavePropertyFile()
	if err != nil {
		return nil, err
	}

	return nil, nil
}
