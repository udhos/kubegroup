package kubegroup

import (
	"context"
	"errors"
	"os"
	"time"

	"golang.org/x/exp/maps"
	corev1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type kubeClient struct {
	clientset *kubernetes.Clientset
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

	config, errConfig := rest.InClusterConfig()
	if errConfig != nil {
		options.Errorf("running OUT-OF-CLUSTER: %v", errConfig)
		return kc, nil
	}

	options.Debugf("running IN-CLUSTER")
	kc.inCluster = true

	clientset, errClientset := kubernetes.NewForConfig(config)
	if errClientset != nil {
		options.Errorf("kube clientset error: %v", errClientset)
		return kc, errClientset
	}

	kc.clientset = clientset

	return kc, nil
}

func (k *kubeClient) getPodName() string {
	host, errHost := os.Hostname()
	if errHost != nil {
		k.options.Errorf("getPodName: hostname: %v", errHost)
	}
	return host
}

func (k *kubeClient) getPod() (*corev1.Pod, error) {
	podName := k.getPodName()
	if podName == "" {
		return nil, errors.New("missing pod name")
	}

	namespace, errNs := findMyNamespace()
	if errNs != nil {
		k.options.Errorf("getPod: could not find pod='%s' namespace: %v", podName, errNs)
		return nil, errNs
	}

	pod, errPod := k.clientset.CoreV1().Pods(namespace).Get(context.TODO(), podName, metav1.GetOptions{})
	if errPod != nil {
		k.options.Errorf("getPod: could not find pod name='%s': %v", podName, errPod)
	}

	return pod, errPod
}

func findMyNamespace() (string, error) {
	buf, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	return string(buf), err
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

	table := map[string]string{}

	if !k.inCluster {
		name := k.getPodName()
		if name == "" {
			return nil, errors.New("getPodTable: out-of-cluster: missing pod name")
		}
		addr, errAddr := findMyAddr()
		if errAddr != nil {
			k.options.Errorf("getPodTable: %v", errAddr)
		}
		if addr == "" {
			return nil, errors.New("getPodTable: out-of-cluster: missing pod address")
		}
		table[name] = addr
		return table, nil
	}

	podInfo, errInfo := k.getPodInfo()
	if errInfo != nil {
		k.options.Errorf("getPodTable: pod info: %v", errInfo)
		return nil, errInfo
	}

	pods, errList := k.clientset.CoreV1().Pods(podInfo.namespace).List(context.TODO(), podInfo.listOptions)
	if errList != nil {
		k.options.Errorf("getPodTable: list pods: %v", errList)
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

	return table, nil
}

func (k *kubeClient) watchPodsAddresses(out chan<- podAddress) error {

	defer close(out) // notify readers

	if !k.inCluster {
		return nil // nothing to do
	}

	// some going down events don't report pod address, so we retrieve addr from a local table
	table, errTable := k.getPodTable()
	if errTable != nil {
		k.options.Errorf("watchPodsAddresses: table: %v", errTable)
		return errTable
	}

	k.options.Debugf("watchPodsAddresses: initial table: %v", table)

	podInfo, errInfo := k.getPodInfo()
	if errInfo != nil {
		k.options.Errorf("watchPodsAddresses: pod info: %v", errInfo)
		return errInfo
	}

	for {
		errWatch := k.watchOnce(out, podInfo, table)
		k.options.Errorf("watchPodsAddresses: %v", errWatch)
		if errWatch != errWatchInputChannelClose {
			return errWatch
		}
		k.options.Errorf("watchPodsAddresses: retrying in %v", k.options.Cooldown)
		time.Sleep(k.options.Cooldown)
	}
}

var errWatchInputChannelClose = errors.New("watchOnce: input channel has been closed")

func (k *kubeClient) watchOnce(out chan<- podAddress, info *podInfo, table map[string]string) error {
	myPodName := info.name

	watcher, errWatch := k.clientset.CoreV1().Pods(info.namespace).Watch(context.TODO(), info.listOptions)
	if errWatch != nil {
		k.options.Errorf("watchOnce: watch: %v", errWatch)
		return errWatch
	}

	in := watcher.ResultChan()
	for event := range in {
		if result, ok := action(table, event, myPodName, k.options); ok {
			out <- result
		}
	}

	return errWatchInputChannelClose
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
