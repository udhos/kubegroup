// Package main implements the example.
package main

import (
	"log"
	"net/http"
	"time"
)

type application struct {
	groupCachePort      string
	groupCacheSizeBytes int64
	groupCacheExpire    time.Duration

	serverGroupCache *http.Server
}

func main() {

	app := &application{
		groupCachePort:      ":5000",
		groupCacheSizeBytes: 1_000_000,        // limit cache at 1 MB
		groupCacheExpire:    60 * time.Second, // cache TTL at 60s
	}

	pool := startGroupcache(app)

	max := 100_000

	for {
		for i := 0; i < max; i++ {
			startPeerWatcher(app, pool)
		}
		log.Printf("testing for leak, executed %d times", max)
		time.Sleep(1 * time.Second)
	}

}
