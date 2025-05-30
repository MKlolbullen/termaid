package main

import (
	"fmt"
	"log"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/MKlolbullen/termaid/internal/graph"
)

type model struct {
	width   int
	height  int
	focus   int // 0=tools, 1=help, 2=input, 3=visual
	toolIdx int
	tools   []string
	dag     *graph.DAG
	selNode string
	layer   int
}

func initialModel() model {
	dag := graph.NewDAG()
	return model{
		tools:   []string{"subfinder", "httpx", "nuclei", "ffuf", "gobuster", "dalfox", "amass", "naabu"},
		dag:     dag,
		selNode: "input",
		layer:   0,
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "tab":
			m.focus = (m.focus + 1) % 4
		case "up":
			if m.focus == 0 && m.toolIdx > 0 {
				m.toolIdx--
			} else if m.focus == 3 && m.layer > 0 {
				m.layer--
				m.updateSelection()
			}
		case "down":
			if m.focus == 0 && m.toolIdx < len(m.tools)-1 {
				m.toolIdx++
			} else if m.focus == 3 {
				m.layer++
				m.updateSelection()
			}
		case "enter", "n":
			if m.focus == 0 {
				m.addTool()
			}
		case "r":
			if m.focus == 3 && m.selNode != "input" {
				m.dag.RemoveNode(m.selNode)
				m.selNode = "input"
				m.layer = 0
			}
		}
	}
	return m, nil
}

func (m *model) addTool() {
	tool := m.tools[m.toolIdx]
	id := fmt.Sprintf("%s-1", tool)
	args := fmt.Sprintf("-d {{domain}} -o {{output}}")
	
	m.dag.AddNodeAtPosition(m.selNode, id, tool, args, m.layer+1, 0, "", false)
	m.selNode = id
	m.layer++
}

func (m *model) updateSelection() {
	nodes := m.dag.GetLayer(m.layer)
	if len(nodes) > 0 {
		m.selNode = nodes[0]
	}
}

func (m model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Initializing..."
	}

	// Calculate 2x2 layout dimensions
	toolsW := m.width / 5           // 20%
	inputW := (m.width * 4) / 5     // 80%
	helpH := m.height / 5           // 20%
	visualH := (m.height * 4) / 5   // 80%

	// Render panels
	toolsPanel := m.renderTools(toolsW, m.height-helpH)
	helpPanel := m.renderHelp(toolsW, helpH)
	inputPanel := m.renderInput(inputW, helpH)
	visualPanel := m.renderVisual(inputW, visualH)

	// Combine layout
	leftCol := lipgloss.JoinVertical(lipgloss.Top, toolsPanel, helpPanel)
	rightCol := lipgloss.JoinVertical(lipgloss.Top, inputPanel, visualPanel)
	
	return lipgloss.JoinHorizontal(lipgloss.Top, leftCol, rightCol)
}

func (m model) renderTools(w, h int) string {
	var content strings.Builder
	content.WriteString("Tools:\n")
	
	start := max(0, m.toolIdx-h+3)
	end := min(len(m.tools), start+h-2)
	
	for i := start; i < end; i++ {
		prefix := "  "
		if i == m.toolIdx {
			prefix = "▶ "
		}
		content.WriteString(fmt.Sprintf("%s%s\n", prefix, m.tools[i]))
	}
	
	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Width(w).Height(h)
	if m.focus == 0 {
		style = style.BorderForeground(lipgloss.Color("10"))
	} else {
		style = style.BorderForeground(lipgloss.Color("8"))
	}
	
	return style.Render(content.String())
}

func (m model) renderHelp(w, h int) string {
	content := "Matrix Controls:\ntab - focus\n↑/↓ - navigate\nn - add tool\nr - remove\nq - quit"
	
	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Width(w).Height(h)
	if m.focus == 1 {
		style = style.BorderForeground(lipgloss.Color("10"))
	} else {
		style = style.BorderForeground(lipgloss.Color("8"))
	}
	
	return style.Render(content)
}

func (m model) renderInput(w, h int) string {
	content := fmt.Sprintf("Target: example.com\nSelected: %s\nMatrix: [%d,%d]", m.selNode, m.layer, 0)
	
	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Width(w).Height(h)
	if m.focus == 2 {
		style = style.BorderForeground(lipgloss.Color("10"))
	} else {
		style = style.BorderForeground(lipgloss.Color("8"))
	}
	
	return style.Render(content)
}

func (m model) renderVisual(w, h int) string {
	var content strings.Builder
	content.WriteString("Matrix Workflow:\n\n")
	
	for layer := 0; layer <= m.dag.MaxX; layer++ {
		layerMatrix := m.dag.GetLayerMatrix(layer)
		prefix := "  "
		if layer == m.layer && m.focus == 3 {
			prefix = "▶ "
		}
		
		content.WriteString(fmt.Sprintf("%sL%d: ", prefix, layer))
		
		hasNodes := false
		for pos := 0; pos <= m.dag.MaxY; pos++ {
			if nodes, exists := layerMatrix[pos]; exists {
				if hasNodes {
					content.WriteString(" | ")
				}
				content.WriteString("P")
				content.WriteString(fmt.Sprintf("%d[", pos))
				for i, node := range nodes {
					if node.ID == m.selNode {
						content.WriteString("*")
						content.WriteString(node.ID)
						content.WriteString("*")
					} else {
						content.WriteString(node.ID)
					}
					if node.Parallel {
						content.WriteString("∥")
					}
					if i < len(nodes)-1 {
						content.WriteString(",")
					}
				}
				content.WriteString("]")
				hasNodes = true
			}
		}
		if !hasNodes {
			content.WriteString("(empty)")
		}
		content.WriteString("\n")
	}
	
	content.WriteString("\nMermaid (LR):\n")
	mermaidLines := strings.Split(m.dag.ToCompactMermaid(), "\n")
	maxLines := min(h-8, len(mermaidLines))
	for i := 0; i < maxLines; i++ {
		if i < len(mermaidLines) && strings.TrimSpace(mermaidLines[i]) != "" {
			content.WriteString(mermaidLines[i])
			content.WriteString("\n")
		}
	}
	
	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Width(w).Height(h)
	if m.focus == 3 {
		style = style.BorderForeground(lipgloss.Color("10"))
	} else {
		style = style.BorderForeground(lipgloss.Color("8"))
	}
	
	return style.Render(content.String())
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if err := p.Start(); err != nil {
		log.Fatal(err)
	}
}