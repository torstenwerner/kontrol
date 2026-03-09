package k8s

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// Client provides read-only Kubernetes list operations for the UI.
type Client struct {
	clientset kubernetes.Interface
}

const kubernetesListTimeout = 5 * time.Second

// PodRow is the UI-ready projection of a Kubernetes pod.
type PodRow struct {
	Name     string
	Status   string
	Ready    string
	Restarts string
	Age      string
	IP       string
	Node     string
	Labels   string
}

// NewClient creates a Client from a Kubernetes interface.
func NewClient(clientset kubernetes.Interface) *Client {
	return &Client{clientset: clientset}
}

// NewClientFromKubeconfig creates a client from kubeconfig and optional context override.
func NewClientFromKubeconfig(kubeconfigPath, contextName string) (*Client, error) {
	overrides := &clientcmd.ConfigOverrides{}
	if contextName != "" {
		overrides.CurrentContext = contextName
	}

	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRulesForPath(kubeconfigPath), overrides)
	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("build Kubernetes REST config: %w", err)
	}

	return NewClientFromRESTConfig(restConfig)
}

// NewClientFromRESTConfig creates a Kubernetes client from a REST config.
func NewClientFromRESTConfig(cfg *rest.Config) (*Client, error) {
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("create Kubernetes clientset: %w", err)
	}
	return NewClient(clientset), nil
}

// ListContexts returns kubeconfig contexts sorted alphabetically.
func ListContexts(kubeconfigPath string) ([]string, error) {
	rawConfig, err := loadRawConfig(kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig contexts: %w", err)
	}

	contexts := make([]string, 0, len(rawConfig.Contexts))
	for name := range rawConfig.Contexts {
		contexts = append(contexts, name)
	}
	sort.Strings(contexts)
	return contexts, nil
}

// CurrentContext returns kubeconfig current-context.
func CurrentContext(kubeconfigPath string) (string, error) {
	rawConfig, err := loadRawConfig(kubeconfigPath)
	if err != nil {
		return "", fmt.Errorf("load kubeconfig current context: %w", err)
	}
	return rawConfig.CurrentContext, nil
}

// ContextNamespace returns the configured namespace for the provided kubeconfig context.
// If the context namespace is empty, "default" is returned.
func ContextNamespace(kubeconfigPath, contextName string) (string, error) {
	rawConfig, err := loadRawConfig(kubeconfigPath)
	if err != nil {
		return "", fmt.Errorf("load kubeconfig context namespace: %w", err)
	}

	resolvedContext := contextName
	if resolvedContext == "" {
		resolvedContext = rawConfig.CurrentContext
	}
	if resolvedContext == "" {
		return "", fmt.Errorf("resolve kubeconfig context namespace: context name is empty")
	}

	kubeContext, ok := rawConfig.Contexts[resolvedContext]
	if !ok {
		return "", fmt.Errorf("resolve kubeconfig context namespace: context %q not found", resolvedContext)
	}
	if kubeContext == nil || kubeContext.Namespace == "" {
		return "default", nil
	}

	return kubeContext.Namespace, nil
}

func loadRawConfig(kubeconfigPath string) (clientcmdapi.Config, error) {
	rawConfig, err := loadingRulesForPath(kubeconfigPath).Load()
	if err != nil {
		return clientcmdapi.Config{}, fmt.Errorf("load kubeconfig: %w", err)
	}
	return *rawConfig, nil
}

func loadingRulesForPath(kubeconfigPath string) *clientcmd.ClientConfigLoadingRules {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfigPath != "" {
		loadingRules.ExplicitPath = kubeconfigPath
	}
	return loadingRules
}

// ListNamespaces returns namespace names sorted alphabetically.
func (c *Client) ListNamespaces(ctx context.Context) ([]string, error) {
	clientset, err := c.clientsetFor("list namespaces")
	if err != nil {
		return nil, err
	}

	listCtx, cancel := withListTimeout(ctx)
	defer cancel()

	nsList, err := clientset.CoreV1().Namespaces().List(listCtx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list namespaces from Kubernetes API: %w", err)
	}

	namespaces := make([]string, 0, len(nsList.Items))
	for _, ns := range nsList.Items {
		namespaces = append(namespaces, ns.Name)
	}
	sort.Strings(namespaces)
	return namespaces, nil
}

// ListPods returns UI-ready pod rows sorted by pod name.
func (c *Client) ListPods(ctx context.Context, namespace string) ([]PodRow, error) {
	clientset, err := c.clientsetFor("list pods")
	if err != nil {
		return nil, err
	}

	listCtx, cancel := withListTimeout(ctx)
	defer cancel()

	podList, err := clientset.CoreV1().Pods(namespace).List(listCtx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list pods in namespace %q: %w", namespace, err)
	}

	now := time.Now()
	rows := make([]PodRow, 0, len(podList.Items))
	for _, pod := range podList.Items {
		rows = append(rows, PodToRow(pod, now))
	}

	sort.Slice(rows, func(i, j int) bool {
		return rows[i].Name < rows[j].Name
	})

	return rows, nil
}

func (c *Client) clientsetFor(op string) (kubernetes.Interface, error) {
	if c == nil || c.clientset == nil {
		return nil, fmt.Errorf("%s: client is not initialized", op)
	}
	return c.clientset, nil
}

func withListTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(ctx, kubernetesListTimeout)
}

// PodToRow maps a pod to UI-ready fields.
func PodToRow(pod corev1.Pod, now time.Time) PodRow {
	return PodRow{
		Name:     pod.Name,
		Status:   PodStatus(pod),
		Ready:    PodReady(pod),
		Restarts: PodRestarts(pod),
		Age:      PodAge(pod, now),
		IP:       pod.Status.PodIP,
		Node:     pod.Spec.NodeName,
		Labels:   LabelsString(pod.Labels),
	}
}

// PodStatus returns a concise pod status suitable for table display.
func PodStatus(pod corev1.Pod) string {
	if pod.DeletionTimestamp != nil {
		return "Terminating"
	}

	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" {
			return cs.State.Waiting.Reason
		}
		if cs.State.Terminated != nil && cs.State.Terminated.Reason != "" {
			return cs.State.Terminated.Reason
		}
	}

	if pod.Status.Reason != "" {
		return pod.Status.Reason
	}
	if pod.Status.Phase != "" {
		return string(pod.Status.Phase)
	}
	return "Unknown"
}

// PodReady returns the ready containers ratio (e.g. "2/3").
func PodReady(pod corev1.Pod) string {
	total := len(pod.Status.ContainerStatuses)
	if total == 0 {
		total = len(pod.Spec.Containers)
	}

	ready := 0
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Ready {
			ready++
		}
	}

	return fmt.Sprintf("%d/%d", ready, total)
}

// PodRestarts returns the total restarts across all containers.
func PodRestarts(pod corev1.Pod) string {
	var restarts int32
	for _, cs := range pod.Status.ContainerStatuses {
		restarts += cs.RestartCount
	}
	return fmt.Sprintf("%d", restarts)
}

// PodAge returns a human-readable pod age.
func PodAge(pod corev1.Pod, now time.Time) string {
	if pod.CreationTimestamp.IsZero() {
		return "0s"
	}
	return FormatAge(now.Sub(pod.CreationTimestamp.Time))
}

// FormatAge formats a duration into Kubernetes-like compact units.
func FormatAge(d time.Duration) string {
	if d < 0 {
		d = 0
	}

	seconds := int64(d.Seconds())
	minutes := int64(d.Minutes())
	hours := int64(d.Hours())
	days := hours / 24

	switch {
	case seconds < 60:
		return fmt.Sprintf("%ds", seconds)
	case minutes < 60:
		return fmt.Sprintf("%dm", minutes)
	case hours < 24:
		return fmt.Sprintf("%dh", hours)
	case days < 30:
		return fmt.Sprintf("%dd", days)
	case days < 365:
		return fmt.Sprintf("%dmo", days/30)
	default:
		return fmt.Sprintf("%dy", days/365)
	}
}

// LabelsString returns labels in stable key-sorted k=v comma-separated form.
func LabelsString(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}

	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, labels[k]))
	}
	return strings.Join(parts, ",")
}
