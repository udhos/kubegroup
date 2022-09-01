package kubegroup

import (
	"context"
	"errors"
	"log"
	"os"
	"time"

	"golang.org/x/exp/maps"
	corev1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type kubeClient struct {
	clientset *kubernetes.Clientset
	inCluster bool
	podCache  *podInfo
}

type podInfo struct {
	name        string
	namespace   string
	listOptions metav1.ListOptions
}

func newKubeClient() (kubeClient, error) {

	kc := kubeClient{}

	config, errConfig := rest.InClusterConfig()
	if errConfig != nil {
		log.Printf("running OUT-OF-CLUSTER: %v", errConfig)
		return kc, nil
	}

	log.Printf("running IN-CLUSTER")
	kc.inCluster = true

	clientset, errClientset := kubernetes.NewForConfig(config)
	if errClientset != nil {
		log.Fatalf("kube clientset error: %v", errClientset)
		return kc, errClientset
	}

	kc.clientset = clientset

	return kc, nil
}

func (k *kubeClient) getPodName() string {
	host, errHost := os.Hostname()
	if errHost != nil {
		log.Printf("getPodName: hostname: %v", errHost)
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
		log.Printf("getPod: could not find pod='%s' namespace: %v", podName, errNs)
		return nil, errNs
	}

	pod, errPod := k.clientset.CoreV1().Pods(namespace).Get(context.TODO(), podName, metav1.GetOptions{})
	if errPod != nil {
		log.Printf("getPod: could not find pod name='%s': %v", podName, errPod)
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
		log.Printf("getPodInfo: could not find pod: %v", errPod)
		return nil, errPod
	}

	// get namespace from my pod
	namespace := pod.ObjectMeta.Namespace

	// get label from my pod
	labelKey := "app"
	labelValue := pod.ObjectMeta.Labels[labelKey]

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

	/*
		if !k.inCluster {
			return []string{findMyAddr()}, nil
		}

		podInfo, errInfo := k.getPodInfo()
		if errInfo != nil {
			log.Printf("listPodsAddresses: pod info: %v", errInfo)
			return nil, errInfo
		}

		pods, errList := k.clientset.CoreV1().Pods(podInfo.namespace).List(context.TODO(), podInfo.listOptions)
		if errList != nil {
			log.Printf("listPodsAddresses: list pods: %v", errList)
			return nil, errList
		}

		var podList []string

		for _, p := range pods.Items {
			if isPodReady(&p) {
				addr := p.Status.PodIP
				podList = append(podList, addr)
			}
		}

		return podList, nil
	*/

	table, errTable := k.getPodTable()
	if errTable != nil {
		log.Printf("listPodsAddresses: pod table: %v", errTable)
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
			log.Printf("getPodTable: %v", errAddr)
		}
		if addr == "" {
			return nil, errors.New("getPodTable: out-of-cluster: missing pod address")
		}
		table[name] = addr
		return table, nil
	}

	podInfo, errInfo := k.getPodInfo()
	if errInfo != nil {
		log.Printf("getPodTable: pod info: %v", errInfo)
		return nil, errInfo
	}

	pods, errList := k.clientset.CoreV1().Pods(podInfo.namespace).List(context.TODO(), podInfo.listOptions)
	if errList != nil {
		log.Printf("getPodTable: list pods: %v", errList)
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
		log.Printf("watchPodsAddresses: table: %v", errTable)
		return errTable
	}

	log.Printf("watchPodsAddresses: initial table: %v", table)

	podInfo, errInfo := k.getPodInfo()
	if errInfo != nil {
		log.Printf("watchPodsAddresses: pod info: %v", errInfo)
		return errInfo
	}

	const cooldown = 5 * time.Second
	for {
		errWatch := k.watchOnce(out, podInfo, table)
		log.Printf("watchPodsAddresses: %v", errWatch)
		if errWatch != errWatchInputChannelClose {
			return errWatch
		}
		log.Printf("watchPodsAddresses: retrying in %v", cooldown)
		time.Sleep(cooldown)
	}
}

var errWatchInputChannelClose = errors.New("watchOnce: input channel has been closed")

func (k *kubeClient) watchOnce(out chan<- podAddress, info *podInfo, table map[string]string) error {
	myPodName := info.name

	watch, errWatch := k.clientset.CoreV1().Pods(info.namespace).Watch(context.TODO(), info.listOptions)
	if errWatch != nil {
		log.Printf("watchOnce: watch: %v", errWatch)
		return errWatch
	}

	in := watch.ResultChan()
	for event := range in {
		pod, ok := event.Object.(*corev1.Pod)
		if !ok {
			log.Printf("watchOnce: unexpected event object: %v", event.Object)
			continue
		}

		name := pod.Name

		addr := pod.Status.PodIP
		if addr == "" {
			// some going down events don't report pod address, so we retrieve it from a local table
			addr = table[name]
		}

		ready := isPodReady(pod)

		log.Printf("watchOnce: event=%s pod=%s addr=%s ready=%t",
			event.Type, name, addr, ready)

		if name == myPodName {
			continue // ignore my own pod
		}

		if event.Type == "DELETED" {
			// pod name/address no longer needed
			delete(table, name)
		}

		if addr == "" {
			continue // ignore empty address
		}

		// record address for future going down events that don't report pod address
		table[name] = addr

		out <- podAddress{address: addr, added: ready}
	}

	return errWatchInputChannelClose
}

type podAddress struct {
	address string
	added   bool
}
