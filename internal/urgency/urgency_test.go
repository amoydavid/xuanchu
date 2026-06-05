package urgency

import (
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/task"
)

func TestExplainIncludesNextAndPriority(t *testing.T) {
	priority := "H"
	tsk := task.Task{
		UUID: "u1", Description: "important", Status: task.StatusPending,
		Entry: 0, Modified: 0, Priority: &priority, Tags: []string{"next"},
	}
	explain := Explain(tsk, Options{NowUnix: 86400})
	if explain.Total < 21 {
		t.Fatalf("Total = %f, want next + H contribution", explain.Total)
	}
	if !hasItem(explain, "tag.next") || !hasItem(explain, "priority.H") {
		t.Fatalf("items = %#v", explain.Items)
	}
}

func TestExplainTotalEqualsItemSum(t *testing.T) {
	tsk := task.Task{UUID: "u1", Description: "task", Status: task.StatusPending, Entry: 0, Modified: 0, Tags: []string{"a", "b"}}
	explain := Explain(tsk, Options{NowUnix: 86400})
	var sum float64
	for _, item := range explain.Items {
		sum += item.Contribution
	}
	if explain.Total != sum {
		t.Fatalf("Total = %f, sum = %f", explain.Total, sum)
	}
}

func TestDueContributionBoundaries(t *testing.T) {
	now := int64(100 * 86400)
	for _, tc := range []struct {
		name         string
		days         int64
		wantPositive bool
	}{
		{name: "today", days: 0, wantPositive: true},
		{name: "three days", days: 3, wantPositive: true},
		{name: "seven days", days: 7, wantPositive: false},
		{name: "thirty days", days: 30, wantPositive: false},
	} {
		due := now + tc.days*86400
		tsk := task.Task{UUID: "u1", Description: "task", Status: task.StatusPending, Entry: now - 86400, Modified: now - 86400, Due: &due}
		explain := Explain(tsk, Options{NowUnix: now})
		got := hasItem(explain, "due")
		if got != tc.wantPositive {
			t.Fatalf("%s due item = %v, want %v", tc.name, got, tc.wantPositive)
		}
	}
}

func TestPriorityAndMultipleTags(t *testing.T) {
	for _, priority := range []string{"L", "M", "H"} {
		tsk := task.Task{UUID: "u1", Description: "task", Status: task.StatusPending, Entry: 0, Modified: 0, Priority: &priority, Tags: []string{"a", "b", "c"}}
		explain := Explain(tsk, Options{NowUnix: 86400})
		if !hasItem(explain, "priority."+priority) || !hasItem(explain, "tags") {
			t.Fatalf("priority %s items = %#v", priority, explain.Items)
		}
	}
}

func TestExplainIncludesM2Contributions(t *testing.T) {
	start := int64(10)
	wait := int64(200)
	tsk := task.Task{
		UUID: "u1", Description: "task", Status: task.StatusWaiting, Entry: 0, Modified: 0,
		Start: &start, Wait: &wait,
		Annotations: []task.Annotation{{Entry: 1, Description: "note"}},
		Depends:     []string{"dep"},
	}
	explain := Explain(tsk, Options{NowUnix: 100, Blocked: true, Blocking: true})
	for _, name := range []string{"active", "waiting", "blocked", "blocking", "annotations"} {
		if !hasItem(explain, name) {
			t.Fatalf("items missing %s: %#v", name, explain.Items)
		}
	}
}

func TestExplainIncludesUDAContribution(t *testing.T) {
	tsk := task.Task{
		UUID: "u1", Description: "task", Status: task.StatusPending, Entry: 0, Modified: 0,
		UDAs: map[string]task.UDAValue{"estimate": {Name: "estimate", Raw: "3", Type: "numeric"}},
	}
	explain := Explain(tsk, Options{NowUnix: 100, UDACoefficients: map[string]float64{"estimate": 2}, UDAValueCoefficients: map[string]float64{"estimate.3": 5}})
	if !hasItem(explain, "uda.estimate") || !hasItem(explain, "uda.estimate.3") {
		t.Fatalf("items = %#v", explain.Items)
	}
}

func hasItem(explain ExplainResult, name string) bool {
	for _, item := range explain.Items {
		if item.Name == name {
			return true
		}
	}
	return false
}
