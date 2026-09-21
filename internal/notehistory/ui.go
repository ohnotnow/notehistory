package notehistory

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// refreshEvery is how often the TUI re-reads the history database so
// notifications recorded by the background watcher appear without a restart.
const refreshEvery = 10 * time.Second

var (
	colourAccent = lipgloss.AdaptiveColor{Light: "#005f87", Dark: "#5fd7ff"}
	colourMuted  = lipgloss.AdaptiveColor{Light: "#5f5f5f", Dark: "#a8a8a8"}
	colourBg     = lipgloss.AdaptiveColor{Light: "#ffffff", Dark: "#000000"}

	styleHeader   = lipgloss.NewStyle().Bold(true).Foreground(colourAccent)
	styleMuted    = lipgloss.NewStyle().Foreground(colourMuted)
	styleSelected = lipgloss.NewStyle().Bold(true).Foreground(colourBg).Background(colourAccent)
	styleApp      = lipgloss.NewStyle().Bold(true)
	styleDivider  = lipgloss.NewStyle().Foreground(colourMuted)
)

type mode int

const (
	modeList mode = iota
	modeSearch
)

type reloadMsg struct {
	rows []Notification
	err  error
}

type tickMsg struct{}

type uiModel struct {
	mode   mode
	store  *Store
	names  *AppNames
	all    []Notification // every row, newest first
	shown  []int          // indexes into all that match the filter
	cursor int            // index into shown
	top    int            // first visible row of shown
	filter string
	input  textinput.Model
	width  int
	height int
	status string
	err    string
}

// RunTUI opens the interactive browser over the history database.
func RunTUI(store *Store) error {
	in := textinput.New()
	in.Prompt = "/"
	in.CharLimit = 200
	m := uiModel{store: store, names: NewAppNames(), input: in}
	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

func (m uiModel) reload() tea.Msg {
	rows, err := m.store.All(context.Background())
	return reloadMsg{rows: rows, err: err}
}

func tick() tea.Cmd {
	return tea.Tick(refreshEvery, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m uiModel) Init() tea.Cmd {
	return tea.Batch(m.reload, tick())
}

func (m uiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clampCursor()
		return m, nil
	case tickMsg:
		return m, tea.Batch(m.reload, tick())
	case reloadMsg:
		if msg.err != nil {
			m.status = "reload failed: " + msg.err.Error()
			return m, nil
		}
		m.setRows(msg.rows)
		return m, nil
	case tea.KeyMsg:
		m.status = ""
		if m.mode == modeSearch {
			return m.updateSearch(msg)
		}
		return m.updateList(msg)
	}
	return m, nil
}

func (m uiModel) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "j", "down":
		m.cursor++
	case "k", "up":
		m.cursor--
	case "g", "home":
		m.cursor = 0
	case "G", "end":
		m.cursor = len(m.shown) - 1
	case "ctrl+d", "pgdown":
		m.cursor += m.listHeight() / 2
	case "ctrl+u", "pgup":
		m.cursor -= m.listHeight() / 2
	case "/":
		m.mode = modeSearch
		m.input.SetValue(m.filter)
		m.input.CursorEnd()
		return m, m.input.Focus()
	case "esc":
		m.filter = ""
		m.applyFilter()
	case "r":
		return m, m.reload
	}
	m.clampCursor()
	return m, nil
}

func (m uiModel) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeList
		m.filter = ""
		m.input.Blur()
		m.applyFilter()
		return m, nil
	case "enter":
		m.mode = modeList
		m.input.Blur()
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "backspace":
		// Backspacing past the start of an empty search feels like
		// deleting the "/", so treat it as leaving search entirely.
		if m.input.Value() == "" {
			return m.updateSearch(tea.KeyMsg{Type: tea.KeyEsc})
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.filter = m.input.Value()
	m.applyFilter()
	return m, cmd
}

// setRows replaces the data, keeping the cursor on the same notification
// where possible so a background refresh does not yank it about.
func (m *uiModel) setRows(rows []Notification) {
	var keep string
	if n, ok := m.selected(); ok {
		keep = n.UUID
	}
	m.all = rows
	m.applyFilter()
	if keep != "" {
		for i, idx := range m.shown {
			if m.all[idx].UUID == keep {
				m.cursor = i
				break
			}
		}
	}
	m.clampCursor()
}

// applyFilter rebuilds shown from the current filter. Every word in the
// filter must appear somewhere in the notification, in any order, ignoring
// case. The resolved app name counts too, so "teams" finds Microsoft Teams.
func (m *uiModel) applyFilter() {
	words := strings.Fields(strings.ToLower(m.filter))
	m.shown = m.shown[:0]
	for i, n := range m.all {
		if matches(n, m.names.Name(n.App), words) {
			m.shown = append(m.shown, i)
		}
	}
	m.clampCursor()
}

func matches(n Notification, appName string, words []string) bool {
	if len(words) == 0 {
		return true
	}
	hay := strings.ToLower(strings.Join([]string{n.DeliveredAt, n.App, appName, n.Title, n.Subtitle, n.Body}, " "))
	for _, w := range words {
		if !strings.Contains(hay, w) {
			return false
		}
	}
	return true
}

func (m *uiModel) clampCursor() {
	if m.cursor >= len(m.shown) {
		m.cursor = len(m.shown) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	h := m.listHeight()
	if h < 1 {
		h = 1
	}
	if m.cursor < m.top {
		m.top = m.cursor
	}
	if m.cursor >= m.top+h {
		m.top = m.cursor - h + 1
	}
	if m.top < 0 {
		m.top = 0
	}
}

func (m uiModel) selected() (Notification, bool) {
	if m.cursor < 0 || m.cursor >= len(m.shown) {
		return Notification{}, false
	}
	return m.all[m.shown[m.cursor]], true
}

const detailLines = 6

// listHeight is the rows available for the list: total minus header,
// divider, detail pane and footer.
func (m uiModel) listHeight() int {
	return m.height - 1 - 1 - detailLines - 1
}

func (m uiModel) View() string {
	if m.err != "" {
		return m.err + "\n"
	}
	if m.width == 0 {
		return "loading..."
	}
	var b strings.Builder
	b.WriteString(m.header())
	b.WriteString("\n")

	h := m.listHeight()
	for i := 0; i < h; i++ {
		idx := m.top + i
		if idx < len(m.shown) {
			b.WriteString(m.row(m.all[m.shown[idx]], idx == m.cursor))
		}
		b.WriteString("\n")
	}
	b.WriteString(styleDivider.Render(strings.Repeat("─", m.width)))
	b.WriteString("\n")
	b.WriteString(m.detail())
	b.WriteString(m.footer())
	return b.String()
}

func (m uiModel) header() string {
	count := fmt.Sprintf("%d of %d", len(m.shown), len(m.all))
	if m.filter == "" {
		count = fmt.Sprintf("%d notifications", len(m.all))
	}
	return styleHeader.Render("notehistory") + "  " + styleMuted.Render(count)
}

func (m uiModel) row(n Notification, selected bool) string {
	text := Clean(strings.Join(nonEmpty(n.Title, n.Subtitle, n.Body), " | "))
	app := m.names.Name(n.App)
	if app == "" {
		app = "?"
	}
	if selected {
		line := fmt.Sprintf(" %s  %-18s %s", n.DeliveredAt, ansi.Truncate(app, 18, "…"), text)
		return styleSelected.Render(padToWidth(ansi.Truncate(line, m.width, "…"), m.width))
	}
	// Style parts separately and pad the plain text only, avoiding the
	// lipgloss Width() miscount with nested styles.
	prefix := " " + styleMuted.Render(n.DeliveredAt) + "  " + styleApp.Render(fmt.Sprintf("%-18s", ansi.Truncate(app, 18, "…"))) + " "
	room := m.width - lipgloss.Width(prefix)
	if room < 0 {
		room = 0
	}
	return prefix + ansi.Truncate(text, room, "…")
}

func (m uiModel) detail() string {
	n, ok := m.selected()
	lines := make([]string, 0, detailLines)
	if !ok {
		lines = append(lines, styleMuted.Render("nothing selected"))
	} else {
		lines = append(lines, styleApp.Render(m.names.Name(n.App))+"  "+styleMuted.Render(n.DeliveredAt))
		for _, part := range nonEmpty(Clean(n.Title), Clean(n.Subtitle)) {
			lines = append(lines, part)
		}
		lines = append(lines, ansi.Hardwrap(Clean(n.Body), m.width, false))
	}
	text := strings.Join(lines, "\n")
	// Cap to detailLines rows and pad so the footer stays put.
	rows := strings.Split(text, "\n")
	if len(rows) > detailLines {
		rows = rows[:detailLines]
		rows[detailLines-1] = ansi.Truncate(rows[detailLines-1], m.width-1, "…")
	}
	for len(rows) < detailLines {
		rows = append(rows, "")
	}
	for i := range rows {
		rows[i] = ansi.Truncate(rows[i], m.width, "…")
	}
	return strings.Join(rows, "\n") + "\n"
}

func (m uiModel) footer() string {
	if m.mode == modeSearch {
		return m.input.View()
	}
	if m.status != "" {
		return m.status
	}
	help := "j/k move  g/G top/bottom  ctrl+d/u page  / search  esc clear  r reload  q quit"
	if m.filter != "" {
		help = "filter: " + m.filter + "   esc clear  / edit  q quit"
	}
	return styleMuted.Render(ansi.Truncate(help, m.width, ""))
}

func nonEmpty(parts ...string) []string {
	out := parts[:0]
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func padToWidth(s string, w int) string {
	vw := lipgloss.Width(s)
	if vw >= w {
		return s
	}
	return s + strings.Repeat(" ", w-vw)
}
