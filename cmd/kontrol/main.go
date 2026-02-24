package main

import (
	"context"
	"errors"
	"log"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"kontrol/internal/config"
	"kontrol/internal/k8s"
	"kontrol/internal/ui"
)

type runtimeState struct {
	currentContext   string
	currentNamespace string
	contexts         []string
	namespaces       []string
	client           *k8s.Client
	pendingErr       error
}

func main() {
	log.SetOutput(os.Stderr)
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	rt := bootstrapRuntime()
	model := ui.NewModel(
		ui.WithRefreshCmd(func() tea.Cmd {
			pods, err := rt.refreshPods()
			return tea.Batch(
				cmdForMsg(ui.ContextsUpdatedMsg{Items: append([]string(nil), rt.contexts...), Current: rt.currentContext}),
				cmdForMsg(ui.NamespacesUpdatedMsg{Items: append([]string(nil), rt.namespaces...), Current: rt.currentNamespace}),
				cmdForMsg(ui.PodsUpdatedMsg{Pods: toUIPods(pods), RefreshedAt: time.Now(), Err: err}),
			)
		}),
		ui.WithContextSelectedCmd(func(selected string) tea.Cmd {
			rt.applyContext(selected)
			pods, err := rt.refreshPods()
			return tea.Batch(
				cmdForMsg(ui.ContextsUpdatedMsg{Items: append([]string(nil), rt.contexts...), Current: rt.currentContext}),
				cmdForMsg(ui.NamespacesUpdatedMsg{Items: append([]string(nil), rt.namespaces...), Current: rt.currentNamespace}),
				cmdForMsg(ui.PodsUpdatedMsg{Pods: toUIPods(pods), RefreshedAt: time.Now(), Err: err}),
			)
		}),
		ui.WithNamespaceSelectedCmd(func(selected string) tea.Cmd {
			rt.applyNamespace(selected)
			pods, err := rt.refreshPods()
			return tea.Batch(
				cmdForMsg(ui.NamespacesUpdatedMsg{Items: append([]string(nil), rt.namespaces...), Current: rt.currentNamespace}),
				cmdForMsg(ui.PodsUpdatedMsg{Pods: toUIPods(pods), RefreshedAt: time.Now(), Err: err}),
			)
		}),
	)

	program := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		log.Printf("run Bubble Tea program: %+v", err)
		os.Exit(1)
	}

	if err := config.Save(config.Config{Context: rt.currentContext, Namespace: rt.currentNamespace}); err != nil {
		log.Printf("persist config on quit (context=%q namespace=%q): %+v", rt.currentContext, rt.currentNamespace, err)
	}
}

func bootstrapRuntime() *runtimeState {
	rt := &runtimeState{}

	cfg, err := config.Load()
	if err != nil {
		log.Printf("load persisted config: %+v", err)
		rt.pendingErr = errors.New("could not load saved preferences")
	}

	contexts, err := k8s.ListContexts("")
	if err != nil {
		log.Printf("list kubeconfig contexts: %+v", err)
		rt.pendingErr = firstErr(rt.pendingErr, errors.New("unable to load Kubernetes contexts"))
	}
	rt.contexts = contexts
	kubeCurrent, currentErr := k8s.CurrentContext("")
	if currentErr != nil {
		log.Printf("resolve kubeconfig current context: %+v", currentErr)
		rt.pendingErr = firstErr(rt.pendingErr, errors.New("no Kubernetes context available"))
	}
	rt.currentContext = resolveContext(cfg.Context, kubeCurrent, contexts)

	rt.applyContext(rt.currentContext)
	rt.currentNamespace = resolveNamespace(cfg.Namespace, rt.namespaces)
	return rt
}

func (rt *runtimeState) applyContext(selected string) {
	if selected == "" {
		rt.client = nil
		rt.namespaces = nil
		rt.currentContext = ""
		rt.currentNamespace = resolveNamespace(rt.currentNamespace, nil)
		return
	}

	client, err := k8s.NewClientFromKubeconfig("", selected)
	if err != nil {
		log.Printf("create Kubernetes client for context %q: %+v", selected, err)
		rt.client = nil
		rt.namespaces = nil
		rt.currentContext = selected
		rt.currentNamespace = resolveNamespace(rt.currentNamespace, nil)
		rt.pendingErr = firstErr(rt.pendingErr, errors.New("unable to connect to selected context"))
		_ = rt.persistConfig()
		return
	}

	namespaces, err := client.ListNamespaces(context.Background())
	if err != nil {
		log.Printf("list namespaces for context %q: %+v", selected, err)
		rt.client = client
		rt.namespaces = nil
		rt.currentContext = selected
		rt.currentNamespace = resolveNamespace(rt.currentNamespace, nil)
		rt.pendingErr = firstErr(rt.pendingErr, errors.New("unable to load namespaces"))
		_ = rt.persistConfig()
		return
	}

	rt.client = client
	rt.namespaces = namespaces
	rt.currentContext = selected
	rt.currentNamespace = resolveNamespace(rt.currentNamespace, namespaces)
	_ = rt.persistConfig()
}

func (rt *runtimeState) applyNamespace(selected string) {
	if selected == "" {
		return
	}
	rt.currentNamespace = selected
	_ = rt.persistConfig()
}

func (rt *runtimeState) refreshPods() ([]k8s.PodRow, error) {
	uiErr := rt.pendingErr
	rt.pendingErr = nil

	if rt.client == nil {
		if uiErr == nil {
			uiErr = errors.New("Kubernetes client not initialized")
		}
		return nil, uiErr
	}

	pods, err := rt.client.ListPods(context.Background(), rt.currentNamespace)
	if err != nil {
		log.Printf("list pods (context=%q namespace=%q): %+v", rt.currentContext, rt.currentNamespace, err)
		if uiErr == nil {
			uiErr = errors.New("unable to load pods")
		}
		return nil, uiErr
	}
	return pods, uiErr
}

func (rt *runtimeState) persistConfig() error {
	err := config.Save(config.Config{
		Context:   rt.currentContext,
		Namespace: rt.currentNamespace,
	})
	if err != nil {
		log.Printf("persist config (context=%q namespace=%q): %+v", rt.currentContext, rt.currentNamespace, err)
		rt.pendingErr = firstErr(rt.pendingErr, errors.New("could not save preferences"))
	}
	return err
}

func resolveContext(saved, kubeCurrent string, contexts []string) string {
	if saved != "" && contains(contexts, saved) {
		return saved
	}
	if kubeCurrent != "" && (len(contexts) == 0 || contains(contexts, kubeCurrent)) {
		return kubeCurrent
	}
	if len(contexts) == 0 {
		return ""
	}
	return contexts[0]
}

func resolveNamespace(saved string, namespaces []string) string {
	if saved != "" && (len(namespaces) == 0 || contains(namespaces, saved)) {
		return saved
	}
	if contains(namespaces, "default") {
		return "default"
	}
	if len(namespaces) > 0 {
		return namespaces[0]
	}
	return "default"
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func toUIPods(rows []k8s.PodRow) []ui.PodRow {
	out := make([]ui.PodRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, ui.PodRow{
			Name:     row.Name,
			Status:   row.Status,
			Ready:    row.Ready,
			Restarts: row.Restarts,
			Age:      row.Age,
			IP:       row.IP,
			Node:     row.Node,
			Labels:   row.Labels,
		})
	}
	return out
}

func firstErr(current, next error) error {
	if current != nil {
		return current
	}
	return next
}

func cmdForMsg(msg tea.Msg) tea.Cmd {
	return func() tea.Msg {
		return msg
	}
}
