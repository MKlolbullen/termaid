package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/MKlolbullen/termaid/internal/graph"
)

/*───────── styles ─────────────────────*/
var (
	borderAct   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("10"))
	borderInact = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("8"))
)

/*───────── Builder model ──────────────*/

type focusArea int

const (
	fDomain focusArea = iota
	fList
	fCanvas
	fArgs
)

type BuilderModel struct {
	domainInp textinput.Model
	toolSel   list.Model
	argsInp   textinput.Model
	canvas    viewport.Model

	g   *graph.DAG
	occ map[string]int

	focus   focusArea
	selNode string
	layer   int
	msg     string
}

func NewBuilder(toolNames []string) BuilderModel {
	// domain field
	dom := textinput.New()
	dom.Placeholder = "example.com"
	dom.Focus()

	// tool list
	items := make([]list.Item, len(toolNames))
	for i, n := range toolNames {
		items[i] = entryItem{n, ""}
	}
	sel := list.New(items, list.NewDefaultDelegate(), 30, 14)
	sel.Title = "Tools"

	// args field
	arg := textinput.New()
	arg.Placeholder = "args (press c to save)"
	arg.Width = 38

	// canvas viewport
	cv := viewport.New(40, 14)

	return BuilderModel{
		domainInp: dom,
		toolSel:   sel,
		argsInp:   arg,
		canvas:    cv,
		g:         graph.NewDAG(),
		occ:       make(map[string]int),
		focus:     fDomain,
		selNode:   "input",
	}
}

func (m BuilderModel) Init() tea.Cmd { return nil }

/*───────── Update ─────────────────────*/

func (m BuilderModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {

	case tea.KeyMsg:
		switch v.String() {

		/* focus cycle */
		case "tab":
			m.focus = (m.focus + 1) % 4
		case "shift+tab":
			m.focus = (m.focus + 3) % 4

		/* navigate layers */
		case "up":
			if m.focus == fCanvas && m.layer > 0 {
				m.layer--
			}
		case "down":
			if m.focus == fCanvas {
				m.layer++
			}

		/* add node */
		case "n":
			if m.focus == fList {
				tool := m.toolSel.SelectedItem().(entryItem).name
				m.occ[tool]++
				id := fmt.Sprintf("%s-%d", tool, m.occ[tool])
				args := defaultArgs(tool)
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
			if m.focus == fCanvas && m.selNode != "input" {
				_ = m.g.RemoveNode(m.selNode)
				m.selNode = "input"
				m.layer = 0
			}

		/* commit args */
		case "c":
			if m.focus == fArgs {
				if n := m.g.Nodes[m.selNode]; n != nil {
					n.Args = m.argsInp.Value()
					m.msg = "args updated"
				}
			}

		/* finish */
		case "f":
			base := filepath.Join("workflows", "workflow-"+time.Now().Format("20060102-150405"))
			_ = os.WriteFile(base+".mmd", []byte(m.g.ToMermaid()), 0644)
			_ = os.WriteFile(base+".json", []byte(m.g.ToJSON()), 0644)
			m.msg = "saved " + base + ".*"
		}

	}

	/* delegate to subcomponents */
	switch m.focus {
	case fDomain:
		m.domainInp, _ = m.domainInp.Update(msg)
	case fList:
		m.toolSel, _ = m.toolSel.Update(msg)
	case fArgs:
		m.argsInp, _ = m.argsInp.Update(msg)
	case fCanvas:
		m.canvas, _ = m.canvas.Update(msg)
	}

	/* update canvas content */
	m.canvas.SetContent(renderLayerView(m.g, m.layer, m.selNode))

	return m, nil
}

/*───────── View ───────────────────────*/

func (m BuilderModel) View() string {
	// domain bar
	domainBar := maybeBorder(m.domainInp.View(), m.focus == fDomain)

	// left column (domain + list)
	left := lipgloss.JoinVertical(lipgloss.Top,
		domainBar,
		maybeBorder(m.toolSel.View(), m.focus == fList),
	)

	// right column (canvas + args)
	right := lipgloss.JoinVertical(lipgloss.Top,
		maybeBorder(m.canvas.View(), m.focus == fCanvas),
		maybeBorder("Args: "+m.argsInp.View(), m.focus == fArgs),
	)

	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Render("↑/↓ n=add r=rm c=args f=save tab=next shift+tab=prev")

	return lipgloss.JoinHorizontal(lipgloss.Top, left, right) +
		"\n" + help + "\n" + m.msg
}

/*───────── helpers ────────────────────*/

func maybeBorder(content string, active bool) string {
	if active {
		return borderAct.Render(content)
	}
	return borderInact.Render(content)
}

func renderLayerView(g *graph.DAG, focusLayer int, sel string) string {
	var out string
	max := 0
	for _, n := range g.Nodes {
		if n.Layer > max {
			max = n.Layer
		}
	}
	for l := 0; l <= max; l++ {
		nodes := g.GetLayer(l)
		prefix := "  "
		if l == focusLayer {
			prefix = "→ "
		}
		out += prefix + fmt.Sprintf("L%d: ", l)
		for i, id := range nodes {
			name := id
			if id == sel {
				name = lipgloss.NewStyle().Underline(true).Render(id)
			}
			out += name
			if i < len(nodes)-1 {
				out += ", "
			}
		}
		out += "\n"
	}
	return out
}
