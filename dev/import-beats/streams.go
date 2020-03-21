// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"fmt"
	"io/ioutil"
	"path"
	"strings"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"

	"github.com/elastic/package-registry/util"
)

type manifestWithVars struct {
	Vars []map[string]interface{} `yaml:"var"`
}

type varWithDefault struct {
	Default interface{} `yaml:"default"`
}

func createStreams(modulePath, moduleName, datasetName, beatType string) ([]util.Stream, error) {
	switch beatType {
	case "logs":
		return createLogStreams(modulePath, moduleName, datasetName, beatType)
	case "metrics":
		return createMetricStreams()
	}
	return nil, fmt.Errorf("invalid beat type: %s", beatType)
}

func createLogStreams(modulePath, moduleName, datasetName, beatType string) ([]util.Stream, error) {
	datasetPath := path.Join(modulePath, datasetName, "manifest.yml")
	manifestPath, err := ioutil.ReadFile(datasetPath)
	if err != nil {
		return nil, errors.Wrapf(err, "reading manifest file failed (path: %s)", datasetPath)
	}

	var mwv manifestWithVars
	err = yaml.Unmarshal(manifestPath, &mwv)
	if err != nil {
		return nil, errors.Wrapf(err, "unmarshalling manifest file failed (path: %s)", datasetPath)
	}

	return []util.Stream{
		{
			Input:       "logs",
			Title:       fmt.Sprintf("%s %s %s", strings.Title(moduleName), strings.Title(datasetName), beatType),
			Description: fmt.Sprintf("Collect %s %s logs", strings.Title(moduleName), strings.Title(datasetName)),
			Vars:        wrapVariablesWithDefault(mwv).Vars,
		},
	}, nil
}

func wrapVariablesWithDefault(mwvs manifestWithVars) manifestWithVars {
	var withDefaults manifestWithVars
	for _, aVar := range mwvs.Vars {
		aVarWithDefaults := map[string]interface{}{}
		for k, v := range aVar {
			if strings.HasPrefix(k, "os.") {
				aVarWithDefaults[k] = varWithDefault{
					Default: v,
				}
			} else {
				aVarWithDefaults[k] = v
			}
		}
		withDefaults.Vars = append(withDefaults.Vars, aVarWithDefaults)
	}
	return withDefaults
}

func createMetricStreams() ([]util.Stream, error) {
	return []util.Stream{
		{
			Input: "TODO",
		},
	}, nil // TODO
}
