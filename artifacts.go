// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/blang/semver"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
)

type artifactsHandler struct {
	cacheTime        time.Duration
	packagesBasePath string
}

var _ http.Handler = new(artifactsHandler)

var errArtifactNotFound = errors.New("artifact not found")

func newArtifactsHandler(packagesBasePath string, cacheTime time.Duration) *artifactsHandler {
	return &artifactsHandler{
		cacheTime:        cacheTime,
		packagesBasePath: packagesBasePath,
	}
}

func (h *artifactsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	packageName := vars["packageName"]
	packageVersion := vars["packageVersion"]

	_, err := semver.Parse(packageVersion)
	if err != nil {
		badRequest(w, "invalid package version")
		return
	}

	packagePath := filepath.Join(h.packagesBasePath, packageName, packageVersion)
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

	gzipWriter := gzip.NewWriter(w)
	tarWriter := tar.NewWriter(gzipWriter)
	defer func() {
		err := tarWriter.Close()
		if err != nil {
			log.Printf("Error occurred while closing tar writer: %v", err)
		}

		err = gzipWriter.Close()
		if err != nil {
			log.Printf("Error occurred while closing gzip writer: %v", err)
		}
	}()

	w.Header().Set("Content-Type", "application/gzip")
	cacheHeaders(w, h.cacheTime)

	err = filepath.Walk(packagePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relativePath, err := filepath.Rel(packagePath, path)
		if err != nil {
			return errors.Wrapf(err, "finding relative path failed (packagePath: %s, path: %s)", packagePath, path)
		}

		if relativePath == "." {
			return nil
		}

		header, err := buildArchiveHeader(info, relativePath)
		if err != nil {
			return errors.Wrapf(err, "building archive header failed (path: %s)", relativePath)
		}

		err = tarWriter.WriteHeader(header)
		if err != nil {
			return errors.Wrapf(err, "writing header failed (path: %s)", relativePath)
		}

		if !info.IsDir() {
			err = writeFileContentToArchive(path, tarWriter)
			if err != nil {
				return errors.Wrapf(err, "archiving file content failed (path: %s)", path)
			}
		}
		return nil
	})
	if err != nil {
		log.Printf("walking package path '%s' failed: %v", packagePath, err)
		return
	}
}

func buildArchiveHeader(info os.FileInfo, relativePath string) (*tar.Header, error) {
	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return nil, errors.Wrapf(err, "reading file info header failed (info: %s)", info.Name())
	}

	header.Name = relativePath
	if info.IsDir() {
		header.Name = header.Name + "/"
	}
	return header, nil
}

func writeFileContentToArchive(path string, writer io.Writer) error {
	f, err := os.Open(path)
	if err != nil {
		return errors.Wrapf(err, "opening file failed (path: %s)", path)
	}
	defer func() {
		err := f.Close()
		if err != nil {
			log.Printf("Error occurred while closing file (path: %s): %v", path, err)
		}
	}()

	_, err = io.Copy(writer, f)
	if err != nil {
		return errors.Wrapf(err, "copying file content failed (path: %s)", path)
	}
	return nil
}
