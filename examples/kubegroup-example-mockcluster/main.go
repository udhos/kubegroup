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

	engineBogus bool
}

func main() {

	mux := http.NewServeMux()

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	log.Printf("ADDR='%s' using %s", os.Getenv("ADDR"), addr)

	app := &application{
		listenAddr:          addr,
		groupCachePort:      ":5000",
		groupCacheSizeBytes: 1_000_000,        // limit cache at 1 MB
		groupCacheExpire:    60 * time.Second, // cache TTL at 60s
	}

	flag.BoolVar(&app.engineBogus, "engineBogus", false, "enable bogus kube engine (for testing)")

	flag.Parse()

	app.serverMain = &http.Server{Addr: app.listenAddr, Handler: mux}

	startGroupcache(app)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { routeHandler(w, r, app) })

	go func() {
		//
		// start main http server
		//
		log.Printf("main server: listening on %s", app.listenAddr)
		err := app.serverMain.ListenAndServe()
		log.Printf("main server: exited: %v", err)
	}()

	shutdown(app)
}

func routeHandler(w http.ResponseWriter, r *http.Request, app *application) {

	filePath := r.URL.Path

	filePath = strings.TrimPrefix(filePath, "/")

	var data []byte
	errGet := app.cache.Get(r.Context(), filePath, groupcache.AllocatingByteSliceSink(&data))
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

	log.Printf("exiting")
}

func httpShutdown(server *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("http server shutdown error: %v", err)
	}
}
