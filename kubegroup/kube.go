package kubegroup

import (
	"errors"
	"fmt"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type kubeClient struct {
	inCluster bool
	podCache  *podInfo
	options   Options
	m         *metrics
}

type podInfo struct {
	name        string
	namespace   string
	listOptions metav1.ListOptions
}

func newKubeClient(options Options) (kubeClient, error) {

	kc := kubeClient{
		options: options,
		m:       newMetrics(options),
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

func (k *kubeClient) getPodTable() (map[string]string, error) {

	const me = "getPodTable"

	table := map[string]string{} // name => addr

	if !k.inCluster {
		name := getPodName(k.options.Errorf)
		if name == "" {
			return nil, fmt.Errorf("%s: out-of-cluster: missing pod name", me)
		}
		addr, errAddr := k.options.Engine.findMyAddress()
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
		k.options.Errorf("%s: not in-cluster, nothing to do", me)
		return nil // nothing to do
	}

	ticker := time.NewTicker(k.options.ListerInterval)

	defer ticker.Stop()

	addresses := map[string]struct{}{} // addr set

	for {
		select {

		case <-done:
			k.options.Debugf("%s: done channel closed, exiting", me)
			return nil

		case <-ticker.C:
			tab, errTable := k.getPodTable()
			if errTable != nil {
				k.options.Errorf("%s: table: %v", me, errTable)
				continue
			}

			count := len(tab)

			//
			// build new address set
			//
			newAddrs := make(map[string]struct{}, count)
			for _, addr := range tab {
				newAddrs[addr] = struct{}{}
			}

			k.options.Debugf("%s: interval=%s new addresses: count=%d: %v",
				me, k.options.ListerInterval, count, newAddrs)

			//
			// remove old addresses
			//
			for addr := range addresses {
				if _, found := newAddrs[addr]; !found {
					out <- podAddress{address: addr, added: false}
				}
			}

			//
			// add new addresses
			//
			for addr := range newAddrs {
				out <- podAddress{address: addr, added: true}
			}

			addresses = newAddrs // replace addr set
		}
	}
}

type podAddress struct {
	address string
	added   bool
}
