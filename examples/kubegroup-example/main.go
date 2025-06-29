// Package main implements the example.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/modernprogram/groupcache/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/udhos/kubegroup/kubegroup"
)

type application struct {
	listenAddr          string
	groupCachePort      string
	groupCacheSizeBytes int64
	groupCacheExpire    time.Duration

	serverMain       *http.Server
	serverGroupCache *http.Server
	cache            *groupcache.Group
	group            *kubegroup.Group

	registry *prometheus.Registry
}

func main() {

	var dogstatsd bool
	var prom bool
	var mockDogstatsd bool
	var emf bool
	var emfCw bool
	flag.BoolVar(&dogstatsd, "dogstatsd", true, "enable dogstatsd")
	flag.BoolVar(&prom, "prom", true, "enable prometheus")
	flag.BoolVar(&mockDogstatsd, "mockDogstatsd", true, "mock dogstatsd")
	flag.BoolVar(&emf, "emf", false, "enable aws cloudwatch emf metrics")
	flag.BoolVar(&emfCw, "emfCw", false, "send aws cloudwatch emf metrics directly to cloudwatch logs")
	flag.Parse()
	log.Printf("dogstatds=%t prom=%t mockDogstatsd=%t emf=%t emfCw=%t",
		dogstatsd, prom, mockDogstatsd, emf, emfCw)

	mux := http.NewServeMux()

	app := &application{
		listenAddr:          ":8080",
		groupCachePort:      ":5000",
		groupCacheSizeBytes: 1_000_000,        // limit cache at 1 MB
		groupCacheExpire:    60 * time.Second, // cache TTL at 60s
	}

	//
	// metrics
	//

	if prom {
		app.registry = prometheus.NewRegistry()

		app.registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
		app.registry.MustRegister(collectors.NewGoCollector())

		mux.Handle("/metrics", app.metricsHandler())
	}

	//
	// main server
	//

	startGroupcache(app, dogstatsd, mockDogstatsd, emf, emfCw)

	app.serverMain = &http.Server{Addr: app.listenAddr, Handler: mux}

	mux.HandleFunc("/", func(w http.ResponseWriter,
		r *http.Request) {
		routeHandler(w, r, app)
	})

	go func() {
		//
		// start main http server
		//
		log.Printf("main server: listening on %s", app.listenAddr)
		err := app.serverMain.ListenAndServe()
		log.Printf("main server: exited: %v", err)
	}()

	shutdown(app)

	log.Printf("exiting")
}

func (app *application) metricsHandler() http.Handler {
	registerer := app.registry
	gatherer := app.registry
	return promhttp.InstrumentMetricHandler(
		registerer, promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{}),
	)
}

func routeHandler(w http.ResponseWriter, r *http.Request, app *application) {

	filePath := r.URL.Path

	filePath = strings.TrimPrefix(filePath, "/")

	var data []byte
	errGet := app.cache.Get(r.Context(), filePath, groupcache.AllocatingByteSliceSink(&data), nil)
	if errGet != nil {
		log.Printf("routeHandler: %s %s: cache error: %v", r.Method, r.URL.Path, errGet)
		http.Error(w, errGet.Error(), 500)
		return
	}

	if _, errWrite := w.Write(data); errWrite != nil {
		log.Printf("routeHandler: %s %s: write error: %v", r.Method, r.URL.Path, errWrite)
	}
}

func shutdown(app *application) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	log.Printf("received signal '%v', initiating shutdown", sig)

	log.Printf("stopping kubegroup")

	app.group.Close() // release kubegroup resources

	time.Sleep(time.Second) // give kubegroup time to log debug messages about exiting

	log.Printf("stopping http servers")

	httpShutdown(app.serverMain)
	httpShutdown(app.serverGroupCache)
}

func httpShutdown(server *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("http server shutdown error: %v", err)
	}
}
