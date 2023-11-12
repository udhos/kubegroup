package kubegroup

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
)

type resultExpect int

const (
	podReady    = true
	podNotReady = false
	added       = true
	removed     = false
	resultTrue  = true
	resultFalse = false
)

type testCaseAction struct {
	name  string
	table map[string]string
	//event         watch.Event
	eventType     string
	eventPodName  string
	eventPodIP    string
	eventPodReady bool

	myPodName string

	expectAddress string
	expectAdded   bool
	expectResult  bool
}

var testTableAction = []testCaseAction{
	{
		"same pod name should result in false action",
		map[string]string{},
		"DELETED",
		"my-pod",
		"1.1.1.1",
		podReady,
		"my-pod",
		"1.1.1.1",
		added,
		resultFalse,
	},
	{
		"other pod name should result in true action",
		map[string]string{},
		"<DONT-CARE>",
		"other-pod",
		"1.1.1.1",
		podReady,
		"my-pod",
		"1.1.1.1",
		added,
		resultTrue,
	},
	{
		"DELETE event type with ready pod should result in added action",
		map[string]string{},
		"DELETE",
		"other-pod",
		"1.1.1.1",
		podReady,
		"my-pod",
		"1.1.1.1",
		added,
		resultTrue,
	},
	{
		"any event type with ready pod should result in added action",
		map[string]string{},
		"<DONT-CARE>",
		"other-pod",
		"1.1.1.1",
		podReady,
		"my-pod",
		"1.1.1.1",
		added,
		resultTrue,
	},
	{
		"DELETE event type with notReady pod should result in removed action",
		map[string]string{},
		"DELETE",
		"other-pod",
		"1.1.1.1",
		podNotReady,
		"my-pod",
		"1.1.1.1",
		removed,
		resultTrue,
	},
	{
		"any event type with notReady pod should result in removed action",
		map[string]string{},
		"<DONT-CARE>",
		"other-pod",
		"1.1.1.1",
		podNotReady,
		"my-pod",
		"1.1.1.1",
		removed,
		resultTrue,
	},
	{
		"retrieve missing address from table",
		map[string]string{"other-pod": "2.2.2.2"},
		"<DONT-CARE>",
		"other-pod",
		"",
		podNotReady,
		"my-pod",
		"2.2.2.2",
		removed,
		resultTrue,
	},
}

func TestAction(t *testing.T) {

	for _, data := range testTableAction {

		pod := corev1.Pod{
			ObjectMeta: v1.ObjectMeta{Name: data.eventPodName},
			Status: corev1.PodStatus{
				PodIP: data.eventPodIP,
			},
		}

		if data.eventPodReady {
			pod.Status.Conditions = append(pod.Status.Conditions,
				corev1.PodCondition{Type: corev1.PodReady, Status: corev1.ConditionTrue})
		}

		event := watch.Event{
			Type:   watch.EventType(data.eventType),
			Object: &pod,
		}

		result, ok := action(data.table, event, data.myPodName)
		if ok != data.expectResult {
			t.Errorf("%s: wrong result: expected=%t got=%t", data.name, data.expectResult, ok)
		}
		if !ok {
			continue
		}
		if result.added != data.expectAdded {
			t.Errorf("%s: wrong added: expected=%t got=%t", data.name, data.expectAdded, result.added)
		}
		if result.address != data.expectAddress {
			t.Errorf("%s: wrong address: expected=%s got=%s", data.name, data.expectAddress, result.address)
		}
	}
}
