// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/pkg/errors"
	"go.elastic.co/apm"

	"github.com/elastic/package-registry/util"
)

func searchHandler(packagesBasePaths []string, cacheTime time.Duration) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		filter, err := newSearchFilterFromParams(r)
		if err != nil {
			badRequest(w, err.Error())
			return
		}

		packages, err := util.GetPackages(r.Context(), packagesBasePaths)
		if err != nil {
			notFoundError(w, errors.Wrapf(err, "fetching package failed"))
			return
		}

		packagesList := filter.Filter(r.Context(), packages)

		data, err := getPackageOutput(r.Context(), packagesList)
		if err != nil {
			notFoundError(w, err)
			return
		}

		cacheHeaders(w, cacheTime)
		jsonHeader(w)
		fmt.Fprint(w, string(data))
	}
}

type searchFilter struct {
	Category      string
	Package       string
	KibanaVersion *semver.Version
	AllVersions   bool
	Internal      bool
	Experimental  bool
}

func newSearchFilterFromParams(r *http.Request) (searchFilter, error) {
	var filter searchFilter

	query := r.URL.Query()
	if len(query) == 0 {
		return filter, nil
	}

	var err error
	if v := query.Get("kibana.version"); v != "" {
		filter.KibanaVersion, err = semver.NewVersion(v)
		if err != nil {
			return filter, fmt.Errorf("invalid Kibana version '%s': %w", v, err)
		}
	}

	if v := query.Get("category"); v != "" {
		filter.Category = v
	}

	if v := query.Get("package"); v != "" {
		filter.Package = v
	}

	if v := query.Get("all"); v != "" {
		// Default is false, also on error
		filter.AllVersions, err = strconv.ParseBool(v)
		if err != nil {
			return filter, fmt.Errorf("invalid 'all' query param: '%s'", v)
		}
	}

	if v := query.Get("internal"); v != "" {
		// In case of error, keep it false
		filter.Internal, err = strconv.ParseBool(v)
		if err != nil {
			return filter, fmt.Errorf("invalid 'internal' query param: '%s'", v)
		}
	}

	if v := query.Get("experimental"); v != "" {
		// In case of error, keep it false
		filter.Experimental, err = strconv.ParseBool(v)
		if err != nil {
			return filter, fmt.Errorf("invalid 'experimental' query param: '%s'", v)
		}
	}

	return filter, nil
}

func (filter searchFilter) Filter(ctx context.Context, packages util.Packages) map[string]map[string]util.Package {
	span, _ := apm.StartSpan(ctx, "FilterPackages", "custom")
	defer span.End()

	packagesList := map[string]map[string]util.Package{}

	// Checks that only the most recent version of an integration is added to the list
	for _, p := range packages {
		// Skip internal packages by default
		if p.Internal && !filter.Internal {
			continue
		}

		// Skip experimental packages if flag is not specified
		if p.Release == util.ReleaseExperimental && !filter.Experimental {
			continue
		}

		// Filter by category first as this could heavily reduce the number of packages
		// It must happen before the version filtering as there only the newest version
		// is exposed and there could be an older package with more versions.
		if filter.Category != "" && !p.HasCategory(filter.Category) {
			continue
		}

		if filter.KibanaVersion != nil {
			if valid := p.HasKibanaVersion(filter.KibanaVersion); !valid {
				continue
			}
		}

		// If package Query is set, all versions of this package are returned
		if filter.Package != "" && filter.Package != p.Name {
			continue
		}

		addPackage := true
		if !filter.AllVersions {
			// Check if the version exists and if it should be added or not.
			for name, versions := range packagesList {
				if name != p.Name {
					continue
				}
				for _, pp := range versions {

					// If the package in the list is newer or equal, do nothing.
					if pp.IsNewerOrEqual(p) {
						addPackage = false
						continue
					}

					// Otherwise delete and later add the new one.
					delete(packagesList[pp.Name], pp.Version)
				}
			}
		}

		if addPackage {
			if _, ok := packagesList[p.Name]; !ok {
				packagesList[p.Name] = map[string]util.Package{}
			}
			if _, ok := packagesList[p.Name][p.Version]; !ok {
				packagesList[p.Name][p.Version] = p
			}
		}
	}

	return packagesList
}

func getPackageOutput(ctx context.Context, packagesList map[string]map[string]util.Package) ([]byte, error) {
	span, _ := apm.StartSpan(ctx, "GetPackageOutput", "custom")
	defer span.End()

	separator := "@"
	// Packages need to be sorted to be always outputted in the same order
	var keys []string
	for key, k := range packagesList {
		for v := range k {
			keys = append(keys, key+separator+v)
		}
	}
	sort.Strings(keys)

	var output []util.BasePackage

	for _, k := range keys {
		parts := strings.Split(k, separator)
		m := packagesList[parts[0]][parts[1]]
		data := m.BasePackage
		output = append(output, data)
	}

	// Instead of return `null` in case of an empty array, return []
	if len(output) == 0 {
		return []byte("[]"), nil
	}

	return json.MarshalIndent(output, "", "  ")
}
