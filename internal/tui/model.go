package tui

import (
	"bytes"
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/MKlolbullen/termaid/internal/pipeline"
)

/* ────────────────── Progress + Log Model ─────────────────────── */

type Model struct {
	cats  []pipeline.Category
	state map[string]pipeline.StatusUpdateType // node ID → status

	logBuf  bytes.Buffer
	vp      viewport.Model
	showLog bool

	statusCh <-chan pipeline.Status
	done     bool
	logPath  string
}

type doneMsg struct{}

func New(cats []pipeline.Category, ch <-chan pipeline.Status) Model {
	vp := viewport.New(0, 10) // width set later
	vp.SetContent("")

	return Model{
		cats:     cats,
		state:    make(map[string]pipeline.StatusUpdateType),
		vp:       vp,
		statusCh: ch,
		logPath:  fmt.Sprintf("run-%d.log", time.Now().Unix()),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.nextStatus(), viewport.Sync(m.vp))
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {

	case pipeline.Status:
		m.state[v.Tool] = v.Type
		line := fmt.Sprintf("[%s] %-15s %s", v.Category, v.Tool, statusWord(v))
		if v.Type == pipeline.StatusError {
			line = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(line)
		}
		m.logBuf.WriteString(line + "\n")
		m.vp.SetContent(m.logBuf.String())
		m.vp.GotoBottom()
		return m, m.nextStatus()

	case doneMsg:
		m.done = true
		m.flushLog()
		return m, nil

	case tea.KeyMsg:
		switch v.String() {
		case "q":
			if !m.done {
				m.flushLog()
			}
			return m, tea.Quit
		case "tab":
			m.showLog = !m.showLog
		}
		if m.showLog {
			var cmd tea.Cmd
			m.vp, cmd = m.vp.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

func (m Model) View() string {
	chart := m.renderChart()

	if m.vp.Width == 0 {
		m.vp.Width = lipgloss.Width(chart)
	}

	footer := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Render("[tab] logs • [q] quit")

	if m.showLog {
		title := lipgloss.NewStyle().Bold(true).Render("Live Output (↑/↓ PgUp/PgDn)")
		return chart + "\n" + title + "\n" + m.vp.View() + "\n" + footer
	}
	return chart + "\n" + footer
}

/* ────────────────── helpers ───────────────────── */

func (m Model) nextStatus() tea.Cmd {
	return func() tea.Msg {
		if st, ok := <-m.statusCh; ok {
			return st
		}
		return doneMsg{}
	}
}

func (m Model) flushLog() {
	_ = os.WriteFile(m.logPath, m.logBuf.Bytes(), 0644)
}

func statusWord(s pipeline.Status) string {
	switch s.Type {
	case pipeline.StatusStart:
		return "started"
	case pipeline.StatusFinish:
		return "done"
	case pipeline.StatusError:
		return "error"
	default:
		return "?"
	}
}

func (m Model) renderChart() string {
	var out string
	for i, cat := range m.cats {
		switch {
		case i == 0:
			out += "/-> "
		case i == len(m.cats)-1:
			out += "\\-> "
		default:
			out += "--> "
		}
		for j, t := range cat.Tools {
			id := t.Name // node ID
			style := lipgloss.NewStyle()
			switch m.state[id] {
			case pipeline.StatusStart:
				style = style.Foreground(lipgloss.Color("11")) // yellow
			case pipeline.StatusFinish:
				style = style.Foreground(lipgloss.Color("10")) // green
			case pipeline.StatusError:
				style = style.Foreground(lipgloss.Color("9")) // red
			}
			out += style.Render(id)
			if j != len(cat.Tools)-1 {
				out += ","
			}
		}
		out += "\n"
	}
	return out
}