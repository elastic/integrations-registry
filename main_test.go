// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	generateFlag  = flag.Bool("generate", false, "Write golden files")
	testCacheTime = 1 * time.Second
)

func TestEndpoints(t *testing.T) {
	packagesBasePaths := []string{"./testdata/package"}

	faviconHandleFunc, err := faviconHandler(testCacheTime)
	require.NoError(t, err)

	indexHandleFunc, err := indexHandler(testCacheTime)
	require.NoError(t, err)

	tests := []struct {
		endpoint string
		path     string
		file     string
		handler  func(w http.ResponseWriter, r *http.Request)
	}{
		{"/", "", "index.json", indexHandleFunc},
		{"/index.json", "", "index.json", indexHandleFunc},
		{"/search", "/search", "search.json", searchHandler(packagesBasePaths, testCacheTime)},
		{"/search?all=true", "/search", "search-all.json", searchHandler(packagesBasePaths, testCacheTime)},
		{"/categories", "/categories", "categories.json", categoriesHandler(packagesBasePaths, testCacheTime)},
		{"/categories?experimental=true", "/categories", "categories-experimental.json", categoriesHandler(packagesBasePaths, testCacheTime)},
		{"/categories?experimental=foo", "/categories", "categories-experimental-error.json", categoriesHandler(packagesBasePaths, testCacheTime)},
		{"/search?kibana.version=6.5.2", "/search", "search-kibana652.json", searchHandler(packagesBasePaths, testCacheTime)},
		{"/search?kibana.version=7.2.1", "/search", "search-kibana721.json", searchHandler(packagesBasePaths, testCacheTime)},
		{"/search?category=metrics", "/search", "search-category-metrics.json", searchHandler(packagesBasePaths, testCacheTime)},
		{"/search?category=logs", "/search", "search-category-logs.json", searchHandler(packagesBasePaths, testCacheTime)},
		{"/search?package=example", "/search", "search-package-example.json", searchHandler(packagesBasePaths, testCacheTime)},
		{"/search?package=example&all=true", "/search", "search-package-example-all.json", searchHandler(packagesBasePaths, testCacheTime)},
		{"/search?internal=true", "/search", "search-package-internal.json", searchHandler(packagesBasePaths, testCacheTime)},
		{"/search?internal=bar", "/search", "search-package-internal-error.json", searchHandler(packagesBasePaths, testCacheTime)},
		{"/search?experimental=true", "/search", "search-package-experimental.json", searchHandler(packagesBasePaths, testCacheTime)},
		{"/search?experimental=foo", "/search", "search-package-experimental-error.json", searchHandler(packagesBasePaths, testCacheTime)},
		{"/favicon.ico", "", "favicon.ico", faviconHandleFunc},
	}

	for _, test := range tests {
		t.Run(test.endpoint, func(t *testing.T) {
			runEndpoint(t, test.endpoint, test.path, test.file, test.handler)
		})
	}
}

func TestArtifacts(t *testing.T) {
	packagesBasePaths := []string{"./testdata/package"}

	artifactsHandler := artifactsHandler(packagesBasePaths, testCacheTime)

	tests := []struct {
		endpoint string
		path     string
		file     string
		handler  func(w http.ResponseWriter, r *http.Request)
	}{
		{"/epr/example/example-0.0.2.tar.gz", artifactsRouterPath, "example-0.0.2.tar.gz-preview.txt", artifactsHandler},
		{"/epr/example/example-999.0.2.tar.gz", artifactsRouterPath, "artifact-package-version-not-found.txt", artifactsHandler},
		{"/epr/example/missing-0.1.2.tar.gz", artifactsRouterPath, "artifact-package-not-found.txt", artifactsHandler},
		{"/epr/example/example-a.b.c.tar.gz", artifactsRouterPath, "artifact-package-invalid-version.txt", artifactsHandler},
	}

	for _, test := range tests {
		t.Run(test.endpoint, func(t *testing.T) {
			runEndpoint(t, test.endpoint, test.path, test.file, test.handler)
		})
	}
}

func TestPackageIndex(t *testing.T) {
	packagesBasePaths := []string{"./testdata/package"}

	packageIndexHandler := packageIndexHandler(packagesBasePaths, testCacheTime)

	tests := []struct {
		endpoint string
		path     string
		file     string
		handler  func(w http.ResponseWriter, r *http.Request)
	}{
		{"/package/example/1.0.0/", packageIndexRouterPath, "package.json", packageIndexHandler},
		{"/package/missing/1.0.0/", packageIndexRouterPath, "index-package-not-found.txt", packageIndexHandler},
		{"/package/example/999.0.0/", packageIndexRouterPath, "index-package-revision-not-found.txt", packageIndexHandler},
		{"/package/example/a.b.c/", packageIndexRouterPath, "index-package-invalid-version.txt", packageIndexHandler},
	}

	for _, test := range tests {
		t.Run(test.endpoint, func(t *testing.T) {
			runEndpoint(t, test.endpoint, test.path, test.file, test.handler)
		})
	}
}

// TestAllPackageIndex generates and compares all index.json files for the test packages
func TestAllPackageIndex(t *testing.T) {
	publicPath := "./testdata/public"
	packagesBasePath := []string{publicPath + "/package"}

	packageIndexHandler := packageIndexHandler(packagesBasePath, testCacheTime)

	// find all packages
	dirs, err := filepath.Glob(packagesBasePath[0] + "/*/*")
	assert.NoError(t, err)

	type Test struct {
		packageName    string
		packageVersion string
	}
	var tests []Test

	for _, path := range dirs {
		packageVersion := filepath.Base(path)
		packageName := filepath.Base(filepath.Dir(path))

		test := Test{packageName, packageVersion}
		tests = append(tests, test)
	}

	for _, test := range tests {
		t.Run(test.packageName+"/"+test.packageVersion, func(t *testing.T) {
			packageEndpoint := "/package/" + test.packageName + "/" + test.packageVersion + "/"
			fileName := filepath.Join("package", test.packageName, test.packageVersion, "index.json")
			runEndpoint(t, packageEndpoint, packageIndexRouterPath, fileName, packageIndexHandler)
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
	if path == "" {
		router.PathPrefix("/").HandlerFunc(handler)
	} else {
		router.HandleFunc(path, handler)
	}
	req.RequestURI = endpoint
	router.ServeHTTP(recorder, req)

	fullPath := "./docs/api/" + file
	err = os.MkdirAll(filepath.Dir(fullPath), 0755)
	assert.NoError(t, err)

	recorded := recorder.Body.Bytes()
	if strings.HasSuffix(file, "-preview.txt") {
		recorded = listArchivedFiles(t, recorded)
	}

	if *generateFlag {
		err = ioutil.WriteFile(fullPath, recorded, 0644)
		if err != nil {
			t.Fatal(err)
		}
	}

	data, err := ioutil.ReadFile(fullPath)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, bytes.TrimSpace(data), bytes.TrimSpace(recorded))

	// Skip cache check if 4xx error
	if recorder.Code >= 200 && recorder.Code < 300 {
		cacheTime := fmt.Sprintf("%.0f", testCacheTime.Seconds())
		assert.Equal(t, recorder.Header()["Cache-Control"], []string{"max-age=" + cacheTime, "public"})
	}
}

func listArchivedFiles(t *testing.T, body []byte) []byte {
	gzippedReader, err := gzip.NewReader(bytes.NewReader(body))
	require.NoError(t, err)

	tarReader := tar.NewReader(gzippedReader)

	var listing bytes.Buffer

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)

		listing.WriteString(fmt.Sprintf("%d %s\n", header.Size, header.Name))
	}
	return listing.Bytes()
}
