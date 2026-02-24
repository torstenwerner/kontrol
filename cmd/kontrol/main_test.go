package main

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"kontrol/internal/k8s"
	"kontrol/internal/ui"
)

func TestRefreshViewCmdPublishesMetadataWithoutWaitingForPods(t *testing.T) {
	t.Parallel()

	blockPods := make(chan struct{})
	clientset := k8sfake.NewSimpleClientset()
	clientset.PrependReactor("list", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		<-blockPods
		return true, &corev1.PodList{Items: []corev1.Pod{{ObjectMeta: metav1.ObjectMeta{Name: "pod-a"}}}}, nil
	})

	rt := &runtimeState{
		contexts:         []string{"dev", "prod"},
		namespaces:       []string{"default"},
		currentContext:   "dev",
		currentNamespace: "default",
		client:           k8s.NewClient(clientset),
	}

	cmd := refreshViewCmd(rt)
	msg := runCmdWithTimeout(t, cmd, 100*time.Millisecond)
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected tea.BatchMsg, got %T", msg)
	}
	if len(batch) != 2 {
		t.Fatalf("expected 2 batched commands, got %d", len(batch))
	}

	metadata := runCmdWithTimeout(t, batch[0], 100*time.Millisecond)
	metadataBatch, ok := metadata.(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected metadata batch, got %T", metadata)
	}
	if len(metadataBatch) != 2 {
		t.Fatalf("expected 2 metadata commands, got %d", len(metadataBatch))
	}

	contextMsg := runCmdWithTimeout(t, metadataBatch[0], 100*time.Millisecond)
	if _, ok := contextMsg.(ui.ContextsUpdatedMsg); !ok {
		t.Fatalf("expected contexts update message, got %T", contextMsg)
	}
	namespaceMsg := runCmdWithTimeout(t, metadataBatch[1], 100*time.Millisecond)
	if _, ok := namespaceMsg.(ui.NamespacesUpdatedMsg); !ok {
		t.Fatalf("expected namespaces update message, got %T", namespaceMsg)
	}

	close(blockPods)
	_ = runCmdWithTimeout(t, batch[1], 100*time.Millisecond)
}

func TestRefreshViewCmdPublishesMetadataWhenPodsFail(t *testing.T) {
	t.Parallel()

	clientset := k8sfake.NewSimpleClientset()
	clientset.PrependReactor("list", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("boom")
	})

	rt := &runtimeState{
		contexts:         []string{"dev", "prod"},
		namespaces:       []string{"default"},
		currentContext:   "dev",
		currentNamespace: "default",
		client:           k8s.NewClient(clientset),
	}

	cmd := refreshViewCmd(rt)
	msg := runCmdWithTimeout(t, cmd, 100*time.Millisecond)
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected tea.BatchMsg, got %T", msg)
	}
	if len(batch) != 2 {
		t.Fatalf("expected 2 batched commands, got %d", len(batch))
	}

	metadata := runCmdWithTimeout(t, batch[0], 100*time.Millisecond)
	metadataBatch, ok := metadata.(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected metadata batch, got %T", metadata)
	}
	if len(metadataBatch) != 2 {
		t.Fatalf("expected 2 metadata commands, got %d", len(metadataBatch))
	}

	if _, ok := runCmdWithTimeout(t, metadataBatch[0], 100*time.Millisecond).(ui.ContextsUpdatedMsg); !ok {
		t.Fatalf("expected contexts update message")
	}
	if _, ok := runCmdWithTimeout(t, metadataBatch[1], 100*time.Millisecond).(ui.NamespacesUpdatedMsg); !ok {
		t.Fatalf("expected namespaces update message")
	}

	podsMsg, ok := runCmdWithTimeout(t, batch[1], 100*time.Millisecond).(ui.PodsUpdatedMsg)
	if !ok {
		t.Fatalf("expected pods update message")
	}
	if podsMsg.Err == nil || podsMsg.Err.Error() != "load pods for current namespace failed" {
		t.Fatalf("expected actionable pods error, got %v", podsMsg.Err)
	}
}

func TestResolveContextFallsBackWhenSavedContextIsStale(t *testing.T) {
	t.Parallel()

	contexts := []string{"dev", "prod"}
	if got := resolveContext("stale", "prod", contexts); got != "prod" {
		t.Fatalf("expected kube current context fallback, got %q", got)
	}
	if got := resolveContext("stale", "missing", contexts); got != "dev" {
		t.Fatalf("expected first available context fallback, got %q", got)
	}
}

func TestResolveNamespaceFallsBackWhenSavedNamespaceIsStale(t *testing.T) {
	t.Parallel()

	if got := resolveNamespace("stale", []string{"team-a", "default"}); got != "default" {
		t.Fatalf("expected default namespace fallback, got %q", got)
	}
	if got := resolveNamespace("stale", []string{"team-a", "team-b"}); got != "team-a" {
		t.Fatalf("expected first available namespace fallback, got %q", got)
	}
}

func TestRefreshPodsWithoutClientReturnsActionableError(t *testing.T) {
	t.Parallel()

	rt := &runtimeState{}
	pods, err := rt.refreshPods()
	if err == nil {
		t.Fatal("expected error when client is not initialized")
	}
	if err.Error() != "load pods failed: connect a context first" {
		t.Fatalf("unexpected error: %v", err)
	}
	if pods != nil {
		t.Fatalf("expected nil pods, got %+v", pods)
	}
}

func TestRefreshPodsListFailureReturnsActionableError(t *testing.T) {
	t.Parallel()

	clientset := k8sfake.NewSimpleClientset()
	clientset.PrependReactor("list", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("boom")
	})

	rt := &runtimeState{
		currentContext:   "dev",
		currentNamespace: "default",
		client:           k8s.NewClient(clientset),
	}

	pods, err := rt.refreshPods()
	if err == nil {
		t.Fatal("expected list pods error")
	}
	if err.Error() != "load pods for current namespace failed" {
		t.Fatalf("unexpected error: %v", err)
	}
	if pods != nil {
		t.Fatalf("expected nil pods, got %+v", pods)
	}
}

func runCmdWithTimeout(t *testing.T, cmd tea.Cmd, timeout time.Duration) tea.Msg {
	t.Helper()

	ch := make(chan tea.Msg, 1)
	go func() {
		ch <- cmd()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	select {
	case msg := <-ch:
		return msg
	case <-ctx.Done():
		t.Fatalf("command did not complete within %s", timeout)
		return nil
	}
}

func TestMockRuntimeWhenEnabled(t *testing.T) {
	t.Setenv("KONTROL_MOCK_DATA", "1")
	rt := bootstrapRuntime()
	if rt.currentContext == "" || rt.currentNamespace == "" {
		t.Fatalf("expected mock context/namespace, got context=%q namespace=%q", rt.currentContext, rt.currentNamespace)
	}
	if len(rt.contexts) == 0 || len(rt.namespaces) == 0 || len(rt.mockPods) == 0 {
		t.Fatalf("expected mock data to be populated, got contexts=%d namespaces=%d pods=%d", len(rt.contexts), len(rt.namespaces), len(rt.mockPods))
	}
	pods, err := rt.refreshPods()
	if err != nil {
		t.Fatalf("refreshPods() unexpected error in mock mode: %v", err)
	}
	if len(pods) != len(rt.mockPods) {
		t.Fatalf("refreshPods() pods=%d, want %d", len(pods), len(rt.mockPods))
	}
}

func TestMockEnabledEnv(t *testing.T) {
	if os.Getenv("KONTROL_MOCK_DATA") == "1" {
		t.Skip("environment already has KONTROL_MOCK_DATA=1")
	}
	if mockEnabled() {
		t.Fatal("mockEnabled() should be false when env var not set to 1")
	}
	t.Setenv("KONTROL_MOCK_DATA", "1")
	if !mockEnabled() {
		t.Fatal("mockEnabled() should be true when env var is 1")
	}
}
