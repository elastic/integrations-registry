// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/blang/semver"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"

	"github.com/elastic/package-registry/archiver"
)

const artifactsRouterPath = "/epr/{packageName}/{packageName:[a-z_]+}-{packageVersion}.tar.gz"

var errArtifactNotFound = errors.New("artifact not found")

func artifactsHandler(packagesBasePath string, cacheTime time.Duration) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		packageName, ok := vars["packageName"]
		if !ok {
			badRequest(w, "missing package name")
			return
		}

		packageVersion, ok := vars["packageVersion"]
		if !ok {
			badRequest(w, "missing package version")
			return
		}

		_, err := semver.Parse(packageVersion)
		if err != nil {
			badRequest(w, "invalid package version")
			return
		}

		packagePath := filepath.Join(packagesBasePath, packageName, packageVersion)
		_, err = os.Stat(packagePath)
		if os.IsNotExist(err) {
			notFoundError(w, errArtifactNotFound)
			return
		}
		if err != nil {
			log.Printf("stat package path '%s' failed: %v", packagePath, err)

			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/gzip")
		cacheHeaders(w, cacheTime)

		err = archiver.ArchivePackage(w, archiver.PackageProperties{
			Name:    packageName,
			Version: packageVersion,
			Path:    packagePath,
		})
		if err != nil {
			log.Printf("archiving package path '%s' failed: %v", packagePath, err)
			return
		}
	}
}
