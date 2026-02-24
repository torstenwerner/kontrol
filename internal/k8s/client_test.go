package k8s

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestListNamespacesSorted(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "zeta"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "alpha"}},
	)

	sut := NewClient(client)
	namespaces, err := sut.ListNamespaces(context.Background())
	if err != nil {
		t.Fatalf("ListNamespaces() error = %v", err)
	}

	want := []string{"alpha", "zeta"}
	if len(namespaces) != len(want) || namespaces[0] != want[0] || namespaces[1] != want[1] {
		t.Fatalf("ListNamespaces() = %v, want %v", namespaces, want)
	}
}

func TestListPodsSortedAndMapped(t *testing.T) {
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	client := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "pod-b",
				Namespace:         "default",
				CreationTimestamp: metav1.NewTime(now.Add(-2 * time.Hour)),
				Labels:            map[string]string{"app": "demo", "tier": "api"},
			},
			Spec: corev1.PodSpec{NodeName: "node-1", Containers: []corev1.Container{{Name: "c1"}}},
			Status: corev1.PodStatus{
				Phase:             corev1.PodRunning,
				PodIP:             "10.0.0.2",
				ContainerStatuses: []corev1.ContainerStatus{{Ready: true, RestartCount: 3}},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "pod-a",
				Namespace:         "default",
				CreationTimestamp: metav1.NewTime(now.Add(-30 * time.Minute)),
			},
			Spec:   corev1.PodSpec{Containers: []corev1.Container{{Name: "c1"}}},
			Status: corev1.PodStatus{Phase: corev1.PodPending},
		},
	)

	sut := NewClient(client)
	rows, err := sut.ListPods(context.Background(), "default")
	if err != nil {
		t.Fatalf("ListPods() error = %v", err)
	}

	if len(rows) != 2 {
		t.Fatalf("ListPods() len = %d, want 2", len(rows))
	}
	if rows[0].Name != "pod-a" || rows[1].Name != "pod-b" {
		t.Fatalf("ListPods() sort order = %v, want pod-a then pod-b", []string{rows[0].Name, rows[1].Name})
	}
}

func TestPodHelpers(t *testing.T) {
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "demo",
			CreationTimestamp: metav1.NewTime(now.Add(-90 * time.Minute)),
			Labels:            map[string]string{"b": "2", "a": "1"},
		},
		Spec: corev1.PodSpec{NodeName: "node-a", Containers: []corev1.Container{{Name: "c1"}, {Name: "c2"}}},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			PodIP: "10.0.0.10",
			ContainerStatuses: []corev1.ContainerStatus{
				{Ready: true, RestartCount: 1},
				{Ready: false, RestartCount: 2},
			},
		},
	}

	if got := PodReady(pod); got != "1/2" {
		t.Fatalf("PodReady() = %q, want %q", got, "1/2")
	}
	if got := PodRestarts(pod); got != "3" {
		t.Fatalf("PodRestarts() = %q, want %q", got, "3")
	}
	if got := PodAge(pod, now); got != "1h" {
		t.Fatalf("PodAge() = %q, want %q", got, "1h")
	}
	if got := LabelsString(pod.Labels); got != "a=1,b=2" {
		t.Fatalf("LabelsString() = %q, want %q", got, "a=1,b=2")
	}
	if got := PodStatus(pod); got != "Running" {
		t.Fatalf("PodStatus() = %q, want %q", got, "Running")
	}

	row := PodToRow(pod, now)
	if row.Name != "demo" || row.Status != "Running" || row.Ready != "1/2" || row.Restarts != "3" || row.Age != "1h" || row.IP != "10.0.0.10" || row.Node != "node-a" || row.Labels != "a=1,b=2" {
		t.Fatalf("PodToRow() produced unexpected row: %+v", row)
	}
}

func TestPodStatusPriority(t *testing.T) {
	now := metav1.Now()
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &now},
		Status: corev1.PodStatus{
			Reason: "PodReason",
			ContainerStatuses: []corev1.ContainerStatus{
				{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}},
			},
		},
	}
	if got := PodStatus(pod); got != "Terminating" {
		t.Fatalf("PodStatus() with deletion timestamp = %q, want %q", got, "Terminating")
	}

	pod.ObjectMeta.DeletionTimestamp = nil
	if got := PodStatus(pod); got != "CrashLoopBackOff" {
		t.Fatalf("PodStatus() with waiting state = %q, want %q", got, "CrashLoopBackOff")
	}
}

func TestFormatAgeBoundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   time.Duration
		want string
	}{
		{name: "negative", in: -time.Second, want: "0s"},
		{name: "seconds", in: 59 * time.Second, want: "59s"},
		{name: "minutes", in: 2 * time.Minute, want: "2m"},
		{name: "hours", in: 3 * time.Hour, want: "3h"},
		{name: "days", in: 10 * 24 * time.Hour, want: "10d"},
		{name: "months", in: 60 * 24 * time.Hour, want: "2mo"},
		{name: "years", in: 730 * 24 * time.Hour, want: "2y"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatAge(tt.in); got != tt.want {
				t.Fatalf("FormatAge(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestListCallsWithNilClientReturnError(t *testing.T) {
	t.Parallel()

	var c *Client
	if _, err := c.ListNamespaces(context.Background()); err == nil {
		t.Fatal("ListNamespaces() expected error for nil client")
	}
	if _, err := c.ListPods(context.Background(), "default"); err == nil {
		t.Fatal("ListPods() expected error for nil client")
	}
}

func TestWithListTimeoutAddsDeadline(t *testing.T) {
	t.Parallel()

	ctx, cancel := withListTimeout(context.Background())
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("withListTimeout() expected deadline")
	}

	remaining := time.Until(deadline)
	if remaining <= 0 || remaining > kubernetesListTimeout {
		t.Fatalf("withListTimeout() deadline remaining = %s, want within (0, %s]", remaining, kubernetesListTimeout)
	}
}

func TestWithListTimeoutUsesParentSoonerDeadline(t *testing.T) {
	t.Parallel()

	parent, cancelParent := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancelParent()

	parentDeadline, _ := parent.Deadline()
	ctx, cancel := withListTimeout(parent)
	defer cancel()

	gotDeadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("withListTimeout() expected deadline")
	}
	if gotDeadline.After(parentDeadline) {
		t.Fatalf("withListTimeout() deadline = %s, want <= parent deadline %s", gotDeadline, parentDeadline)
	}
}
