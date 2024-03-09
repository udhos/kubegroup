[![license](http://img.shields.io/badge/license-MIT-blue.svg)](https://github.com/udhos/kubegroup/blob/main/LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/udhos/kubegroup)](https://goreportcard.com/report/github.com/udhos/kubegroup)
[![Go Reference](https://pkg.go.dev/badge/github.com/udhos/kubegroup.svg)](https://pkg.go.dev/github.com/udhos/kubegroup)

# kubegroup

[kubegroup](https://github.com/udhos/kubegroup) provides peer autodiscovery for pods running [groupcache](https://github.com/mailgun/groupcache) within a kubernetes cluster.

Peer pods are automatically discovered by continuously watching for other pods with the same label `app=<value>` as in the current pod, in current pod's namespace.

# Usage

Import the package `github.com/udhos/kubegroup/kubegroup`.

```go
import "github.com/udhos/kubegroup/kubegroup"
```

Spawn a goroutine for `kubegroup.UpdatePeers(pool, groupcachePort)`.

```go
groupcachePort := ":5000"

// 1. get my groupcache URL
myURL, errURL = kubegroup.FindMyURL(groupcachePort)

// 2. spawn groupcache peering server
pool := groupcache.NewHTTPPoolOpts(myURL, &groupcache.HTTPPoolOptions{})
server := &http.Server{Addr: groupcachePort, Handler: pool}
go func() {
    log.Printf("groupcache server: listening on %s", groupcachePort)
    err := server.ListenAndServe()
    log.Printf("groupcache server: exited: %v", err)
}()

// 3. spawn peering autodiscovery

clientset, errClientset := kubeclient.New(kubeclient.Options{})
if errClientset != nil {
  log.Fatalf("startGroupcache: kubeclient: %v", errClientset)
}

options := kubegroup.Options{
  Client:                clientset,
  Pool:                  pool,
  LabelSelector:         "app=my-app",
  GroupCachePort:        groupCachePort,
  MetricsRegisterer:     prometheus.DefaultRegisterer,
  MetricsGatherer:       prometheus.DefaultGatherer,
}

group, errGroup := kubegroup.UpdatePeers(options)
if errGroup != nil {
  log.Fatalf("kubegroup: %v", errGroup)
}

// 4. create groupcache groups, etc: groupOne := groupcache.NewGroup()

// 5. before shutdown you might stop kubegroup to release resources

group.Close() // release kubegroup resources
server.Shutdown(...) // do not forget to shutdown the peering server
```

# Example

See [./examples/kubegroup-example](./examples/kubegroup-example)

# POD Permissions

The application PODs will need permissions to get/list/watch PODs against kubernetes API, as illustrated by the role below.

## Role

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: role-name
rules:
- apiGroups:
  - ""
  resources:
  - 'pods'
  verbs:
  - 'get'
  - 'list'
  - 'watch'
```

Example chart template: https://github.com/udhos/gateboard/blob/main/charts/gateboard/templates/role.yaml

## Role Binding

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: rolebinding-name
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: role-name
subjects:
- kind: ServiceAccount
  name: put-pod-service-account-here
  namespace: put-pod-namespace-here
```

Example chart template: https://github.com/udhos/gateboard/blob/main/charts/gateboard/templates/rolebinding.yaml
