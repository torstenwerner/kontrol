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

var (
	newClientFromKubeconfig = k8s.NewClientFromKubeconfig
	contextNamespace        = k8s.ContextNamespace
)

const namespaceFallbackHint = "could not fetch namespaces; using kubeconfig namespace"

type runtimeState struct {
	currentContext   string
	currentNamespace string
	namespacesByCtx  map[string]string
	contexts         []string
	namespaces       []string
	client           *k8s.Client
	mockPods         []k8s.PodRow
	pendingErr       error
}

func main() {
	log.SetOutput(os.Stderr)
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	rt := bootstrapRuntime()
	model := ui.NewModel(
		ui.WithRefreshCmd(func() tea.Cmd {
			return refreshViewCmd(rt)
		}),
		ui.WithContextSelectedCmd(func(selected string) tea.Cmd {
			return contextSelectedCmd(rt, selected)
		}),
		ui.WithNamespaceSelectedCmd(func(selected string) tea.Cmd {
			return namespaceSelectedCmd(rt, selected)
		}),
	)

	program := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		log.Printf("run Bubble Tea program: %+v", err)
		os.Exit(1)
	}

	if err := rt.persistConfig(); err != nil {
		log.Printf("persist config on quit (context=%q namespace=%q): %+v", rt.currentContext, rt.currentNamespace, err)
	}
}

func bootstrapRuntime() *runtimeState {
	if mockEnabled() {
		return mockRuntime()
	}

	rt := &runtimeState{}

	cfg, err := config.Load()
	if err != nil {
		log.Printf("load persisted config: %+v", err)
		rt.pendingErr = errors.New("load saved preferences failed")
	}
	rt.namespacesByCtx = map[string]string{}
	for contextName, namespace := range cfg.NamespacesByContext {
		rt.namespacesByCtx[contextName] = namespace
	}

	contexts, err := k8s.ListContexts("")
	if err != nil {
		log.Printf("list kubeconfig contexts: %+v", err)
		rt.pendingErr = firstErr(rt.pendingErr, errors.New("load Kubernetes contexts failed; check kubeconfig"))
	}
	rt.contexts = contexts
	kubeCurrent, currentErr := k8s.CurrentContext("")
	if currentErr != nil {
		log.Printf("resolve kubeconfig current context: %+v", currentErr)
		rt.pendingErr = firstErr(rt.pendingErr, errors.New("resolve current Kubernetes context failed; check kubeconfig"))
	}
	rt.currentContext = resolveContext(cfg.Context, kubeCurrent, contexts)

	rt.applyContext(rt.currentContext)
	return rt
}

func (rt *runtimeState) applyContext(selected string) {
	savedNamespace := rt.namespaceForContext(selected)

	if len(rt.mockPods) > 0 {
		if selected == "" {
			return
		}
		rt.currentContext = selected
		rt.currentNamespace = resolveNamespace(savedNamespace, rt.namespaces)
		rt.rememberNamespace()
		_ = rt.persistConfig()
		return
	}

	if selected == "" {
		rt.client = nil
		rt.namespaces = nil
		rt.currentContext = ""
		rt.currentNamespace = resolveNamespace(savedNamespace, nil)
		return
	}

	client, err := newClientFromKubeconfig("", selected)
	if err != nil {
		log.Printf("create Kubernetes client for context %q: %+v", selected, err)
		rt.client = nil
		rt.namespaces = nil
		rt.currentContext = selected
		rt.currentNamespace = resolveNamespace(savedNamespace, nil)
		rt.rememberNamespace()
		rt.pendingErr = firstErr(rt.pendingErr, errors.New("connect selected context failed"))
		_ = rt.persistConfig()
		return
	}

	namespaces, err := client.ListNamespaces(context.Background())
	if err != nil {
		log.Printf("list namespaces for context %q: %+v", selected, err)
		fallbackNamespace, fallbackErr := contextNamespace("", selected)
		if fallbackErr != nil {
			log.Printf("resolve kubeconfig namespace for context %q: %+v", selected, fallbackErr)
			fallbackNamespace = "default"
		}
		log.Printf("using kubeconfig namespace fallback for context %q: %q", selected, fallbackNamespace)
		rt.client = client
		rt.namespaces = nil
		rt.currentContext = selected
		rt.currentNamespace = resolveNamespace(fallbackNamespace, nil)
		rt.rememberNamespace()
		rt.pendingErr = firstErr(rt.pendingErr, errors.New(namespaceFallbackHint))
		_ = rt.persistConfig()
		return
	}

	rt.client = client
	rt.namespaces = namespaces
	rt.currentContext = selected
	rt.currentNamespace = resolveNamespace(savedNamespace, namespaces)
	rt.rememberNamespace()
	_ = rt.persistConfig()
}

func (rt *runtimeState) applyNamespace(selected string) {
	if selected == "" {
		return
	}
	rt.currentNamespace = selected
	rt.rememberNamespace()
	_ = rt.persistConfig()
}

func (rt *runtimeState) refreshPods() ([]k8s.PodRow, error) {
	uiErr := rt.pendingErr
	rt.pendingErr = nil

	if len(rt.mockPods) > 0 {
		return append([]k8s.PodRow(nil), rt.mockPods...), uiErr
	}

	if rt.client == nil {
		if uiErr == nil {
			uiErr = errors.New("load pods failed: connect a context first")
		}
		return nil, uiErr
	}

	pods, err := rt.client.ListPods(context.Background(), rt.currentNamespace)
	if err != nil {
		log.Printf("list pods (context=%q namespace=%q): %+v", rt.currentContext, rt.currentNamespace, err)
		if uiErr == nil {
			uiErr = errors.New("load pods for current namespace failed")
		}
		return nil, uiErr
	}
	return pods, uiErr
}

func (rt *runtimeState) persistConfig() error {
	err := config.Save(config.Config{
		Context:             rt.currentContext,
		Namespace:           rt.currentNamespace,
		NamespacesByContext: rt.namespacesByCtx,
	})
	if err != nil {
		log.Printf("persist config (context=%q namespace=%q): %+v", rt.currentContext, rt.currentNamespace, err)
		rt.pendingErr = firstErr(rt.pendingErr, errors.New("save preferences failed"))
	}
	return err
}

func (rt *runtimeState) namespaceForContext(contextName string) string {
	if rt == nil || contextName == "" {
		return ""
	}
	return rt.namespacesByCtx[contextName]
}

func (rt *runtimeState) rememberNamespace() {
	if rt == nil || rt.currentContext == "" || rt.currentNamespace == "" {
		return
	}
	if rt.namespacesByCtx == nil {
		rt.namespacesByCtx = map[string]string{}
	}
	rt.namespacesByCtx[rt.currentContext] = rt.currentNamespace
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

func refreshViewCmd(rt *runtimeState) tea.Cmd {
	return tea.Batch(metadataCmd(rt), podsCmd(rt))
}

func contextSelectedCmd(rt *runtimeState, selected string) tea.Cmd {
	rt.applyContext(selected)
	return tea.Batch(metadataCmd(rt), podsCmd(rt))
}

func namespaceSelectedCmd(rt *runtimeState, selected string) tea.Cmd {
	rt.applyNamespace(selected)
	return tea.Batch(namespaceCmd(rt), podsCmd(rt))
}

func metadataCmd(rt *runtimeState) tea.Cmd {
	return tea.Batch(contextCmd(rt), namespaceCmd(rt))
}

func contextCmd(rt *runtimeState) tea.Cmd {
	return cmdForMsg(ui.ContextsUpdatedMsg{
		Items:   append([]string(nil), rt.contexts...),
		Current: rt.currentContext,
	})
}

func namespaceCmd(rt *runtimeState) tea.Cmd {
	return cmdForMsg(ui.NamespacesUpdatedMsg{
		Items:   append([]string(nil), rt.namespaces...),
		Current: rt.currentNamespace,
	})
}

func podsCmd(rt *runtimeState) tea.Cmd {
	return func() tea.Msg {
		pods, err := rt.refreshPods()
		return ui.PodsUpdatedMsg{Pods: toUIPods(pods), RefreshedAt: time.Now(), Err: err}
	}
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

func mockEnabled() bool {
	return os.Getenv("KONTROL_MOCK_DATA") == "1"
}

func mockRuntime() *runtimeState {
	pods := []k8s.PodRow{
		{Name: "api-7d4f6dbf5f-4m2rk", Status: "Running", Ready: "2/2", Restarts: "0", Age: "12m", IP: "10.42.1.11", Node: "worker-a", Labels: "app=api,team=platform"},
		{Name: "api-7d4f6dbf5f-l2nxd", Status: "Running", Ready: "2/2", Restarts: "1", Age: "9m", IP: "10.42.1.12", Node: "worker-b", Labels: "app=api,team=platform"},
		{Name: "web-5bc9c6f9cf-b9ksp", Status: "Running", Ready: "1/1", Restarts: "0", Age: "15m", IP: "10.42.2.20", Node: "worker-b", Labels: "app=web,tier=frontend"},
		{Name: "web-5bc9c6f9cf-mj8qp", Status: "Running", Ready: "1/1", Restarts: "0", Age: "15m", IP: "10.42.2.21", Node: "worker-a", Labels: "app=web,tier=frontend"},
		{Name: "worker-7f55d9b96d-2hg7k", Status: "Running", Ready: "1/1", Restarts: "0", Age: "42m", IP: "10.42.3.30", Node: "worker-c", Labels: "app=worker,queue=default"},
		{Name: "worker-7f55d9b96d-j6nrz", Status: "Running", Ready: "1/1", Restarts: "2", Age: "41m", IP: "10.42.3.31", Node: "worker-c", Labels: "app=worker,queue=default"},
		{Name: "worker-7f55d9b96d-v4mqp", Status: "CrashLoopBackOff", Ready: "0/1", Restarts: "6", Age: "40m", IP: "10.42.3.32", Node: "worker-b", Labels: "app=worker,queue=priority"},
		{Name: "payments-66b57bd96f-5r8tg", Status: "Pending", Ready: "0/2", Restarts: "0", Age: "3m", IP: "", Node: "worker-a", Labels: "app=payments,team=finance"},
		{Name: "payments-66b57bd96f-9vckm", Status: "Running", Ready: "2/2", Restarts: "0", Age: "3m", IP: "10.42.4.40", Node: "worker-a", Labels: "app=payments,team=finance"},
		{Name: "redis-0", Status: "Running", Ready: "1/1", Restarts: "0", Age: "2h", IP: "10.42.5.50", Node: "worker-c", Labels: "app=redis,role=cache"},
		{Name: "reporting-5c8f5d9674-6mmkc", Status: "Error", Ready: "0/1", Restarts: "3", Age: "18m", IP: "10.42.6.60", Node: "worker-b", Labels: "app=reporting"},
		{Name: "batch-1700001-abcd", Status: "Completed", Ready: "0/1", Restarts: "0", Age: "1h", IP: "", Node: "worker-a", Labels: "job=batch"},
		{Name: "batch-1700002-efgh", Status: "Failed", Ready: "0/1", Restarts: "1", Age: "58m", IP: "", Node: "worker-a", Labels: "job=batch"},
	}

	return &runtimeState{
		currentContext:   "mock-dev",
		currentNamespace: "default",
		namespacesByCtx: map[string]string{
			"mock-dev": "default",
		},
		contexts:   []string{"mock-dev", "mock-stage", "mock-prod"},
		namespaces: []string{"default", "kube-system", "payments", "monitoring"},
		mockPods:   pods,
	}
}
