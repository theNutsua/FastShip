package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// --- the data the TUI displays, fetched from the daemon ---

type statusRow struct {
	Name  string `json:"name"`
	Image string `json:"image"`
	State string `json:"state"`
}
type metricRow struct {
	Name        string `json:"name"`
	MemoryBytes uint64 `json:"memory_bytes"`
}
type imageRow struct {
	Component string `json:"component"`
	SizeBytes int64  `json:"size_bytes"`
}

// model is the TUI state.
type model struct {
	app     string
	status  []statusRow
	metrics []metricRow
	images  []imageRow
	logs    []string
	width   int
	height  int
	err     error
}

// tickMsg fires on the refresh timer.
type tickMsg time.Time

// tick schedules the next refresh.
func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m model) Init() tea.Cmd {
	return tick() // start the refresh loop
}

// Update handles events: refresh ticks and keypresses.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// q or ctrl+c quits.
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tickMsg:
		// Refresh all data, then schedule the next tick.
		m.status = fetchStatus(m.app)
		m.metrics = fetchMetrics(m.app)
		m.images = fetchImages(m.app)
		m.logs = fetchLogs(m.app)
		return m, tick()
	}
	return m, nil
}

// View renders the dashboard.
func (m model) View() string {
	// Top panel: components + images + memory.
	var top strings.Builder

	top.WriteString(sectionStyle.Render("COMPONENTS") + "    " + sectionStyle.Render("IMAGES") + "\n")
	// Pair status and images side by side by component name.
	for _, s := range m.status {
		dot := stoppedDot
		if s.State == "running" {
			dot = runningDot
		}
		size := ""
		for _, img := range m.images {
			if img.Component == s.Name {
				size = formatBytes(img.SizeBytes)
			}
		}
		top.WriteString(fmt.Sprintf("%s %-10s %-8s   %s\n", dot, s.Name, s.State, size))
	}

	top.WriteString("\n" + sectionStyle.Render("MEMORY") + "\n")
	for _, mt := range m.metrics {
		top.WriteString(fmt.Sprintf("  %-10s %s\n", mt.Name, formatBytes(int64(mt.MemoryBytes))))
	}

	topPanel := panelStyle.Width(m.width - 2).Render(
		titleStyle.Render(" "+m.app+" ") + "\n" + top.String())

	// Bottom panel: live logs (last N lines that fit).
	logLines := m.logs
	maxLogs := m.height - lipgloss.Height(topPanel) - 4
	if maxLogs > 0 && len(logLines) > maxLogs {
		logLines = logLines[len(logLines)-maxLogs:]
	}
	logPanel := panelStyle.Width(m.width - 2).Render(
		sectionStyle.Render(" logs ") + "\n" + strings.Join(logLines, "\n"))

	help := helpStyle.Render("q to quit")

	return topPanel + "\n" + logPanel + "\n" + help
}

// --- styles ---

var (
	panelStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("39"))
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	sectionStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("245"))
	helpStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	runningDot   = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("●")
	stoppedDot   = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("●")
)

// runDebugTUI starts the program.
func runDebugTUI(app string) error {
	if err := ensureDaemon(); err != nil {
		return err
	}
	p := tea.NewProgram(model{app: app}, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// --- data fetchers (hit the daemon endpoints) ---

func fetchStatus(app string) []statusRow {
	var out []statusRow
	fetchJSON("/status?name="+app, &out)
	return out
}
func fetchMetrics(app string) []metricRow {
	var out []metricRow
	fetchJSON("/metrics?name="+app, &out)
	return out
}
func fetchImages(app string) []imageRow {
	var out []imageRow
	fetchJSON("/images?name="+app, &out)
	return out
}
func fetchLogs(app string) []string {
	client := newClient()
	resp, err := client.Get("http://localhost/logs?name=" + app)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return strings.Split(strings.TrimRight(string(data), "\n"), "\n")
}

// fetchJSON is a small helper for the JSON endpoints.
func fetchJSON(path string, out any) {
	client := newClient()
	resp, err := client.Get("http://localhost" + path)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	json.NewDecoder(resp.Body).Decode(out)
}

// formatBytes turns raw bytes into "5.0 MB" for display.
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

var _ = context.Background // keep import if unused after edits
