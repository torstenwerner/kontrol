package main

import (
	"context"
	"errors"
	"log"
	"os"
	"slices"
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
	rt := &runtimeState{}

	cfg, err := config.Load()
	if err != nil {
		log.Printf("load persisted config: %+v", err)
		rt.setPendingErr("load saved preferences failed")
	}
	rt.namespacesByCtx = map[string]string{}
	for contextName, namespace := range cfg.NamespacesByContext {
		rt.namespacesByCtx[contextName] = namespace
	}

	contexts, err := k8s.ListContexts("")
	if err != nil {
		log.Printf("list kubeconfig contexts: %+v", err)
		rt.setPendingErr("load Kubernetes contexts failed; check kubeconfig")
	}
	rt.contexts = contexts
	kubeCurrent, currentErr := k8s.CurrentContext("")
	if currentErr != nil {
		log.Printf("resolve kubeconfig current context: %+v", currentErr)
		rt.setPendingErr("resolve current Kubernetes context failed; check kubeconfig")
	}
	rt.currentContext = resolveContext(cfg.Context, kubeCurrent, contexts)

	rt.applyContext(rt.currentContext)
	return rt
}

func (rt *runtimeState) applyContext(selected string) {
	savedNamespace := rt.namespaceForContext(selected)
	persist := func() {
		rt.rememberNamespace()
		_ = rt.persistConfig()
	}

	rt.currentContext, rt.currentNamespace = selected, resolveNamespace(savedNamespace, nil)
	if selected == "" {
		rt.client, rt.namespaces = nil, nil
		return
	}

	client, err := newClientFromKubeconfig("", selected)
	if err != nil {
		log.Printf("create Kubernetes client for context %q: %+v", selected, err)
		rt.client, rt.namespaces = nil, nil
		rt.setPendingErr("connect selected context failed")
		persist()
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
		rt.client, rt.namespaces = client, nil
		rt.currentNamespace = resolveNamespace(fallbackNamespace, nil)
		rt.setPendingErr(namespaceFallbackHint)
		persist()
		return
	}

	rt.client, rt.namespaces = client, namespaces
	rt.currentNamespace = resolveNamespace(savedNamespace, namespaces)
	persist()
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
		rt.setPendingErr("save preferences failed")
	}
	return err
}

func (rt *runtimeState) setPendingErr(msg string) {
	if rt.pendingErr == nil {
		rt.pendingErr = errors.New(msg)
	}
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
	if saved != "" && slices.Contains(contexts, saved) {
		return saved
	}
	if kubeCurrent != "" && (len(contexts) == 0 || slices.Contains(contexts, kubeCurrent)) {
		return kubeCurrent
	}
	if len(contexts) == 0 {
		return ""
	}
	return contexts[0]
}

func resolveNamespace(saved string, namespaces []string) string {
	if saved != "" && (len(namespaces) == 0 || slices.Contains(namespaces, saved)) {
		return saved
	}
	if slices.Contains(namespaces, "default") {
		return "default"
	}
	if len(namespaces) > 0 {
		return namespaces[0]
	}
	return "default"
}

func toUIPods(rows []k8s.PodRow) []ui.PodRow {
	out := make([]ui.PodRow, len(rows))
	for i, row := range rows {
		out[i] = ui.PodRow(row)
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
	msg := ui.ContextsUpdatedMsg{
		Items:   append([]string(nil), rt.contexts...),
		Current: rt.currentContext,
	}
	return func() tea.Msg { return msg }
}

func namespaceCmd(rt *runtimeState) tea.Cmd {
	msg := ui.NamespacesUpdatedMsg{
		Items:   append([]string(nil), rt.namespaces...),
		Current: rt.currentNamespace,
	}
	return func() tea.Msg { return msg }
}

func podsCmd(rt *runtimeState) tea.Cmd {
	return func() tea.Msg {
		pods, err := rt.refreshPods()
		return ui.PodsUpdatedMsg{Pods: toUIPods(pods), RefreshedAt: time.Now(), Err: err}
	}
}
