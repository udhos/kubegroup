// Package main implements the example.
package main

import (
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/modernprogram/groupcache/v2"
	"github.com/udhos/boilerplate/envconfig"
	"github.com/udhos/kubegroup/kubegroup"
)

type application struct {
	//listenAddr          string
	groupCachePort      string
	groupCacheSizeBytes int64
	groupCacheExpire    time.Duration

	//serverMain       *http.Server
	//serverGroupCache *http.Server
	cache *groupcache.Group
	group *kubegroup.Group

	//engineBogus bool

	once bool
}

func main() {

	//mux := http.NewServeMux()

	me := filepath.Base(os.Args[0])

	env := envconfig.NewSimple(me)

	app := &application{
		//listenAddr:          ":8080",
		groupCachePort:      ":5000",
		groupCacheSizeBytes: 1_000_000,        // limit cache at 1 MB
		groupCacheExpire:    60 * time.Second, // cache TTL at 60s
		once:                env.Bool("ONCE", false),
	}

	/*
		flag.BoolVar(&app.engineBogus, "engineBogus", false, "enable bogus kube engine (for testing)")

		flag.Parse()

		//app.serverMain = &http.Server{Addr: app.listenAddr, Handler: mux}

	*/

	if app.once {
		startGroupcache(app)
		return
	}

	const max = 50000

	for {
		for i := 0; i < max; i++ {
			startGroupcache(app)
			app.cache = nil   // release groupcache resources
			app.group.Close() // release kubegroup resources
		}
		log.Printf("testing leak: alloc/release %d times", max)
		time.Sleep(time.Second)
	}

	/*
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
	*/
}

/*
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
*/
