// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"

	"go.elastic.co/apm/module/apmgorilla"

	ucfgYAML "github.com/elastic/go-ucfg/yaml"

	"github.com/elastic/package-registry/util"
)

const (
	serviceName = "package-registry"
	version     = "0.20.0"
)

var (
	address    string
	dryRun     bool
	configPath = "config.yml"

	defaultConfig = Config{
		CacheTimeIndex:      10 * time.Second,
		CacheTimeSearch:     10 * time.Minute,
		CacheTimeCategories: 10 * time.Minute,
		CacheTimeCatchAll:   10 * time.Minute,
	}
)

func init() {
	flag.StringVar(&address, "address", "localhost:8080", "Address of the package-registry service.")
	// This flag is experimental and might be removed in the future or renamed
	flag.BoolVar(&dryRun, "dry-run", false, "Runs a dry-run of the registry without starting the web service (experimental)")
	flag.BoolVar(&util.PackageValidationDisabled, "disable-package-validation", false, "Disable package content validation")
}

type Config struct {
	PackagePaths        []string      `config:"package_paths"`
	CacheTimeIndex      time.Duration `config:"cache_time.index"`
	CacheTimeSearch     time.Duration `config:"cache_time.search"`
	CacheTimeCategories time.Duration `config:"cache_time.categories"`
	CacheTimeCatchAll   time.Duration `config:"cache_time.catch_all"`
}

func main() {
	flag.Parse()
	log.Println("Package registry started.")
	defer log.Println("Package registry stopped.")

	config := mustLoadConfig()
	packagesBasePaths := getPackagesBasePaths(config)
	ensurePackagesAvailable(packagesBasePaths)

	// If -dry-run=true is set, service stops here after validation
	if dryRun {
		return
	}

	router := mustLoadRouter(config, packagesBasePaths)
	server := &http.Server{Addr: address, Handler: router}

	go func() {
		err := server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("Error occurred while serving: %s", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx := context.TODO()
	if err := server.Shutdown(ctx); err != nil {
		log.Fatal(err)
	}
}

func mustLoadConfig() *Config {
	config, err := getConfig()
	if err != nil {
		log.Fatal(err)
	}
	printConfig(config)
	return config
}

func getConfig() (*Config, error) {
	cfg, err := ucfgYAML.NewConfigWithFile(configPath)
	if os.IsNotExist(err) {
		log.Printf(`Using default configuration options as "%s" is not available.`, configPath)
		return &defaultConfig, nil
	}
	if err != nil {
		return nil, errors.Wrapf(err, "reading config failed (path: %s)", configPath)
	}

	config := defaultConfig
	err = cfg.Unpack(&config)
	if err != nil {
		return nil, errors.Wrapf(err, "unpacking config failed (path: %s)", configPath)
	}
	return &config, nil
}

func getPackagesBasePaths(config *Config) []string {
	var paths []string
	paths = append(paths, config.PackagePaths...)
	return paths
}

func printConfig(config *Config) {
	log.Printf("Packages paths: %s\n", strings.Join(config.PackagePaths, ", "))
	log.Println("Cache time for /search: ", config.CacheTimeSearch)
	log.Println("Cache time for /categories: ", config.CacheTimeCategories)
	log.Println("Cache time for all others: ", config.CacheTimeCatchAll)
}

func ensurePackagesAvailable(packagesBasePaths []string) {
	packages, err := util.GetPackages(context.TODO(), packagesBasePaths)
	if err != nil {
		log.Fatal(err)
	}

	if len(packages) == 0 {
		log.Fatal("No packages available")
	}

	log.Printf("%v package manifests loaded.\n", len(packages))
}

func mustLoadRouter(config *Config, packagesBasePaths []string) *mux.Router {
	router, err := getRouter(config, packagesBasePaths)
	if err != nil {
		log.Fatal(err)
	}
	return router
}

func getRouter(config *Config, packagesBasePaths []string) (*mux.Router, error) {
	artifactsHandler := artifactsHandler(packagesBasePaths, config.CacheTimeCatchAll)
	faviconHandleFunc, err := faviconHandler(config.CacheTimeCatchAll)
	if err != nil {
		return nil, err
	}
	indexHandlerFunc, err := indexHandler(config.CacheTimeIndex)
	if err != nil {
		return nil, err
	}

	packageIndexHandler := packageIndexHandler(packagesBasePaths, config.CacheTimeCatchAll)

	router := mux.NewRouter().StrictSlash(true)
	apmgorilla.Instrument(router)

	router.HandleFunc("/", indexHandlerFunc)
	router.HandleFunc("/index.json", indexHandlerFunc)
	router.HandleFunc("/search", searchHandler(packagesBasePaths, config.CacheTimeSearch))
	router.HandleFunc("/categories", categoriesHandler(packagesBasePaths, config.CacheTimeCategories))
	router.HandleFunc("/health", healthHandler)
	router.HandleFunc("/favicon.ico", faviconHandleFunc)
	router.HandleFunc(artifactsRouterPath, artifactsHandler)
	router.HandleFunc(packageIndexRouterPath, packageIndexHandler)
	router.PathPrefix("/package").HandlerFunc(staticHandler(packagesBasePaths, "/package", config.CacheTimeCatchAll))
	router.Use(loggingMiddleware)
	router.NotFoundHandler = http.Handler(notFoundHandler(fmt.Errorf("404 page not found")))
	return router, nil
}

// healthHandler is used for Docker/K8s deployments. It returns 200 if the service is live
// In addition ?ready=true can be used for a ready request. Currently both are identical.
func healthHandler(w http.ResponseWriter, r *http.Request) {}

// logging middle to log all requests
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logRequest(r)
		next.ServeHTTP(w, r)
	})
}

// logRequest converts a request object into a proper logging event
func logRequest(r *http.Request) {
	// Do not log requests to the health endpoint
	if r.RequestURI == "/health" {
		return
	}
	log.Println(fmt.Sprintf("source.ip: %s, url.original: %s", r.RemoteAddr, r.RequestURI))
}
