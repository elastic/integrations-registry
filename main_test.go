// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"flag"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"

	"github.com/magiconair/properties/assert"
)

var (
	generateFlag = flag.Bool("generate", false, "Write golden files")
)

func TestEndpoints(t *testing.T) {

	packagesPath = "./testdata/packages"

	tests := []struct {
		endpoint string
		path     string
		file     string
		handler  func(w http.ResponseWriter, r *http.Request)
	}{
		{"/", "/", "info.json", infoHandler()},
		{"/list", "/list", "list.json", listHandler()},
		{"/package/example-1.0.0", "/package/{name}", "package.json", packageHandler()},
	}

	for _, test := range tests {
		t.Run(test.endpoint, func(t *testing.T) {
			runEndpoint(t, test.endpoint, test.path, test.file, test.handler)
		})
	}
}

func runEndpoint(t *testing.T, endpoint, path, file string, handler func(w http.ResponseWriter, r *http.Request)) {
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()

	router := mux.NewRouter()
	router.HandleFunc(path, handler)
	router.ServeHTTP(recorder, req)

	fullPath := "./docs/api/" + file

	if *generateFlag {
		err = ioutil.WriteFile(fullPath, recorder.Body.Bytes(), 0644)
		if err != nil {
			t.Fatal(err)
		}
	}

	data, err := ioutil.ReadFile(fullPath)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, string(data), recorder.Body.String())
}
