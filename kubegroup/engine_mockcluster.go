package kubegroup

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
)

// KubeMockCluster implements a bogus kube engine for testing.
type KubeMockCluster struct {
	options   Options
	myAddr    string
	addresses []string
}

// NewKubeMockCluster create a new KubeMockCluster engine.
func NewKubeMockCluster(myAddr string, addresses []string) *KubeMockCluster {
	return &KubeMockCluster{
		myAddr:    myAddr,
		addresses: addresses,
	}
}

func (e *KubeMockCluster) initClient(options Options) (bool, error) {
	e.options = options
	return true, nil
}

func (e *KubeMockCluster) getPod(namespace, podName string) (*corev1.Pod, error) {
	addr, _ := e.findMyAddress()
	pod := newPod(namespace, podName, addr)
	return pod, nil
}

func newPod(namespace, podName, address string) *corev1.Pod {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
			Labels: map[string]string{
				"app": "app1",
			},
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
			PodIP: address,
		},
	}
	return &pod
}

func (e *KubeMockCluster) listPods(namespace string,
	_ /*opts*/ metav1.ListOptions) (*corev1.PodList, error) {

	list := corev1.PodList{
		Items: []corev1.Pod{},
	}

	for i, addr := range e.addresses {
		podName := fmt.Sprintf("pod-%d", i)
		pod := newPod(namespace, podName, addr)
		list.Items = append(list.Items, *pod)
	}

	return &list, nil
}

func (e *KubeMockCluster) watchPods(_ /*namespace*/ string,
	_ /*opts*/ metav1.ListOptions) (watch.Interface, error) {
	w := portClusterWatch{
		ch: make(chan watch.Event),
	}
	return &w, nil
}

type portClusterWatch struct {
	ch chan watch.Event
}

func (w *portClusterWatch) Stop() {
	close(w.ch)
}

func (w *portClusterWatch) ResultChan() <-chan watch.Event {
	return w.ch
}

func (e *KubeMockCluster) findMyNamespace() (string, error) {
	return "default", nil
}

func (e *KubeMockCluster) findMyAddress() (string, error) {
	return e.myAddr, nil
}
