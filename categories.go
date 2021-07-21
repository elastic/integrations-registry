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
	"time"

	"github.com/Masterminds/semver/v3"
	"go.elastic.co/apm"

	"github.com/elastic/package-registry/util"
)

type Category struct {
	Id    string `yaml:"id" json:"id"`
	Title string `yaml:"title" json:"title"`
	Count int    `yaml:"count" json:"count"`
}

// categoriesHandler is a dynamic handler as it will also allow filtering in the future.
func categoriesHandler(packagesBasePaths []string, cacheTime time.Duration) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		filter, err := newCategoriesFilterFromParams(r)
		if err != nil {
			badRequest(w, err.Error())
			return
		}

		packages, err := util.GetPackages(r.Context(), packagesBasePaths)
		if err != nil {
			notFoundError(w, err)
			return
		}

		packageList := filter.Filter(r.Context(), packages)
		categories := filter.FilterCategories(r.Context(), packageList)

		data, err := getCategoriesOutput(r.Context(), categories)
		if err != nil {
			notFoundError(w, err)
			return
		}

		cacheHeaders(w, cacheTime)
		jsonHeader(w)
		fmt.Fprint(w, string(data))
	}
}

type categoriesFilter struct {
	Experimental           bool
	KibanaVersion          *semver.Version
	IncludePolicyTemplates bool
}

func newCategoriesFilterFromParams(r *http.Request) (categoriesFilter, error) {
	var filter categoriesFilter

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

	if v := query.Get("experimental"); v != "" {
		filter.Experimental, err = strconv.ParseBool(v)
		if err != nil {
			return filter, fmt.Errorf("invalid 'experimental' query param: '%s'", v)
		}
	}

	if v := query.Get("include_policy_templates"); v != "" {
		filter.IncludePolicyTemplates, err = strconv.ParseBool(v)
		if err != nil {
			return filter, fmt.Errorf("invalid 'include_policy_templates' query param: '%s'", v)
		}
	}

	return filter, nil
}

func (filter categoriesFilter) Filter(ctx context.Context, packages util.Packages) map[string]util.Package {
	span, _ := apm.StartSpan(ctx, "FilterPackages", "custom")
	defer span.End()

	packageList := map[string]util.Package{}

	// Get unique list of newest packages
	for _, p := range packages {
		// Check if the package is compatible with Kibana version
		if filter.KibanaVersion != nil {
			if valid := p.HasKibanaVersion(filter.KibanaVersion); !valid {
				continue
			}
		}

		// Skip internal packages
		if p.Internal {
			continue
		}

		// Skip experimental packages if flag is not specified
		if p.Release == util.ReleaseExperimental && !filter.Experimental {
			continue
		}

		// Check if the version exists and if it should be added or not.
		// If the package in the list is newer or equal, do nothing.
		if pp, ok := packageList[p.Name]; ok && pp.IsNewerOrEqual(p) {
			continue
		}

		// Otherwise delete and later add the new one.
		packageList[p.Name] = p
	}

	return packageList
}

func (filter categoriesFilter) FilterCategories(ctx context.Context, packageList map[string]util.Package) map[string]*Category {
	span, _ := apm.StartSpan(ctx, "FilterCategories", "custom")
	defer span.End()

	categories := map[string]*Category{}

	for _, p := range packageList {
		for _, c := range p.Categories {
			if _, ok := categories[c]; !ok {
				categories[c] = &Category{
					Id:    c,
					Title: c,
					Count: 0,
				}
			}

			categories[c].Count = categories[c].Count + 1
		}

		if filter.IncludePolicyTemplates {
			for _, t := range p.PolicyTemplates {
				// Skip when policy template level `categories` is empty and there is only one policy template
				if t.Categories == nil && len(p.PolicyTemplates) == 1 {
					break
				}

				for _, c := range p.Categories {
					categories[c].Count = categories[c].Count + 1
				}

				// Add policy template level categories.
				for _, c := range t.Categories {
					if _, ok := categories[c]; !ok {
						categories[c] = &Category{
							Id:    c,
							Title: c,
							Count: 0,
						}
					}

					categories[c].Count = categories[c].Count + 1
				}
			}
		}
	}

	return categories
}

func getCategoriesOutput(ctx context.Context, categories map[string]*Category) ([]byte, error) {
	span, _ := apm.StartSpan(ctx, "GetCategoriesOutput", "custom")
	defer span.End()

	var keys []string
	for k := range categories {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var outputCategories []*Category
	for _, k := range keys {
		c := categories[k]
		if title, ok := util.CategoryTitles[c.Title]; ok {
			c.Title = title
		}
		outputCategories = append(outputCategories, c)
	}

	return json.MarshalIndent(outputCategories, "", "  ")
}
