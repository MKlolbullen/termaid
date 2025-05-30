package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/MKlolbullen/termaid/internal/graph"
)


/* ---------- Builder model ---------- */

type BuilderModel struct {
	toolSel list.Model
	argsInp textinput.Model
	g       *graph.DAG

	occ   map[string]int // tool → count
	focus string         // list | dag | args

	selNode string
	layer   int
	msg     string
}

func NewBuilder(toolNames []string) BuilderModel {
	items := make([]list.Item, len(toolNames))
	for i, n := range toolNames {
		items[i] = entryItem{n, ""}
	}
	sel := list.New(items, list.NewDefaultDelegate(), 28, 14)
	sel.Title = "Tools"

	inp := textinput.New()
	inp.Placeholder = "args (press c to save)"
	inp.Width = 40

	return BuilderModel{
		toolSel: sel,
		argsInp: inp,
		g:       graph.NewDAG(),
		occ:     make(map[string]int),
		focus:   "list",
		selNode: "input",
	}
}

func (m BuilderModel) Init() tea.Cmd { return nil }

/* ---------- Update ---------- */

func (m BuilderModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {

	case tea.KeyMsg:
		switch v.String() {

		case "tab":
			switch m.focus {
			case "list":
				m.focus = "dag"
			case "dag":
				m.focus = "args"
			default:
				m.focus = "list"
			}

		case "up":
			if m.focus == "dag" && m.layer > 0 {
				m.layer--
			}
		case "down":
			if m.focus == "dag" {
				m.layer++
			}

		/* add node */
		case "n":
			if m.focus == "list" {
				tool := m.toolSel.SelectedItem().(entryItem).name
				m.occ[tool]++
				id := fmt.Sprintf("%s-%d", tool, m.occ[tool])
				args := defaultArgs(tool)
				if args == "" {
					args = m.argsInp.Value()
				}
				if err := m.g.AddNode(m.selNode, id, tool, args, m.layer+1); err != nil {
					m.msg = err.Error()
				} else {
					m.selNode = id
					m.layer++
					m.msg = "added " + id
				}
			}

		/* remove node */
		case "r":
			if m.focus == "dag" && m.selNode != "input" {
				if err := m.g.RemoveNode(m.selNode); err != nil {
					m.msg = err.Error()
				} else {
					m.selNode, m.layer = "input", 0
					m.msg = "node removed"
				}
			}

		/* commit args */
		case "c":
			if m.focus == "args" {
				if n := m.g.Nodes[m.selNode]; n != nil {
					n.Args = m.argsInp.Value()
					m.msg = "args saved"
				}
			}

		/* finish / save */
		case "f":
			base := filepath.Join("workflows", "workflow-"+time.Now().Format("20060102-150405"))
			_ = os.WriteFile(base+".mmd", []byte(m.g.ToMermaid()), 0644)
			_ = os.WriteFile(base+".json", []byte(m.g.ToJSON()), 0644)
			m.msg = "saved " + base + ".*"
		}
	}

	// delegate internal components
	switch m.focus {
	case "list":
		m.toolSel, _ = m.toolSel.Update(msg)
	case "args":
		m.argsInp, _ = m.argsInp.Update(msg)
	}
	return m, nil
}

/* ---------- View ---------- */

func (m BuilderModel) View() string {
	left := m.toolSel.View()

	right := lipgloss.NewStyle().Bold(true).Render("Workflow:") + "\n"
	for l := 0; l <= m.layer+1; l++ {
		row := m.g.GetLayer(l)
		if len(row) == 0 {
			continue
		}
		right += fmt.Sprintf("[L%d] ", l)
		for _, id := range row {
			style := lipgloss.NewStyle()
			if id == m.selNode {
				style = style.Foreground(lipgloss.Color("10"))
			}
			right += style.Render(id) + " "
		}
		right += "\n"
	}
	right += "\nArgs: " + m.argsInp.View()

	return lipgloss.JoinHorizontal(lipgloss.Top, left, right) +
		"\n\n" + m.msg
}
