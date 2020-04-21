// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	yaml "gopkg.in/yaml.v2"

	"github.com/elastic/package-registry/util"
)

func compactDatasetVariables(datasets datasetContentArray) (datasetContentArray, map[string][]util.Variable, error) { // map[inputType][]util.Variable
	varsPerInputType := map[string][]util.Variable{}
	var compacted datasetContentArray

	for _, dataset := range datasets {
		if len(dataset.manifest.Streams) != 1 {
			return nil, nil, fmt.Errorf("only datasets with single streams are supported (datasetName: %s, beatType: %s)", dataset.name, dataset.beatType)
		}

		stream := dataset.manifest.Streams[0]
		var notCompactedVars []util.Variable
		for _, aVar := range stream.Vars {
			isAlreadyCompacted := isVariableAlreadyCompacted(varsPerInputType, aVar, stream.Input)
			if !isAlreadyCompacted {
				canBeCompacted, err := canVariableBeCompacted(datasets, varsPerInputType, aVar, stream.Input)
				if err != nil {
					return nil, nil, errors.Wrap(err, "checking compactibility failed")
				}
				if canBeCompacted {
					varsPerInputType[stream.Input] = append(varsPerInputType[stream.Input], aVar)
				} else {
					notCompactedVars = append(notCompactedVars, aVar)
				}
			}
		}
		stream.Vars = notCompactedVars
		dataset.manifest.Streams[0] = stream
		compacted = append(compacted, dataset)
	}
	return compacted, varsPerInputType, nil
}

func isVariableAlreadyCompacted(varsPerInputType map[string][]util.Variable, aVar util.Variable, inputType string) bool {
	if vars, ok := varsPerInputType[inputType]; ok {
		for _, v := range vars {
			if v.Name == aVar.Name {
				return true // variable already compacted
			}
		}
	}
	return false
}

func canVariableBeCompacted(datasets datasetContentArray, varsPerInputType map[string][]util.Variable, aVar util.Variable, inputType string) (bool, error) {
	for _, dataset := range datasets {
		for _, stream := range dataset.manifest.Streams {
			if stream.Input != inputType {
				continue // input is not related with this var
			}

			var varUsed bool
			for _, streamVar := range stream.Vars {
				if isNonCompactableVariable(aVar) {
					continue
				}

				equal, err := areVariablesEqual(streamVar, aVar)
				if err != nil {
					return false, errors.Wrap(err, "comparing variables failed")
				}
				if equal {
					varUsed = true
					break
				}
			}

			if !varUsed {
				return false, nil // variable not present in this dataset
			}
		}
	}
	return true, nil
}

func areVariablesEqual(first util.Variable, second util.Variable) (bool, error) {
	if first.Name != second.Name || first.Type != second.Type {
		return false, nil
	}

	firstValue, err := yaml.Marshal(first.Default)
	if err != nil {
		return false, errors.Wrap(err, "marshalling first value failed")
	}
	secondValue, err := yaml.Marshal(second.Default)
	if err != nil {
		return false, errors.Wrap(err, "marshalling second value failed")
	}

	firstValueStr := strings.TrimSpace(string(firstValue))
	secondValueStr := strings.TrimSpace(string(secondValue))
	return firstValueStr == secondValueStr, nil
}

func isNonCompactableVariable(aVar util.Variable) bool {
	return aVar.Name == "period" || aVar.Name == "paths"
}
