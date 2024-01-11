package kubegroup

import (
	"context"
	"os"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// KubeEngine defines interface for kube client.
type KubeEngine interface {
	initClient(options Options) (bool, error)
	getPod(namespace, podName string) (*corev1.Pod, error)
	listPods(namespace string, opts metav1.ListOptions) (*corev1.PodList, error)
	watchPods(namespace string, opts metav1.ListOptions) (watch.Interface, error)
	findMyNamespace() (string, error)
	findMyAddress() (string, error)
}

// KubeReal defines real engine implementing KubeEngine interface.
type KubeReal struct {
	clientset *kubernetes.Clientset
	options   Options
}

// NewKubeReal creates a new KubeReal engine.
func NewKubeReal() *KubeReal {
	return &KubeReal{}
}

func (e *KubeReal) initClient(options Options) (bool, error) {

	var inCluster bool

	config, errConfig := rest.InClusterConfig()
	if errConfig != nil {
		options.Errorf("running OUT-OF-CLUSTER: %v", errConfig)
		return inCluster, nil
	}

	options.Debugf("running IN-CLUSTER")
	inCluster = true

	clientset, errClientset := kubernetes.NewForConfig(config)
	if errClientset != nil {
		options.Errorf("kube clientset error: %v", errClientset)
		return inCluster, errClientset
	}

	e.clientset = clientset
	e.options = options

	return inCluster, nil
}

func (e *KubeReal) getPod(namespace, podName string) (*corev1.Pod, error) {
	return e.clientset.CoreV1().Pods(namespace).Get(context.TODO(), podName, metav1.GetOptions{})
}

func (e *KubeReal) listPods(namespace string, opts metav1.ListOptions) (*corev1.PodList, error) {
	return e.clientset.CoreV1().Pods(namespace).List(context.TODO(), opts)
}

func (e *KubeReal) watchPods(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	return e.clientset.CoreV1().Pods(namespace).Watch(context.TODO(), opts)
}

func (e *KubeReal) findMyNamespace() (string, error) {
	buf, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	return string(buf), err
}

func (e *KubeReal) findMyAddress() (string, error) {
	return findMyAddr()
}

// KubeBogus implements a bogus kube engine for testing.
type KubeBogus struct {
	options Options
}

// NewKubeBogus create a new KubeBogus engine.
func NewKubeBogus() *KubeBogus {
	return &KubeBogus{}
}

func (e *KubeBogus) initClient(options Options) (bool, error) {
	e.options = options
	return true, nil
}

func (e *KubeBogus) getPod(namespace, podName string) (*corev1.Pod, error) {

	addr, _ := e.findMyAddress()

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
			PodIP: addr,
		},
	}
	return &pod, nil
}

func (e *KubeBogus) listPods(namespace string, _ /*opts*/ metav1.ListOptions) (*corev1.PodList, error) {
	podName := getPodName(e.options.Errorf)
	pod, _ := e.getPod(namespace, podName)
	list := corev1.PodList{
		Items: []corev1.Pod{*pod},
	}
	return &list, nil
}

func (e *KubeBogus) watchPods(_ /*namespace*/ string, _ /*opts*/ metav1.ListOptions) (watch.Interface, error) {
	w := bogusWatch{
		ch: make(chan watch.Event),
	}
	return &w, nil
}

type bogusWatch struct {
	ch chan watch.Event
}

func (w *bogusWatch) Stop() {
	close(w.ch)
}

func (w *bogusWatch) ResultChan() <-chan watch.Event {
	return w.ch
}

func (e *KubeBogus) findMyNamespace() (string, error) {
	return "default", nil
}

func (e *KubeBogus) findMyAddress() (string, error) {
	return "127.0.0.1", nil
}
