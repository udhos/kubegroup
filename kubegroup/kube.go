package kubegroup

import (
	"errors"
	"fmt"
	"os"
	"time"

	"golang.org/x/exp/maps"
	corev1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
)

type kubeClient struct {
	inCluster bool
	podCache  *podInfo
	options   Options
}

type podInfo struct {
	name        string
	namespace   string
	listOptions metav1.ListOptions
}

func newKubeClient(options Options) (kubeClient, error) {

	kc := kubeClient{
		options: options,
	}

	inCluster, errInit := options.Engine.initClient(options)

	kc.inCluster = inCluster

	return kc, errInit
}

func getPodName(errorf func(format string, v ...any)) string {
	const me = "getPodName"
	host, errHost := os.Hostname()
	if errHost != nil {
		errorf("%s: hostname: %v", me, errHost)
	}
	return host
}

func (k *kubeClient) getPod() (*corev1.Pod, error) {
	podName := getPodName(k.options.Errorf)
	if podName == "" {
		return nil, errors.New("missing pod name")
	}

	namespace, errNs := k.options.Engine.findMyNamespace()
	if errNs != nil {
		k.options.Errorf("getPod: could not find pod='%s' namespace: %v", podName, errNs)
		return nil, errNs
	}

	pod, errPod := k.options.Engine.getPod(namespace, podName)
	if errPod != nil {
		k.options.Errorf("getPod: could not find pod name='%s': %v", podName, errPod)
	}

	return pod, errPod
}

func isPodReady(pod *corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func (k *kubeClient) getPodInfo() (*podInfo, error) {
	if k.podCache != nil {
		return k.podCache, nil
	}

	// get my pod
	pod, errPod := k.getPod()
	if errPod != nil {
		k.options.Errorf("getPodInfo: could not find pod: %v", errPod)
		return nil, errPod
	}

	// get namespace from my pod
	namespace := pod.ObjectMeta.Namespace

	// get label to match peer PODs

	var labelKey string
	if k.options.PodLabelKey == "" {
		labelKey = "app" // default label key
	} else {
		labelKey = k.options.PodLabelKey
	}

	var labelValue string
	if k.options.PodLabelValue == "" {
		labelValue = pod.ObjectMeta.Labels[labelKey] // default label value
	} else {
		labelValue = k.options.PodLabelValue
	}

	// search other pods using label from my pod
	listOptions := metav1.ListOptions{LabelSelector: labelKey + "=" + labelValue}

	k.podCache = &podInfo{
		name:        pod.ObjectMeta.Name,
		namespace:   namespace,
		listOptions: listOptions,
	}

	return k.podCache, nil
}

func (k *kubeClient) listPodsAddresses() ([]string, error) {

	table, errTable := k.getPodTable()
	if errTable != nil {
		k.options.Errorf("listPodsAddresses: pod table: %v", errTable)
		return nil, errTable
	}

	return maps.Values(table), nil
}

func (k *kubeClient) getPodTable() (map[string]string, error) {

	const me = "getPodTable"

	table := map[string]string{}

	if !k.inCluster {
		name := getPodName(k.options.Errorf)
		if name == "" {
			return nil, fmt.Errorf("%s: out-of-cluster: missing pod name", me)
		}
		addr, errAddr := findMyAddr()
		if errAddr != nil {
			k.options.Errorf("%s: %v", me, errAddr)
		}
		if addr == "" {
			return nil, fmt.Errorf("%s: out-of-cluster: missing pod address", me)
		}
		table[name] = addr
		return table, nil
	}

	podInfo, errInfo := k.getPodInfo()
	if errInfo != nil {
		k.options.Errorf("%s: pod info: %v", me, errInfo)
		return nil, errInfo
	}

	pods, errList := k.options.Engine.listPods(podInfo.namespace, podInfo.listOptions)
	if errList != nil {
		k.options.Errorf("%s: list pods: %v", me, errList)
		return nil, errList
	}

	for _, p := range pods.Items {
		if !isPodReady(&p) {
			continue
		}
		name := p.ObjectMeta.Name
		addr := p.Status.PodIP
		table[name] = addr
	}

	k.options.Debugf("%s: found=%d ready=%d", me, len(pods.Items), len(table))

	return table, nil
}

func (k *kubeClient) watchPodsAddresses(out chan<- podAddress, done chan struct{}) error {

	const me = "watchPodsAddresses"

	defer close(out) // notify readers

	if !k.inCluster {
		return nil // nothing to do
	}

	// some going down events don't report pod address, so we retrieve addr from a local table
	table, errTable := k.getPodTable()
	if errTable != nil {
		k.options.Errorf("%s: table: %v", me, errTable)
		return errTable
	}

	k.options.Debugf("%s: initial table: %v", me, table)

	podInfo, errInfo := k.getPodInfo()
	if errInfo != nil {
		k.options.Errorf("%s: pod info: %v", me, errInfo)
		return errInfo
	}

	for {
		select {
		case <-done:
			k.options.Debugf("%s: done channel closed, exiting", me)
			return nil
		default:
			errWatch := k.watchOnce(out, podInfo, table, done)
			k.options.Errorf("%s: %v", me, errWatch)
			if errWatch != errWatchInputChannelClose {
				return errWatch
			}
			k.options.Errorf("%s: retrying in %v", me, k.options.Cooldown)
			time.Sleep(k.options.Cooldown)
		}
	}
}

var errWatchInputChannelClose = errors.New("watchOnce: input channel has been closed")

func (k *kubeClient) watchOnce(out chan<- podAddress, info *podInfo, table map[string]string, done chan struct{}) error {
	const me = "watchOnce"

	myPodName := info.name

	watcher, errWatch := k.options.Engine.watchPods(info.namespace, info.listOptions)
	if errWatch != nil {
		k.options.Errorf("%s: watch: %v", me, errWatch)
		return errWatch
	}

	in := watcher.ResultChan()
	for {
		select {
		case <-done:
			watcher.Stop()
			k.options.Debugf("%s: done channel closed, exiting", me)
			return nil
		case event, ok := <-in:
			if !ok {
				return errWatchInputChannelClose
			}
			if result, ok := action(table, event, myPodName, k.options); ok {
				out <- result
			}
		}
	}

	// not reached
}

func action(table map[string]string, event watch.Event, myPodName string, options Options) (podAddress, bool) {
	const me = "action"

	var result podAddress

	pod, ok := event.Object.(*corev1.Pod)
	if !ok {
		options.Errorf("%s: unexpected event object: %v", me, event.Object)
		return result, false
	}

	if pod == nil {
		options.Errorf("%s: unexpected nil pod from event object: %v", me, event.Object)
		return result, false
	}

	name := pod.ObjectMeta.Name

	addr := pod.Status.PodIP
	if addr == "" {
		// some going down events don't report pod address, so we retrieve it from a local table
		addr = table[name]
	}

	ready := isPodReady(pod)

	if name == myPodName {
		options.Debugf("%s: event=%s pod=%s addr=%s ready=%t: ignoring my own pod",
			me, event.Type, name, addr, ready)
		return result, false // ignore my own pod
	}

	if event.Type == watch.Deleted {
		// pod name/address no longer needed
		delete(table, name)
	}

	if addr == "" {
		options.Debugf("%s: event=%s pod=%s addr=%s ready=%t: ignoring, cannot add/remove unknown address",
			me, event.Type, name, addr, ready)
		return result, false // ignore empty address
	}

	options.Debugf("%s: event=%s pod=%s addr=%s ready=%t: success: sending update",
		me, event.Type, name, addr, ready)

	if event.Type != watch.Deleted {
		// record address for future going down events that don't report pod address
		table[name] = addr
	}

	result.address = addr
	result.added = ready
	return result, true
}

type podAddress struct {
	address string
	added   bool
}
