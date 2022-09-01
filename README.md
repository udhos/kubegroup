# kubegroup

## Usage

Import the package `github.com/udhos/kubegroup/kubegroup`.

```
import "github.com/udhos/kubegroup/kubegroup"
```

Spawn a goroutine for `kubegroup.UpdatePeers(pool, groupcachePort)`.

```
groupcachePort := ":5000"

// 1. get my groupcache URL
myURL, errURL = kubegroup.FindMyURL(groupcachePort)

// 2. spawn groupcache peering server
pool := groupcache.NewHTTPPool(myURL)
server := &http.Server{Addr: groupcachePort, Handler: pool}
go func() {
    log.Printf("groupcache server: listening on %s", groupcachePort)
    err := server.ListenAndServe()
    log.Printf("groupcache server: exited: %v", err)
}()

// 3. spawn peering autodiscovery
go kubegroup.UpdatePeers(pool, groupcachePort)

// 4. create groupcache groups, etc: groupOne := groupcache.NewGroup()
```
