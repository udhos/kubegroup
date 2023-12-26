package kubegroup

import (
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
)

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

	eventType     watch.EventType
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
		watch.Deleted,
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
		watch.Modified,
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
		watch.Deleted,
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
		watch.Modified,
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
		watch.Deleted,
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
		watch.Modified,
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
		watch.Modified,
		"other-pod",
		"",
		podNotReady,
		"my-pod",
		"2.2.2.2",
		removed,
		resultTrue,
	},
}

func testOptions() Options {
	return defaultOptions(Options{Debug: true})
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

		result, ok := action(data.table, event, data.myPodName, testOptions())
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

func TestActionNonDeleteWithReady(t *testing.T) {

	table := map[string]string{}

	for i := 1; i <= 3; i++ {

		addr := fmt.Sprintf("%[1]d.%[1]d.%[1]d.%[1]d", i)

		pod := corev1.Pod{
			ObjectMeta: v1.ObjectMeta{Name: fmt.Sprintf("other-pod-%d", i)},
			Status: corev1.PodStatus{
				PodIP:      addr,
				Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
			},
		}

		event := watch.Event{
			Type:   watch.Modified,
			Object: &pod,
		}

		result, ok := action(table, event, "this-pod", testOptions())
		if !ok {
			t.Errorf("unexpected not ok action")
		}
		if !result.added {
			t.Errorf("unexpected not added action")
		}
		if result.address != addr {
			t.Errorf("wrong action address: expected=%s got=%s", addr, result.address)
		}
	}

	if len(table) != 3 {
		t.Errorf("unexpected table size: expected=%d got=%d", 3, len(table))
	}
}

func TestActionNonDeleteWithNotReady(t *testing.T) {

	table := map[string]string{}

	for i := 1; i <= 3; i++ {

		addr := fmt.Sprintf("%[1]d.%[1]d.%[1]d.%[1]d", i)

		pod := corev1.Pod{
			ObjectMeta: v1.ObjectMeta{Name: fmt.Sprintf("other-pod-%d", i)},
			Status: corev1.PodStatus{
				PodIP:      addr,
				Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionFalse}},
			},
		}

		event := watch.Event{
			Type:   watch.Modified,
			Object: &pod,
		}

		result, ok := action(table, event, "this-pod", testOptions())
		if !ok {
			t.Errorf("unexpected not ok action")
		}
		if result.added {
			t.Errorf("unexpected added action")
		}
		if result.address != addr {
			t.Errorf("wrong action address: expected=%s got=%s", addr, result.address)
		}
	}

	if len(table) != 3 {
		t.Errorf("unexpected table size: expected=%d got=%d", 3, len(table))
	}
}

func TestActionDeleteWithReady(t *testing.T) {

	table := map[string]string{}

	for i := 1; i <= 3; i++ {

		addr := fmt.Sprintf("%[1]d.%[1]d.%[1]d.%[1]d", i)

		pod := corev1.Pod{
			ObjectMeta: v1.ObjectMeta{Name: fmt.Sprintf("other-pod-%d", i)},
			Status: corev1.PodStatus{
				PodIP:      addr,
				Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
			},
		}

		event := watch.Event{
			Type:   watch.Deleted,
			Object: &pod,
		}

		result, ok := action(table, event, "this-pod", testOptions())
		if !ok {
			t.Errorf("unexpected not ok action")
		}
		if !result.added {
			t.Errorf("unexpected not added action")
		}
		if result.address != addr {
			t.Errorf("wrong action address: expected=%s got=%s", addr, result.address)
		}
	}

	if len(table) != 0 {
		t.Errorf("unexpected table size: expected=%d got=%d", 0, len(table))
	}
}

// go test -run TestActionDeleteWithNotReady ./kubegroup
func TestActionDeleteWithNotReady(t *testing.T) {

	table := map[string]string{}

	for i := 1; i <= 3; i++ {

		addr := fmt.Sprintf("%[1]d.%[1]d.%[1]d.%[1]d", i)

		pod := corev1.Pod{
			ObjectMeta: v1.ObjectMeta{Name: fmt.Sprintf("other-pod-%d", i)},
			Status: corev1.PodStatus{
				PodIP:      addr,
				Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionFalse}},
			},
		}

		event := watch.Event{
			Type:   watch.Deleted,
			Object: &pod,
		}

		result, ok := action(table, event, "this-pod", testOptions())
		if !ok {
			t.Errorf("unexpected not ok action")
		}
		if result.added {
			t.Errorf("unexpected added action")
		}
		if result.address != addr {
			t.Errorf("wrong action address: expected=%s got=%s", addr, result.address)
		}
	}

	if len(table) != 0 {
		t.Errorf("unexpected table size: expected=%d got=%d", 0, len(table))
	}
}

func TestActionMyPodName(t *testing.T) {

	table := map[string]string{}

	pod := corev1.Pod{
		ObjectMeta: v1.ObjectMeta{Name: "this-pod"},
		Status: corev1.PodStatus{
			PodIP:      "1.1.1.1",
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
		},
	}

	event := watch.Event{
		Type:   watch.Modified,
		Object: &pod,
	}

	_, ok := action(table, event, "this-pod", testOptions())
	if ok {
		t.Errorf("unexpected ok action")
	}

	if len(table) != 0 {
		t.Errorf("unexpected table size: expected=%d got=%d", 0, len(table))
	}
}

func TestActionMissingAddr(t *testing.T) {

	table := map[string]string{}

	pod := corev1.Pod{
		ObjectMeta: v1.ObjectMeta{Name: "other-pod"},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
		},
	}

	event := watch.Event{
		Type:   watch.Modified,
		Object: &pod,
	}

	_, ok := action(table, event, "this-pod", testOptions())
	if ok {
		t.Errorf("unexpected ok action")
	}

	if len(table) != 0 {
		t.Errorf("unexpected table size: expected=%d got=%d", 0, len(table))
	}
}

func TestActionMissingAddrSolvedFromTable(t *testing.T) {

	addr := "1.1.1.1"

	table := map[string]string{"other-pod": addr}

	pod := corev1.Pod{
		ObjectMeta: v1.ObjectMeta{Name: "other-pod"},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
		},
	}

	event := watch.Event{
		Type:   watch.Modified,
		Object: &pod,
	}

	result, ok := action(table, event, "this-pod", testOptions())
	if !ok {
		t.Errorf("unexpected not ok action")
	}
	if !result.added {
		t.Errorf("unexpected not added action")
	}
	if result.address != addr {
		t.Errorf("wrong action address: expected=%s got=%s", addr, result.address)
	}

	if len(table) != 1 {
		t.Errorf("unexpected table size: expected=%d got=%d", 1, len(table))
	}
}
