// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"flag"
	"log"
	"net/url"
	"os"

	"github.com/pkg/errors"
)

type importerOptions struct {
	// Beats repository directory
	beatsDir string

	// Kibana host and port
	kibanaHostPort string
	// Kibana repository directory
	kibanaDir string

	// Elastic UI Framework directory
	euiDir string

	// Target public directory where the generated packages should end up in
	outputDir string
}

func (o *importerOptions) validate() error {
	_, err := os.Stat(o.beatsDir)
	if err != nil {
		return errors.Wrapf(err, "stat file failed (path: %s)", o.beatsDir)
	}

	_, err = url.Parse(o.kibanaHostPort)
	if err != nil {
		return errors.Wrapf(err, "parsing Kibana's host:port failed (hostPort: %s)", o.kibanaHostPort)
	}

	_, err = os.Stat(o.kibanaDir)
	if err != nil {
		return errors.Wrapf(err, "stat file failed (path: %s)", o.kibanaDir)
	}

	_, err = os.Stat(o.euiDir)
	if err != nil {
		return errors.Wrapf(err, "stat file failed (path: %s)", o.euiDir)
	}

	_, err = os.Stat(o.outputDir)
	if err != nil {
		return errors.Wrapf(err, "stat file failed (path: %s)", o.outputDir)
	}
	return nil
}

func main() {
	var options importerOptions

	flag.StringVar(&options.beatsDir, "beatsDir", "../beats", "Path to the beats repository")
	flag.StringVar(&options.kibanaDir, "kibanaDir", "../kibana", "Path to the kibana repository")
	flag.StringVar(&options.kibanaHostPort, "kibanaHostPort", "http://localhost:5601", "Kibana host and port")
	flag.StringVar(&options.euiDir, "euiDir", "../eui", "Path to the Elastic UI framework repository")
	flag.StringVar(&options.outputDir, "outputDir", "dev/packages/beats", "Path to the output directory")
	flag.Parse()

	err := options.validate()
	if err != nil {
		log.Fatal(err)
	}

	if err := build(options); err != nil {
		log.Fatal(err)
	}
}

// build method visits all beats in beatsDir to collect configuration data for modules.
// The package-registry groups integrations per target product not per module type. It's opposite to the beats project,
// where logs and metrics are distributed with different beats (oriented either on logs or metrics - metricbeat,
// filebeat, etc.).
func build(options importerOptions) error {
	iconRepository, err := newIconRepository(options.euiDir, options.kibanaDir)
	if err != nil {
		return errors.Wrap(err, "creating icon repository failed")
	}
	kibanaMigrator := newKibanaMigrator(options.kibanaHostPort)
	repository := newPackageRepository(iconRepository, kibanaMigrator)

	for _, beatName := range logSources {
		err := repository.createPackagesFromSource(options.beatsDir, beatName, "logs")
		if err != nil {
			return errors.Wrap(err, "creating from logs source failed")
		}
	}

	for _, beatName := range metricSources {
		err := repository.createPackagesFromSource(options.beatsDir, beatName, "metrics")
		if err != nil {
			return errors.Wrap(err, "creating from metrics source failed")
		}
	}
	return repository.save(options.outputDir)
}
