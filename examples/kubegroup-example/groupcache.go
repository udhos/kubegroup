package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/modernprogram/groupcache/v2"
	"github.com/udhos/cloudwatchlog/cwlog"
	"github.com/udhos/dogstatsdclient/dogstatsdclient"
	"github.com/udhos/kube/kubeclient"
	"github.com/udhos/kubegroup/kubegroup"
)

func startGroupcache(app *application, dogstatsd, mockDogstatsd, emfEnable, emfSend bool) {

	//
	// create groupcache pool
	//

	myURL, errURL := kubegroup.FindMyURL(app.groupCachePort)
	if errURL != nil {
		log.Fatalf("my URL: %v", errURL)
	}
	log.Printf("groupcache my URL: %s", myURL)

	workspace := groupcache.NewWorkspace()

	pool := groupcache.NewHTTPPoolOptsWithWorkspace(workspace, myURL,
		&groupcache.HTTPPoolOptions{})

	//
	// start groupcache server
	//

	app.serverGroupCache = &http.Server{Addr: app.groupCachePort, Handler: pool}

	go func() {
		log.Printf("groupcache server: listening on %s", app.groupCachePort)
		err := app.serverGroupCache.ListenAndServe()
		log.Printf("groupcache server: exited: %v", err)
	}()

	//
	// start watcher for addresses of peers
	//

	const debug = true

	clientsetOpt := kubeclient.Options{DebugLog: debug}
	clientset, errClientset := kubeclient.New(clientsetOpt)
	if errClientset != nil {
		log.Fatalf("startGroupcache: kubeclient: %v", errClientset)
	}

	var dogstatsdClient kubegroup.DogstatsdClient
	if dogstatsd {
		if mockDogstatsd {
			dogstatsdClient = &kubegroup.DogstatsdClientMock{}
		} else {
			c, errClient := dogstatsdclient.New(dogstatsdclient.Options{
				Namespace: "kubegroup",
				Debug:     debug,
			})
			if errClient != nil {
				log.Fatalf("dogstatsd client: %v", errClient)
			}
			dogstatsdClient = c
		}
	}

	options := kubegroup.Options{
		Client:                clientset,
		Pool:                  pool,
		LabelSelector:         "app=miniapi",
		GroupCachePort:        app.groupCachePort,
		Debug:                 debug,
		DogstatsdClient:       dogstatsdClient,
		ForceNamespaceDefault: true,
	}

	if app.registry != nil {
		options.MetricsRegisterer = app.registry
	}

	if emfEnable {
		//
		// enable AWS CloudWatch EMF metrics
		//
		appName := "kubegroup-example"
		options.EmfEnable = true
		options.EmfDimensions = map[string]string{"app": appName}
		if emfSend {
			//
			// send AWS CloudWatch EMF metrics directly to CloudWatch logs
			//
			awsConfig, errConfig := config.LoadDefaultConfig(context.TODO())
			if errConfig != nil {
				log.Fatalf("emf aws config error: %v", errConfig)
			}
			client, err := cwlog.New(cwlog.Options{
				AwsConfig:       awsConfig,
				LogGroup:        "/kubegroup/" + appName,
				RetentionInDays: 7,
			})
			if err != nil {
				log.Fatalf("emf cloudwatch logs client error: %v", err)
			}
			options.EmfCloudWatchLogsClient = client
		}
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
		func(_ /*ctx*/ context.Context, filePath string, dest groupcache.Sink, _ *groupcache.Info) error {

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

			return dest.SetBytes(data, expire)
		},
	)

	const purgeExpired = true

	groupcacheOptions := groupcache.Options{
		Workspace:       workspace,
		Name:            "files",
		PurgeExpired:    purgeExpired,
		CacheBytesLimit: app.groupCacheSizeBytes,
		Getter:          getter,
	}

	// https://talks.golang.org/2013/oscon-dl.slide#46
	//
	// 64 MB max per-node memory usage
	app.cache = groupcache.NewGroupWithWorkspace(groupcacheOptions)
}
