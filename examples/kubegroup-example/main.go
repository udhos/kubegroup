// Package main implements the example.
package main

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/mailgun/groupcache"
)

type application struct {
	listenAddr          string
	groupCachePort      string
	groupCacheSizeBytes int64
	groupCacheExpire    time.Duration

	serverMain       *http.Server
	serverGroupCache *http.Server
	cache            *groupcache.Group
}

func main() {

	mux := http.NewServeMux()

	app := &application{
		listenAddr:          ":8080",
		groupCachePort:      ":5000",
		groupCacheSizeBytes: 1_000_000,        // limit cache at 1 MB
		groupCacheExpire:    60 * time.Second, // cache TTL at 60s
	}

	app.serverMain = &http.Server{Addr: app.listenAddr, Handler: mux}

	startGroupcache(app)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { routeHandler(w, r, app) })

	log.Printf("main server: listening on %s", app.listenAddr)
	err := app.serverMain.ListenAndServe()
	log.Printf("main server: exited: %v", err)
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
