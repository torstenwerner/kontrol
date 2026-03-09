package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestKeyFlowContextSelectionUpdatesStateAndFiresCallback(t *testing.T) {
	t.Parallel()

	var selected string
	m := NewModel(WithContextSelectedCmd(func(ctx string) tea.Cmd {
		selected = ctx
		return func() tea.Msg { return ctx }
	}))

	next, _ := m.Update(ContextsUpdatedMsg{
		Items:   []string{"dev", "prod"},
		Current: "dev",
	})
	model := next.(Model)

	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	model = next.(Model)
	if model.modal != modalContext || model.modalIndex != 0 {
		t.Fatalf("expected context modal open at current item, got modal=%v index=%d", model.modal, model.modalIndex)
	}

	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = next.(Model)
	if model.modalIndex != 1 {
		t.Fatalf("expected modal index 1 after down key, got %d", model.modalIndex)
	}

	next, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	if model.modal != modalNone {
		t.Fatalf("expected modal to close after selection, got %v", model.modal)
	}
	if model.currentContext != "prod" {
		t.Fatalf("expected current context to be updated, got %q", model.currentContext)
	}
	if selected != "prod" {
		t.Fatalf("expected callback selected context %q, got %q", "prod", selected)
	}
	if cmd == nil {
		t.Fatal("expected non-nil command from context selection")
	}
	if msg := cmd(); msg != "prod" {
		t.Fatalf("expected command message to include selected context, got %v", msg)
	}
}

func TestKeyFlowNamespaceModalEscResetsState(t *testing.T) {
	t.Parallel()

	m := NewModel()
	next, _ := m.Update(NamespacesUpdatedMsg{
		Items:   []string{"default", "team-a"},
		Current: "default",
	})
	model := next.(Model)

	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	model = next.(Model)
	if model.modal != modalNamespace {
		t.Fatalf("expected namespace modal, got %v", model.modal)
	}

	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = next.(Model)
	if model.modalIndex != 1 {
		t.Fatalf("expected modal index 1, got %d", model.modalIndex)
	}

	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = next.(Model)
	if model.modal != modalNone {
		t.Fatalf("expected modal closed on esc, got %v", model.modal)
	}
	if model.modalIndex != 0 {
		t.Fatalf("expected modal index reset to 0, got %d", model.modalIndex)
	}
}

func TestPodsUpdatedMsgZeroRefreshTimeSetsTimestamp(t *testing.T) {
	t.Parallel()

	m := NewModel()
	next, _ := m.Update(PodsUpdatedMsg{
		Pods: []PodRow{{Name: "pod-a"}},
	})
	model := next.(Model)
	if model.lastRefresh.IsZero() {
		t.Fatal("expected non-zero refresh timestamp")
	}
	if len(model.pods) != 1 || model.pods[0].Name != "pod-a" {
		t.Fatalf("expected pod rows to update, got %+v", model.pods)
	}
}

func TestPodsUpdatedMsgUsesProvidedTimestampAndError(t *testing.T) {
	t.Parallel()

	m := NewModel()
	ts := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	next, _ := m.Update(PodsUpdatedMsg{
		RefreshedAt: ts,
		Err:         assertErr("boom"),
	})
	model := next.(Model)
	if !model.lastRefresh.Equal(ts) {
		t.Fatalf("expected provided refresh timestamp, got %v", model.lastRefresh)
	}
	if model.err == nil || model.err.Error() != "boom" {
		t.Fatalf("expected error to be retained, got %v", model.err)
	}
}

type assertErr string

func (e assertErr) Error() string { return string(e) }

func TestManualRefreshWorksWhileModalOpen(t *testing.T) {
	t.Parallel()

	m := NewModel(WithRefreshCmd(func() tea.Cmd {
		return func() tea.Msg { return "manual-refresh" }
	}))
	m.modal = modalContext

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	model := next.(Model)
	if model.modal != modalContext {
		t.Fatalf("expected modal to stay open, got %v", model.modal)
	}
	if cmd == nil {
		t.Fatal("expected refresh command")
	}
	if msg := cmd(); msg != "manual-refresh" {
		t.Fatalf("expected manual refresh command to run, got %v", msg)
	}
}

func TestTickDoesNotAutoRefreshWhileModalOpen(t *testing.T) {
	t.Parallel()

	m := NewModel(WithRefreshCmd(func() tea.Cmd {
		return func() tea.Msg { return "tick-refresh" }
	}))
	m.modal = modalNamespace

	next, cmd := m.Update(refreshTickMsg{})
	model := next.(Model)
	if model.modal != modalNamespace {
		t.Fatalf("expected modal to stay open, got %v", model.modal)
	}
	if cmd == nil {
		t.Fatal("expected tick command")
	}
	msg := cmd()
	if _, ok := msg.(refreshTickMsg); !ok {
		t.Fatalf("expected refreshTickMsg while modal open, got %T", msg)
	}
}

func TestTickTriggersRefreshWhenNoModalOpen(t *testing.T) {
	t.Parallel()

	m := NewModel(WithRefreshCmd(func() tea.Cmd {
		return func() tea.Msg { return "tick-refresh" }
	}))

	_, cmd := m.Update(refreshTickMsg{})
	if cmd == nil {
		t.Fatal("expected batch command")
	}
	msg := cmd()
	if _, ok := msg.(tea.BatchMsg); !ok {
		t.Fatalf("expected tea.BatchMsg from tick, got %T", msg)
	}
}

func TestTinyWindowSizeDoesNotCollapseBody(t *testing.T) {
	t.Parallel()

	m := NewModel()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := next.(Model)
	next, _ = model.Update(tea.WindowSizeMsg{Width: 120, Height: 3})
	model = next.(Model)

	if model.height <= 6 {
		t.Fatalf("expected height to remain usable, got %d", model.height)
	}
}

func TestViewContainsPodRowsAfterUpdate(t *testing.T) {
	t.Parallel()

	m := NewModel()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	model := next.(Model)
	next, _ = model.Update(PodsUpdatedMsg{
		Pods: []PodRow{{Name: "mock-pod-a", Status: "Running", Ready: "1/1", Restarts: "0", Age: "1m", IP: "10.0.0.1", Node: "node-a", Labels: "app=demo"}},
	})
	model = next.(Model)

	view := model.View()
	if !strings.Contains(view, "mock-pod-a") {
		t.Fatalf("expected rendered view to include pod row, got: %q", view)
	}
}

func TestOutlierWindowHeightDoesNotCreateHugeBlankBody(t *testing.T) {
	t.Parallel()

	m := NewModel()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 9999})
	model := next.(Model)
	next, _ = model.Update(PodsUpdatedMsg{
		Pods: []PodRow{{Name: "mock-pod-a", Status: "Running", Ready: "1/1", Restarts: "0", Age: "1m", IP: "10.0.0.1", Node: "node-a", Labels: "app=demo"}},
	})
	model = next.(Model)

	view := model.View()
	if !strings.Contains(view, "mock-pod-a") {
		t.Fatalf("expected rendered view to include pod row, got: %q", view)
	}
	if strings.Count(view, "\n") > 120 {
		t.Fatalf("expected compact view output, got too many lines: %d", strings.Count(view, "\n"))
	}
}

func TestNamespaceModalViewUsesScrollableWindow(t *testing.T) {
	t.Parallel()

	m := NewModel()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 12})
	model := next.(Model)
	next, _ = model.Update(NamespacesUpdatedMsg{
		Items:   numberedItems("team", 20),
		Current: "team-18",
	})
	model = next.(Model)

	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	model = next.(Model)

	view := model.View()
	if !strings.Contains(view, "team-18") {
		t.Fatalf("expected selected namespace to stay visible, got: %q", view)
	}
	if strings.Contains(view, "team-01") {
		t.Fatalf("expected modal to render a bounded window, got: %q", view)
	}
	if !strings.Contains(view, "15-20 of 20") {
		t.Fatalf("expected modal footer to describe visible range, got: %q", view)
	}
}

func TestNamespaceModalNavigationScrollsVisibleWindow(t *testing.T) {
	t.Parallel()

	m := NewModel()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 12})
	model := next.(Model)
	next, _ = model.Update(NamespacesUpdatedMsg{
		Items:   numberedItems("team", 20),
		Current: "team-01",
	})
	model = next.(Model)

	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	model = next.(Model)
	for range 10 {
		next, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
		model = next.(Model)
	}

	view := model.View()
	if model.modalIndex != 10 {
		t.Fatalf("expected modal index 10 after navigation, got %d", model.modalIndex)
	}
	if !strings.Contains(view, "team-11") {
		t.Fatalf("expected navigated namespace to be visible, got: %q", view)
	}
	if strings.Contains(view, "team-01") {
		t.Fatalf("expected early namespaces to scroll out of the visible window, got: %q", view)
	}
}

func TestModalRenderStaysWithinViewport(t *testing.T) {
	t.Parallel()

	m := NewModel()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 18})
	model := next.(Model)
	next, _ = model.Update(PodsUpdatedMsg{
		Pods: []PodRow{{Name: "mock-pod-a", Status: "Running", Ready: "1/1", Restarts: "0", Age: "1m", IP: "10.0.0.1", Node: "node-a", Labels: "app=demo"}},
	})
	model = next.(Model)
	next, _ = model.Update(NamespacesUpdatedMsg{
		Items:   numberedItems("team", 20),
		Current: "team-10",
	})
	model = next.(Model)

	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	model = next.(Model)

	view := model.View()
	if lines := strings.Count(view, "\n") + 1; lines > 18 {
		t.Fatalf("expected modal render to stay within viewport, got %d lines", lines)
	}
	if !strings.Contains(view, "Select namespace") {
		t.Fatalf("expected modal title in view, got: %q", view)
	}
}

func numberedItems(prefix string, count int) []string {
	items := make([]string, 0, count)
	for i := 1; i <= count; i++ {
		items = append(items, fmt.Sprintf("%s-%02d", prefix, i))
	}
	return items
}
