package ui

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// PodRow is a single row in the pod table.
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

// PodsUpdatedMsg updates table data.
type PodsUpdatedMsg struct {
	Pods        []PodRow
	RefreshedAt time.Time
	Err         error
}

// ContextsUpdatedMsg updates available contexts.
type ContextsUpdatedMsg struct {
	Items   []string
	Current string
}

// NamespacesUpdatedMsg updates available namespaces.
type NamespacesUpdatedMsg struct {
	Items   []string
	Current string
}

type modalType int

const (
	modalNone modalType = iota
	modalContext
	modalNamespace
)

const refreshInterval = 5 * time.Second

type refreshTickMsg struct{}

// Model is the Bubble Tea UI state.
type Model struct {
	width  int
	height int

	pods       []PodRow
	contexts   []string
	namespaces []string

	currentContext   string
	currentNamespace string
	lastRefresh      time.Time
	err              error

	scrollOffset int

	modal      modalType
	modalIndex int

	styles Styles

	refreshCmd         func() tea.Cmd
	contextSelectedCmd func(string) tea.Cmd
	namespaceSelectCmd func(string) tea.Cmd
}

// Styles holds all lipgloss style definitions.
type Styles struct {
	App          lipgloss.Style
	Header       lipgloss.Style
	Body         lipgloss.Style
	Footer       lipgloss.Style
	TableHeader  lipgloss.Style
	TableCell    lipgloss.Style
	Modal        lipgloss.Style
	ModalTitle   lipgloss.Style
	ModalItem    lipgloss.Style
	ModalActive  lipgloss.Style
	StatusColors map[string]lipgloss.Style
	Error        lipgloss.Style
}

// Option configures the model.
type Option func(*Model)

// WithRefreshCmd sets the command for refresh key handling.
func WithRefreshCmd(fn func() tea.Cmd) Option {
	return func(m *Model) {
		m.refreshCmd = fn
	}
}

// WithContextSelectedCmd sets callback for context selection.
func WithContextSelectedCmd(fn func(string) tea.Cmd) Option {
	return func(m *Model) {
		m.contextSelectedCmd = fn
	}
}

// WithNamespaceSelectedCmd sets callback for namespace selection.
func WithNamespaceSelectedCmd(fn func(string) tea.Cmd) Option {
	return func(m *Model) {
		m.namespaceSelectCmd = fn
	}
}

// NewModel creates a new UI model.
func NewModel(opts ...Option) Model {
	m := Model{
		styles: defaultStyles(),
		width:  100,
		height: 30,
	}
	for _, opt := range opts {
		opt(&m)
	}
	return m
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{refreshTickCmd()}
	if m.refreshCmd != nil {
		cmds = append(cmds, m.refreshCmd())
	}
	return tea.Batch(cmds...)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Some terminals briefly report tiny/invalid sizes; avoid collapsing the table area.
		if msg.Width > 0 && msg.Width < 500 {
			m.width = msg.Width
		}
		// Some terminals/plugins can emit outlier heights; ignore unrealistic values.
		if msg.Height > 6 && msg.Height < 200 {
			m.height = msg.Height
		}
		m.clampScroll()
		return m, nil

	case PodsUpdatedMsg:
		m.pods = msg.Pods
		if msg.RefreshedAt.IsZero() {
			m.lastRefresh = time.Now()
		} else {
			m.lastRefresh = msg.RefreshedAt
		}
		m.err = msg.Err
		m.clampScroll()
		return m, nil

	case ContextsUpdatedMsg:
		m.contexts = msg.Items
		if msg.Current != "" {
			m.currentContext = msg.Current
		}
		m.modalIndex = 0
		return m, nil

	case NamespacesUpdatedMsg:
		m.namespaces = msg.Items
		if msg.Current != "" {
			m.currentNamespace = msg.Current
		}
		m.modalIndex = 0
		return m, nil

	case refreshTickMsg:
		if m.modal != modalNone {
			return m, refreshTickCmd()
		}
		if refresh := m.refreshIfEnabled(); refresh != nil {
			return m, tea.Batch(refreshTickCmd(), refresh)
		}
		return m, refreshTickCmd()

	case tea.KeyMsg:
		return m.updateKey(msg)
	}

	return m, nil
}

func (m Model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if key == "ctrl+c" || key == "Q" {
		return m, tea.Quit
	}

	if m.modal != modalNone {
		if index, handled := modalLetterIndex(m.modalItems(), msg); handled {
			if index >= 0 {
				m.modalIndex = index
			}
			return m, nil
		}

		switch key {
		case "R":
			return m, m.refreshIfEnabled()
		case "esc":
			m.modal = modalNone
			m.modalIndex = 0
			return m, nil
		case "up", "left":
			if m.modalIndex > 0 {
				m.modalIndex--
			}
			return m, nil
		case "down", "right":
			if m.modalIndex < m.modalLen()-1 {
				m.modalIndex++
			}
			return m, nil
		case "enter":
			return m.applyModalSelection()
		}
		return m, nil
	}

	switch key {
	case "C":
		if len(m.contexts) > 0 {
			m.modal = modalContext
			m.modalIndex = selectedIndex(m.contexts, m.currentContext)
		}
		return m, nil
	case "N":
		if len(m.namespaces) > 0 {
			m.modal = modalNamespace
			m.modalIndex = selectedIndex(m.namespaces, m.currentNamespace)
		}
		return m, nil
	case "R":
		return m, m.refreshIfEnabled()
	case "up", "left":
		if m.scrollOffset > 0 {
			m.scrollOffset--
		}
		return m, nil
	case "down", "right":
		if m.scrollOffset < m.maxScrollOffset() {
			m.scrollOffset++
		}
		return m, nil
	}

	return m, nil
}

func refreshTickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(time.Time) tea.Msg {
		return refreshTickMsg{}
	})
}

func (m Model) refreshIfEnabled() tea.Cmd {
	if m.refreshCmd != nil {
		return m.refreshCmd()
	}
	return nil
}

func (m Model) applyModalSelection() (tea.Model, tea.Cmd) {
	activeModal := m.modal
	items := m.modalItems()
	m.modal = modalNone
	if len(items) == 0 || m.modalIndex < 0 || m.modalIndex >= len(items) {
		m.modalIndex = 0
		return m, nil
	}
	selected := items[m.modalIndex]
	m.modalIndex = 0

	switch activeModal {
	case modalContext:
		m.currentContext = selected
		if m.contextSelectedCmd != nil {
			return m, m.contextSelectedCmd(selected)
		}
	case modalNamespace:
		m.currentNamespace = selected
		if m.namespaceSelectCmd != nil {
			return m, m.namespaceSelectCmd(selected)
		}
	}

	return m, nil
}

func (m Model) View() string {
	header := m.renderHeader()
	body := m.renderBody()
	footer := m.renderFooter()

	base := lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
	base = m.styles.App.Width(m.width).Render(base)

	if m.modal != modalNone {
		return m.renderModal(base)
	}

	return base
}

func (m Model) renderHeader() string {
	ctx := m.currentContext
	if ctx == "" {
		ctx = "-"
	}
	ns := m.currentNamespace
	if ns == "" {
		ns = "-"
	}
	last := "never"
	if !m.lastRefresh.IsZero() {
		last = m.lastRefresh.Format("15:04:05")
	}
	line := fmt.Sprintf("kontrol | context: %s | namespace: %s | refreshed: %s", ctx, ns, last)
	return m.styles.Header.Width(m.width).Render(line)
}

func (m Model) renderBody() string {
	bodyHeight := m.bodyHeight()
	if bodyHeight <= 0 {
		return ""
	}

	head := m.styles.TableHeader.Render(m.tableHeader())
	rows := m.visibleRows(max(0, bodyHeight-1))
	content := append([]string{head}, rows...)

	if m.err != nil {
		content = append(content, m.styles.Error.Render("error: "+m.err.Error()))
	}

	body := strings.Join(content, "\n")
	return m.styles.Body.Width(m.width).Render(body)
}

func (m Model) renderFooter() string {
	info := fmt.Sprintf("pods: %d  scroll: %d/%d", len(m.pods), m.scrollOffset, m.maxScrollOffset())
	help := "C context • N namespace • R refresh • ↑/↓ scroll • Q quit"
	if m.modal != modalNone {
		help = "type lowercase letter jump • ↑/↓ move • R refresh • enter select • esc close • Q quit"
	}
	return m.styles.Footer.Width(m.width).Render(info + "\n" + help)
}

func (m Model) renderModal(base string) string {
	background := strings.Split(lipgloss.Place(m.width, m.height, lipgloss.Left, lipgloss.Top, base), "\n")
	faint := lipgloss.NewStyle().Faint(true)
	for i, line := range background {
		background[i] = faint.Render(line)
	}

	box := m.modalBox()
	boxWidth := lipgloss.Width(box)
	boxHeight := lipgloss.Height(box)
	x := max(0, (m.width-boxWidth)/2)
	y := max(0, (m.height-boxHeight)/2)

	return overlayLines(background, strings.Split(box, "\n"), x, y, m.width)
}

func (m Model) tableHeader() string {
	return strings.Join([]string{
		fixedCell("NAME", 24),
		fixedCell("STATUS", 10),
		fixedCell("READY", 7),
		fixedCell("RESTARTS", 8),
		fixedCell("AGE", 6),
		fixedCell("IP", 15),
		fixedCell("NODE", 18),
		"LABELS",
	}, " ")
}

func (m Model) visibleRows(maxRows int) []string {
	if len(m.pods) == 0 || maxRows <= 0 {
		return nil
	}

	start := m.scrollOffset
	if start > len(m.pods)-1 {
		start = max(0, len(m.pods)-1)
	}
	end := min(len(m.pods), start+maxRows)

	out := make([]string, 0, end-start)
	for _, pod := range m.pods[start:end] {
		status := m.statusStyle(pod.Status).Width(10).Render(truncate(pod.Status, 10))
		row := strings.Join([]string{
			fixedCell(pod.Name, 24),
			status,
			fixedCell(pod.Ready, 7),
			fixedCell(pod.Restarts, 8),
			fixedCell(pod.Age, 6),
			fixedCell(pod.IP, 15),
			fixedCell(pod.Node, 18),
			truncate(pod.Labels, max(0, m.width-100)),
		}, " ")
		out = append(out, m.styles.TableCell.Render(row))
	}
	return out
}

func (m Model) statusStyle(status string) lipgloss.Style {
	key := strings.ToLower(status)
	for k, style := range m.styles.StatusColors {
		if strings.Contains(key, k) {
			return style
		}
	}
	return m.styles.TableCell
}

func (m *Model) clampScroll() {
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
	if m.scrollOffset > m.maxScrollOffset() {
		m.scrollOffset = m.maxScrollOffset()
	}
}

func (m Model) maxScrollOffset() int {
	rowsPerPage := max(1, m.bodyHeight()-1)
	maxOffset := len(m.pods) - rowsPerPage
	if maxOffset < 0 {
		return 0
	}
	return maxOffset
}

func (m Model) bodyHeight() int {
	const headerHeight = 1
	const footerHeight = 2
	h := m.height - headerHeight - footerHeight
	if h < 1 {
		return 1
	}
	return h
}

func (m Model) modalItems() []string {
	if m.modal == modalContext {
		return m.contexts
	}
	if m.modal == modalNamespace {
		return m.namespaces
	}
	return nil
}

func (m Model) modalLen() int {
	return len(m.modalItems())
}

func modalLetterIndex(items []string, msg tea.KeyMsg) (int, bool) {
	if msg.Type != tea.KeyRunes || len(msg.Runes) != 1 || !unicode.IsLower(msg.Runes[0]) {
		return 0, false
	}

	prefix := strings.ToLower(string(msg.Runes[0]))
	for i, item := range items {
		if strings.HasPrefix(strings.ToLower(item), prefix) {
			return i, true
		}
	}

	return -1, true
}

func (m Model) modalBox() string {
	title := "Select context"
	if m.modal == modalNamespace {
		title = "Select namespace"
	}

	items := m.modalItems()
	total := len(items)
	activeIndex := clampIndex(m.modalIndex, total)
	if total == 0 {
		items = []string{"(no entries)"}
	}

	boxWidth := min(56, max(1, m.width-4))
	boxStyle := m.styles.Modal.Width(boxWidth)
	innerWidth := max(1, boxWidth-boxStyle.GetHorizontalFrameSize())
	pageSize := max(1, m.height-boxStyle.GetVerticalFrameSize()-2)

	start, end := modalWindow(total, activeIndex, pageSize)
	if total == 0 {
		start, end = 0, 1
	}

	lines := make([]string, 0, end-start+2)
	lines = append(lines, m.styles.ModalTitle.Width(innerWidth).Render(truncate(title, innerWidth)))

	itemWidth := max(1, innerWidth-2)
	for i, item := range items[start:end] {
		actualIndex := start + i
		prefix := "  "
		style := m.styles.ModalItem.Width(innerWidth)
		if total > 0 && actualIndex == activeIndex {
			prefix = "› "
			style = m.styles.ModalActive.Width(innerWidth)
		}
		lines = append(lines, style.Render(prefix+truncate(item, itemWidth)))
	}

	lines = append(lines, lipgloss.NewStyle().
		Foreground(lipgloss.Color("244")).
		Width(innerWidth).
		Render(truncate(modalFooter(start, end, total), innerWidth)))

	return boxStyle.Render(strings.Join(lines, "\n"))
}

func modalWindow(total, selected, pageSize int) (int, int) {
	if total <= 0 {
		return 0, 0
	}
	if pageSize <= 0 || total <= pageSize {
		return 0, total
	}

	selected = clampIndex(selected, total)
	start := selected - pageSize/2
	if start < 0 {
		start = 0
	}

	maxStart := total - pageSize
	if start > maxStart {
		start = maxStart
	}

	return start, min(total, start+pageSize)
}

func modalFooter(start, end, total int) string {
	if total <= 0 {
		return "0 items"
	}
	return fmt.Sprintf("%d-%d of %d", start+1, end, total)
}

func overlayLines(background, foreground []string, x, y, width int) string {
	if width <= 0 {
		return strings.Join(background, "\n")
	}

	for i, line := range foreground {
		row := y + i
		if row < 0 || row >= len(background) {
			continue
		}

		lineWidth := lipgloss.Width(line)
		if lineWidth <= 0 || x >= width {
			continue
		}

		left := ansi.Cut(background[row], 0, x)
		rightStart := min(width, x+lineWidth)
		right := ansi.Cut(background[row], rightStart, width)
		background[row] = left + line + right
	}

	return strings.Join(background, "\n")
}

func clampIndex(index, total int) int {
	if total <= 0 || index < 0 {
		return 0
	}
	if index >= total {
		return total - 1
	}
	return index
}

func selectedIndex(items []string, current string) int {
	for i, item := range items {
		if item == current {
			return i
		}
	}
	return 0
}

func defaultStyles() Styles {
	return Styles{
		App:         lipgloss.NewStyle().Padding(0, 1),
		Header:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62")).Padding(0, 1),
		Body:        lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Padding(0, 1),
		Footer:      lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Padding(0, 1),
		TableHeader: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("229")),
		TableCell:   lipgloss.NewStyle().Foreground(lipgloss.Color("252")),
		Modal:       lipgloss.NewStyle().Border(lipgloss.ThickBorder()).BorderForeground(lipgloss.Color("62")).Padding(1, 2).Width(56),
		ModalTitle:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("229")),
		ModalItem:   lipgloss.NewStyle().Foreground(lipgloss.Color("252")),
		ModalActive: lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Background(lipgloss.Color("62")).Bold(true),
		Error:       lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true),
		StatusColors: map[string]lipgloss.Style{
			"running":   lipgloss.NewStyle().Foreground(lipgloss.Color("42")),
			"succeeded": lipgloss.NewStyle().Foreground(lipgloss.Color("42")),
			"pending":   lipgloss.NewStyle().Foreground(lipgloss.Color("214")),
			"unknown":   lipgloss.NewStyle().Foreground(lipgloss.Color("214")),
			"failed":    lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true),
			"error":     lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true),
			"crash":     lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true),
		},
	}
}

func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	return string(r[:width-1]) + "…"
}

func fixedCell(s string, width int) string {
	return lipgloss.NewStyle().Width(width).Render(truncate(s, width))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
