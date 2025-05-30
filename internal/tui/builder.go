package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/MKlolbullen/termaid/internal/graph"
)

/*───────── styles ─────────────────────*/
var (
	borderStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder())
	activeBorder   = borderStyle.Copy().BorderForeground(lipgloss.Color("10"))
	inactiveBorder = borderStyle.Copy().BorderForeground(lipgloss.Color("8"))
	toolStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	selectedStyle  = lipgloss.NewStyle().Background(lipgloss.Color("8")).Foreground(lipgloss.Color("15"))
	helpStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	errorStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	successStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
)

/*───────── Builder model ──────────────*/

type focusArea int

const (
	fTools focusArea = iota
	fHelp
	fInput
	fVisual
)

type BuilderModel struct {
	width  int
	height int

	// Responsive design
	responsive *ResponsiveManager
	scroll     *ScrollManager
	layout     LayoutDimensions

	// Tools area
	tools       []string
	toolIndex   int
	toolScroll  int

	// Input area
	domainInput textinput.Model
	argsInput   textinput.Model
	inputMode   int // 0=domain, 1=args

	// Visual area
	mermaidView string

	// State
	g       *graph.DAG
	occ     map[string]int
	focus   focusArea
	selNode string
	layer   int
	msg     string
	msgType int // 0=normal, 1=error, 2=success
}

func NewBuilder(toolNames []string) BuilderModel {
	// Domain input
	domainInp := textinput.New()
	domainInp.Placeholder = "target.com"
	domainInp.Focus()
	domainInp.Width = 30

	// Args input  
	argsInp := textinput.New()
	argsInp.Placeholder = "tool arguments"
	argsInp.Width = 50

	dag := graph.NewDAG()

	return BuilderModel{
		tools:       toolNames,
		domainInput: domainInp,
		argsInput:   argsInp,
		responsive:  NewResponsiveManager(),
		scroll:      NewScrollManager(),
		g:           dag,
		occ:         make(map[string]int),
		focus:       fTools,
		selNode:     "input",
		layer:       0,
		mermaidView: dag.ToMermaid(),
		msg:         "Welcome to Workflow Builder",
		msgType:     0,
	}
}

func (m BuilderModel) Init() tea.Cmd {
	return textinput.Blink
}

/*───────── Update ─────────────────────*/

func (m BuilderModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout = m.responsive.CalculateLayout(m.width, m.height)
		m.updateInputWidths()

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			if m.focus == fInput {
				// Allow normal input when focused on text fields
				break
			}
			return m, tea.Quit

		case "tab":
			m.focus = (m.focus + 1) % 4
			m.updateInputFocus()

		case "shift+tab":
			m.focus = (m.focus + 3) % 4
			m.updateInputFocus()

		case "up":
			switch m.focus {
			case fTools:
				if m.toolIndex > 0 {
					m.toolIndex--
					m.adjustToolScroll()
				}
			case fInput:
				if m.inputMode == 1 {
					m.inputMode = 0
					m.updateInputFocus()
				}
			case fVisual:
				if m.layer > 0 {
					m.layer--
					m.updateSelection()
				} else {
					m.scroll.ScrollUp("visual_y", 1)
				}
			}

		case "down":
			switch m.focus {
			case fTools:
				if m.toolIndex < len(m.tools)-1 {
					m.toolIndex++
					m.adjustToolScroll()
				} else {
					m.scroll.ScrollDown("tools", 1)
				}
			case fInput:
				if m.inputMode == 0 {
					m.inputMode = 1
					m.updateInputFocus()
				}
			case fVisual:
				m.layer++
				m.updateSelection()
			}

		case "left", "right":
			if m.focus == fVisual {
				nodes := m.g.GetLayer(m.layer)
				if len(nodes) > 1 {
					for i, id := range nodes {
						if id == m.selNode {
							if msg.String() == "left" && i > 0 {
								m.selNode = nodes[i-1]
								m.updateArgsForSelection()
							} else if msg.String() == "right" && i < len(nodes)-1 {
								m.selNode = nodes[i+1]
								m.updateArgsForSelection()
							}
							break
						}
					}
				} else {
					// Horizontal scrolling for large workflows
					if msg.String() == "left" {
						m.scroll.ScrollLeft("visual_x", 2)
					} else {
						m.scroll.ScrollRight("visual_x", 2)
					}
				}
			}

		case "enter":
			switch m.focus {
			case fTools:
				m.addSelectedTool()
			case fInput:
				if m.inputMode == 1 && m.selNode != "input" {
					m.commitArgs()
				}
			}

		case "n":
			if m.focus == fTools {
				m.addSelectedTool()
			}

		case "r":
			if m.focus == fVisual && m.selNode != "input" {
				m.removeNode()
			}

		case "c":
			if m.selNode != "input" {
				m.commitArgs()
			}

		case "m":
			if m.focus == fVisual && m.selNode != "input" {
				m.moveNodeMode()
			}

		case "p":
			if m.focus == fVisual && m.selNode != "input" {
				m.toggleParallel()
			}

		case "s":
			m.saveWorkflow()

		case "esc":
			if m.focus == fInput {
				m.domainInput.Blur()
				m.argsInput.Blur()
				m.focus = fTools
			}
		}
	}

	// Update text inputs
	if m.focus == fInput {
		var cmd tea.Cmd
		if m.inputMode == 0 {
			m.domainInput, cmd = m.domainInput.Update(msg)
		} else {
			m.argsInput, cmd = m.argsInput.Update(msg)
		}
		cmds = append(cmds, cmd)
	}

	// Update mermaid view
	m.mermaidView = m.g.ToMermaid()

	return m, tea.Batch(cmds...)
}

/*───────── View ───────────────────────*/

func (m BuilderModel) View() string {
	if m.width == 0 || m.height == 0 {
		return "Resizing..."
	}

	// Use responsive layout calculations
	if m.layout.ToolsWidth == 0 {
		m.layout = m.responsive.CalculateLayout(m.width, m.height)
	}

	// Update scroll bounds
	m.scroll.UpdateBounds(len(m.tools), m.g.MaxX+1, m.g.MaxY+1, 10, m.layout)

	// Render each area with responsive dimensions
	toolsArea := m.renderTools(m.layout.ToolsWidth, m.layout.ToolsHeight)
	helpArea := m.renderHelp(m.layout.HelpWidth, m.layout.HelpHeight)
	inputArea := m.renderInput(m.layout.InputWidth, m.layout.InputHeight)
	visualArea := m.renderVisual(m.layout.VisualWidth, m.layout.VisualHeight)

	// Combine left column
	leftCol := lipgloss.JoinVertical(lipgloss.Top, toolsArea, helpArea)

	// Combine right column  
	rightCol := lipgloss.JoinVertical(lipgloss.Top, inputArea, visualArea)

	// Combine columns
	main := lipgloss.JoinHorizontal(lipgloss.Top, leftCol, rightCol)

	// Add status message
	status := m.renderStatus()
	
	return main + "\n" + status
}

/*───────── Render Methods ─────────────────────*/

func (m BuilderModel) renderTools(width, height int) string {
	var content strings.Builder
	
	// Adaptive title based on screen size
	if m.layout.Config.CompactMode {
		content.WriteString("Tools\n")
	} else {
		content.WriteString("🔧 Available Tools\n")
	}

	visibleHeight := height - 3 // Account for title and borders
	scrollState := m.scroll.GetState()
	start := scrollState.ToolsOffset
	end := min(len(m.tools), start+min(visibleHeight, m.layout.Config.MaxToolsEntries))

	for i := start; i < end; i++ {
		tool := m.tools[i]
		if i == m.toolIndex {
			content.WriteString(selectedStyle.Render(fmt.Sprintf("▶ %s", tool)))
		} else {
			content.WriteString(fmt.Sprintf("  %s", tool))
		}
		content.WriteString("\n")
	}

	// Add scroll indicator if needed
	if m.layout.Config.UseVerticalScroll && len(m.tools) > m.layout.Config.MaxToolsEntries {
		content.WriteString(helpStyle.Render(fmt.Sprintf("(%d/%d)", start+1, len(m.tools))))
	}

	// Fill remaining space
	for i := end - start; i < visibleHeight-1; i++ {
		content.WriteString("\n")
	}

	result := content.String()
	result = strings.TrimSuffix(result, "\n")

	styles := StyleAdaptive(m.layout.ScreenSize)
	style := styles.Border
	if m.focus == fTools {
		style = style.BorderForeground(lipgloss.Color("10"))
	} else {
		style = style.BorderForeground(lipgloss.Color("8"))
	}

	return style.Width(width).Height(height).Render(result)
}

func (m BuilderModel) renderHelp(width, height int) string {
	var help strings.Builder
	
	if m.layout.Config.ShowDetailedHelp {
		help.WriteString("📋 Matrix Controls:\n")
		help.WriteString("tab/shift+tab - focus\n")
		help.WriteString("↑/↓ - navigate\n")
		help.WriteString("←/→ - move in layer\n")
		help.WriteString("n/enter - add tool\n")
		help.WriteString("r - remove node\n")
		help.WriteString("c - commit args\n")
		help.WriteString("m - move node\n")
		help.WriteString("p - toggle parallel\n")
		help.WriteString("s - save workflow\n")
		help.WriteString("q - quit\n\n")
	} else {
		help.WriteString("Controls:\n")
		help.WriteString("tab-focus ↑/↓-nav\n")
		help.WriteString("n-add r-rm s-save\n")
		help.WriteString("q-quit\n\n")
	}
	
	help.WriteString(fmt.Sprintf("Matrix: [%d,%d]\n", m.layer, 0))
	if m.layout.Config.CompactMode {
		help.WriteString(fmt.Sprintf("Sel: %s", m.selNode))
	} else {
		help.WriteString(fmt.Sprintf("Selected: %s", m.selNode))
	}

	content := helpStyle.Render(help.String())

	styles := StyleAdaptive(m.layout.ScreenSize)
	style := styles.Border
	if m.focus == fHelp {
		style = style.BorderForeground(lipgloss.Color("10"))
	} else {
		style = style.BorderForeground(lipgloss.Color("8"))
	}

	return style.Width(width).Height(height).Render(content)
}

func (m BuilderModel) renderInput(width, height int) string {
	var content strings.Builder
	content.WriteString("Configuration\n\n")

	// Domain input
	domainLabel := "Domain: "
	if m.focus == fInput && m.inputMode == 0 {
		domainLabel = selectedStyle.Render("Domain: ")
	}
	content.WriteString(domainLabel + m.domainInput.View() + "\n\n")

	// Args input
	argsLabel := "Args: "
	if m.focus == fInput && m.inputMode == 1 {
		argsLabel = selectedStyle.Render("Args: ")
	}
	if m.selNode != "input" {
		content.WriteString(argsLabel + m.argsInput.View())
	} else {
		content.WriteString(helpStyle.Render("Select a node to edit args"))
	}

	style := inactiveBorder
	if m.focus == fInput {
		style = activeBorder
	}

	return style.Width(width).Height(height).Render(content.String())
}

func (m BuilderModel) renderVisual(width, height int) string {
	var content strings.Builder
	content.WriteString("Matrix Workflow Visualization\n\n")

	// Render matrix view with coordinates
	content.WriteString(fmt.Sprintf("Matrix: %dx%d (X=layers, Y=positions)\n\n", m.g.MaxX+1, m.g.MaxY+1))

	// Show matrix grid
	for l := 0; l <= m.g.MaxX; l++ {
		layerMatrix := m.g.GetLayerMatrix(l)
		
		if len(layerMatrix) == 0 {
			content.WriteString(fmt.Sprintf("L%d: (empty)\n", l))
			continue
		}

		// Layer indicator
		prefix := "  "
		if l == m.layer && m.focus == fVisual {
			prefix = "▶ "
		}
		content.WriteString(fmt.Sprintf("%sL%d: ", prefix, l))

		// Show positions in matrix format
		hasNodes := false
		for pos := 0; pos <= m.g.MaxY; pos++ {
			if nodes, exists := layerMatrix[pos]; exists {
				if hasNodes {
					content.WriteString(" | ")
				}
				content.WriteString(fmt.Sprintf("P%d[", pos))
				for i, node := range nodes {
					if node.ID == m.selNode {
						content.WriteString(selectedStyle.Render(node.ID))
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

	// Show current selection info
	if m.selNode != "input" {
		if coord, exists := m.g.GetCoordinate(m.selNode); exists {
			content.WriteString(fmt.Sprintf("\nSelected: %s at [%d,%d]\n", m.selNode, coord.X, coord.Y))
		}
	}

	// Add compact mermaid preview
	content.WriteString("\n" + helpStyle.Render("Mermaid (LR):") + "\n")
	mermaidLines := strings.Split(m.g.ToCompactMermaid(), "\n")
	previewLines := min(6, len(mermaidLines))
	for i := 0; i < previewLines; i++ {
		if i < len(mermaidLines) {
			content.WriteString(helpStyle.Render(mermaidLines[i]) + "\n")
		}
	}

	style := inactiveBorder
	if m.focus == fVisual {
		style = activeBorder
	}

	return style.Width(width).Height(height).Render(content.String())
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (m BuilderModel) renderStatus() string {
	style := lipgloss.NewStyle()
	switch m.msgType {
	case 1:
		style = errorStyle
	case 2:
		style = successStyle
	}
	return style.Render(m.msg)
}

/*───────── Helper Methods ─────────────────────*/

func (m *BuilderModel) updateInputFocus() {
	m.domainInput.Blur()
	m.argsInput.Blur()

	if m.focus == fInput {
		if m.inputMode == 0 {
			m.domainInput.Focus()
		} else {
			m.argsInput.Focus()
		}
	}
}

func (m *BuilderModel) adjustToolScroll() {
	maxVisible := m.layout.Config.MaxToolsEntries
	if m.toolIndex < m.toolScroll {
		m.toolScroll = m.toolIndex
	} else if m.toolIndex >= m.toolScroll+maxVisible {
		m.toolScroll = m.toolIndex - maxVisible + 1
	}
}

func (m *BuilderModel) updateInputWidths() {
	// Adjust input field widths based on available space
	if m.layout.InputWidth > 50 {
		m.domainInput.Width = min(40, m.layout.InputWidth-20)
		m.argsInput.Width = min(60, m.layout.InputWidth-10)
	} else {
		m.domainInput.Width = min(20, m.layout.InputWidth-10)
		m.argsInput.Width = min(30, m.layout.InputWidth-5)
	}
}

func (m *BuilderModel) updateSelection() {
	nodes := m.g.GetLayer(m.layer)
	if len(nodes) > 0 {
		m.selNode = nodes[0]
		m.updateArgsForSelection()
	}
}

func (m *BuilderModel) updateArgsForSelection() {
	if node := m.g.Nodes[m.selNode]; node != nil {
		m.argsInput.SetValue(node.Args)
	}
}

func (m *BuilderModel) addSelectedTool() {
	if m.toolIndex >= len(m.tools) {
		return
	}

	tool := m.tools[m.toolIndex]
	m.occ[tool]++
	id := fmt.Sprintf("%s-%d", tool, m.occ[tool])
	args := defaultArgs(tool)

	// Use matrix positioning
	position := m.g.GetNextPosition(m.layer+1, "")
	parallel := false // TODO: Add UI control for this
	
	if err := m.g.AddNodeAtPosition(m.selNode, id, tool, args, m.layer+1, position, "", parallel); err != nil {
		m.msg = "Error: " + err.Error()
		m.msgType = 1
	} else {
		m.selNode = id
		m.layer++
		m.argsInput.SetValue(args)
		m.msg = fmt.Sprintf("Added %s at [%d,%d]", id, m.layer, position)
		m.msgType = 2
	}
}

func (m *BuilderModel) removeNode() {
	if m.selNode == "input" {
		m.msg = "Cannot remove input node"
		m.msgType = 1
		return
	}

	if err := m.g.RemoveNode(m.selNode); err != nil {
		m.msg = "Error: " + err.Error()
		m.msgType = 1
	} else {
		m.msg = fmt.Sprintf("Removed %s", m.selNode)
		m.msgType = 2
		m.selNode = "input"
		m.layer = 0
		m.argsInput.SetValue("")
	}
}

func (m *BuilderModel) commitArgs() {
	if node := m.g.Nodes[m.selNode]; node != nil {
		node.Args = m.argsInput.Value()
		m.msg = fmt.Sprintf("Updated args for %s", m.selNode)
		m.msgType = 2
	}
}

func (m *BuilderModel) saveWorkflow() {
	if err := os.MkdirAll("workflows", 0755); err != nil {
		m.msg = "Failed to create workflows directory: " + err.Error()
		m.msgType = 1
		return
	}

	// Validate matrix before saving
	if err := m.g.ValidateMatrix(); err != nil {
		m.msg = "Matrix validation failed: " + err.Error()
		m.msgType = 1
		return
	}

	timestamp := time.Now().Format("20060102-150405")
	base := filepath.Join("workflows", "workflow-"+timestamp)

	if err := os.WriteFile(base+".mmd", []byte(m.g.ToMermaid()), 0644); err != nil {
		m.msg = "Failed to save .mmd file: " + err.Error()
		m.msgType = 1
		return
	}

	if err := os.WriteFile(base+".json", []byte(m.g.ToJSON()), 0644); err != nil {
		m.msg = "Failed to save .json file: " + err.Error()
		m.msgType = 1
		return
	}

	// Also save execution plan
	if err := os.WriteFile(base+".plan", []byte(m.g.ToExecutionPlan()), 0644); err != nil {
		m.msg = "Failed to save execution plan: " + err.Error()
		m.msgType = 1
		return
	}

	m.msg = fmt.Sprintf("Saved workflow: %s.* (matrix: %dx%d)", base, m.g.MaxX+1, m.g.MaxY+1)
	m.msgType = 2
}

func (m *BuilderModel) moveNodeMode() {
	// Simple move implementation - could be enhanced with UI
	if node := m.g.Nodes[m.selNode]; node != nil {
		newLayer := m.layer
		newPosition := (node.Position + 1) % (m.g.MaxY + 2)
		
		if err := m.g.MoveNode(m.selNode, newLayer, newPosition); err != nil {
			m.msg = "Move failed: " + err.Error()
			m.msgType = 1
		} else {
			m.msg = fmt.Sprintf("Moved %s to [%d,%d]", m.selNode, newLayer, newPosition)
			m.msgType = 2
		}
	}
}

func (m *BuilderModel) toggleParallel() {
	if node := m.g.Nodes[m.selNode]; node != nil {
		node.Parallel = !node.Parallel
		status := "sequential"
		if node.Parallel {
			status = "parallel"
		}
		m.msg = fmt.Sprintf("%s is now %s", m.selNode, status)
		m.msgType = 2
	}
}