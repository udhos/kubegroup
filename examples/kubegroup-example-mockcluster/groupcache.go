package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/modernprogram/groupcache/v2"
	"github.com/udhos/kubegroup/kubegroup"
)

func startGroupcache(app *application) {

	myAddr := os.Getenv("MY_ADDR")
	if myAddr == "" {
		log.Fatalf("missing env var MY_ADDR")
	}

	//
	// create groupcache pool
	//

	myURL := "http://" + myAddr + app.groupCachePort

	log.Printf("groupcache my URL: %s", myURL)

	pool := groupcache.NewHTTPPoolOpts(myURL, &groupcache.HTTPPoolOptions{})

	//
	// start groupcache server
	//

	listenAddr := myAddr + app.groupCachePort

	app.serverGroupCache = &http.Server{Addr: listenAddr, Handler: pool}

	go func() {
		log.Printf("groupcache server: listening on %s", listenAddr)
		err := app.serverGroupCache.ListenAndServe()
		log.Printf("groupcache server: exited: %v", err)
	}()

	//
	// start watcher for addresses of peers
	//

	addresses := strings.Fields(os.Getenv("ADDRESSES"))
	if len(addresses) < 2 {
		log.Fatalf("ADDRESSES requires 2 addresses minimum: '%v'", addresses)
	}

	options := kubegroup.Options{
		Pool:           pool,
		GroupCachePort: app.groupCachePort,
		//PodLabelKey:    "app",         // default is "app"
		//PodLabelValue:  "my-app-name", // default is current PODs label value for label key
		Debug: true,
		Engine: kubegroup.NewKubeMockCluster(
			myAddr,
			addresses,
		),
	}

	if app.engineBogus {
		options.Engine = kubegroup.NewKubeBogus()
	}

	group, errGroup := kubegroup.UpdatePeers(options)
	if errGroup != nil {
		log.Fatalf("kubegroup: %v", errGroup)
	}

	app.group = group

	//
	// create cache
	//

	getter := groupcache.GetterFunc(
		func(_ /*ctx*/ context.Context, filePath string, dest groupcache.Sink) error {

			log.Printf("cache miss, loading file: %s (ttl:%v)",
				filePath, app.groupCacheExpire)

			data, errRead := os.ReadFile(filePath)
			if errRead != nil {
				return errRead
			}

			var expire time.Time // zero value for expire means no expiration
			if app.groupCacheExpire != 0 {
				expire = time.Now().Add(app.groupCacheExpire)
			}

			dest.SetBytes(data, expire)

			return nil
		},
	)

	// https://talks.golang.org/2013/oscon-dl.slide#46
	//
	// 64 MB max per-node memory usage
	app.cache = groupcache.NewGroup("files", app.groupCacheSizeBytes, getter)
}
