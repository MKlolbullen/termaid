package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/MKlolbullen/termaid/internal/graph"
	"github.com/MKlolbullen/termaid/internal/pipeline"
)

/*───────── entryItem ────────────────────────────────────────────────────────*/
// moved to catalog.go

/*───────── Menu model ───────────────────────────────────────────────────────*/

type MenuModel struct{ choices list.Model }

func NewMenu() MenuModel {
	// Count available templates
	templateCount := 0
	if files, err := filepath.Glob("workflows/*.json"); err == nil {
		templateCount = len(files)
	}
	
	// Check if default workflow exists
	defaultExists := "✗"
	if _, err := os.Stat("workflow.json"); err == nil {
		defaultExists = "✓"
	}

	l := list.New([]list.Item{
		entryItem{"🚀 Run Default Workflow", fmt.Sprintf("Execute workflow.json [%s available]", defaultExists)},
		entryItem{"📋 Run Template", fmt.Sprintf("Choose from %d saved templates", templateCount)},
		entryItem{"👁️  Preview Workflow", "View Mermaid diagram of current workflow"},
		entryItem{"🛠️  Create Workflow", "Open visual workflow builder"},
		entryItem{"📊 View Results", "Browse previous execution results"},
		entryItem{"🧹 Clean Workdir", "Remove old execution files"},
		entryItem{"❌ Exit", "Quit Termaid"},
	}, list.NewDefaultDelegate(), 45, 15)
	l.Title = "🔧 Termaid - Bug Bounty Automation"
	return MenuModel{choices: l}
}

func (m MenuModel) Init() tea.Cmd { return nil }

func (m MenuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {

	case tea.KeyMsg:
		if v.String() == "q" || v.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if v.String() != "enter" {
			break
		}

		switch m.choices.SelectedItem().(entryItem).name {

		case "🚀 Run Default Workflow":
			// check if workflow.json exists
			if _, err := os.Stat("workflow.json"); os.IsNotExist(err) {
				return errView(fmt.Errorf("workflow.json not found - please create a workflow first or use a template")), nil
			}
			// ask for domain first
			domInput := textinput.New()
			domInput.Placeholder = "target.com"
			domInput.Focus()
			return domainPrompt{input: domInput, template: "workflow.json"}, nil

		case "📋 Run Template":
			files, _ := filepath.Glob("workflows/*.json")
			return newTmplPicker(files), nil

		case "👁️  Preview Workflow":
			if _, err := os.Stat("workflow.mmd"); os.IsNotExist(err) {
				return errView(fmt.Errorf("workflow.mmd not found - please create a workflow first")), nil
			}
			return previewMermaid()

		case "🛠️  Create Workflow":
			return NewBuilder(catalogueNames()), nil

		case "📊 View Results":
			return m.viewResults()

		case "🧹 Clean Workdir":
			return m.cleanWorkdir()

		case "❌ Exit":
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.choices, cmd = m.choices.Update(msg)
	return m, cmd
}

func (m MenuModel) View() string {
	// Enhanced header with version and status
	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("14")).
		Render("Termaid v1.0") + " " +
		lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Render("- Bug Bounty Automation Platform")

	// Status information
	statusInfo := m.getStatusInfo()
	
	// Footer with keyboard shortcuts
	footer := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Render("↑/↓ navigate • enter select • q quit")

	return header + "\n" + statusInfo + "\n" + m.choices.View() + "\n" + footer
}

/*───────── domainPrompt ──────────────────────────────────────────────────────*/

type domainPrompt struct {
	input    textinput.Model
	template string
}

func (d domainPrompt) Init() tea.Cmd { return nil }

func (d domainPrompt) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.KeyMsg:
		switch v.String() {
		case "enter":
			return runWorkflowWithDomain(d.template, d.input.Value())
		case "esc":
			return NewMenu(), nil
		}
	}
	var cmd tea.Cmd
	d.input, cmd = d.input.Update(msg)
	return d, cmd
}

func (d domainPrompt) View() string {
	return "Enter target domain:\n\n" + d.input.View() + "\n\n[enter] to continue • [esc] cancel"
}

/*───────── helpers ──────────────────────────────────────────────────────────*/

func catalogueNames() []string {
	out := make([]string, len(catalog))
	for i, c := range catalog {
		out[i] = c.Name
	}
	sort.Strings(out)
	return out
}

func LoadWorkflow(path string) (*graph.DAG, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	
	// Try new format first
	var newFormat struct {
		Version   string                      `json:"version"`
		Matrix    struct {
			MaxX int `json:"max_x"`
			MaxY int `json:"max_y"`
		} `json:"matrix"`
		Subgraphs []struct {
			ID       string   `json:"id"`
			Name     string   `json:"name"`
			Parallel bool     `json:"parallel"`
			Nodes    []string `json:"nodes"`
		} `json:"subgraphs"`
		Workflow []graph.Node `json:"workflow"`
	}
	
	if err := json.Unmarshal(data, &newFormat); err == nil && newFormat.Version == "2.0" {
		// New format with matrix positioning
		g := graph.NewDAG()
		g.MaxX = newFormat.Matrix.MaxX
		g.MaxY = newFormat.Matrix.MaxY
		
		// Load subgraphs
		for _, sg := range newFormat.Subgraphs {
			g.Subgraphs[sg.ID] = &graph.SubgraphInfo{
				ID:       sg.ID,
				Name:     sg.Name,
				Parallel: sg.Parallel,
				Nodes:    sg.Nodes,
				Matrix:   make(map[string]graph.Coordinate),
			}
		}
		
		// Load nodes
		for _, n := range newFormat.Workflow {
			cp := n
			g.Nodes[n.ID] = &cp
			g.Matrix[graph.Coordinate{X: n.Layer, Y: n.Position}] = append(
				g.Matrix[graph.Coordinate{X: n.Layer, Y: n.Position}], &cp)
		}
		
		return g, nil
	}
	
	// Fallback to old format
	var oldFormat struct {
		Workflow []graph.Node `json:"workflow"`
	}
	if err := json.Unmarshal(data, &oldFormat); err != nil {
		return nil, err
	}
	
	g := graph.NewDAG()
	for _, n := range oldFormat.Workflow {
		cp := n
		// Convert old format: no position field, so auto-assign
		if cp.Position == 0 && cp.ID != "input" {
			cp.Position = g.GetNextPosition(cp.Layer, cp.Subgraph)
		}
		g.Nodes[n.ID] = &cp
		g.Matrix[graph.Coordinate{X: cp.Layer, Y: cp.Position}] = append(
			g.Matrix[graph.Coordinate{X: cp.Layer, Y: cp.Position}], &cp)
		g.UpdateBounds(cp.Layer, cp.Position)
	}
	
	return g, nil
}

func runWorkflow(path string) (tea.Model, tea.Cmd) {
	return runWorkflowWithDomain(path, "")
}

func runWorkflowWithDomain(path, domain string) (tea.Model, tea.Cmd) {
	if domain == "" {
		return errView(fmt.Errorf("domain cannot be empty")), nil
	}
	
	dag, err := LoadWorkflow(path)
	if err != nil {
		if os.IsNotExist(err) {
			return errView(fmt.Errorf("workflow file '%s' not found - please create a workflow first", path)), nil
		}
		return errView(fmt.Errorf("failed to load workflow '%s': %w", path, err)), nil
	}
	
	cats := dagToCategories(dag)
	if len(cats) == 0 {
		return errView(fmt.Errorf("workflow '%s' contains no valid tools to execute", path)), nil
	}

	ch := make(chan pipeline.Status, 128)
	go func() {
		if err := pipeline.Run(context.Background(), domain, "workdir", cats, 6, ch); err != nil {
			ch <- pipeline.Status{
				Type: pipeline.StatusError,
				Tool: "pipeline",
				Err:  err,
			}
		}
		close(ch)
	}()
	return New(cats, ch), nil
}

func previewMermaid() (tea.Model, tea.Cmd) {
	raw, err := os.ReadFile("workflow.mmd")
	if err != nil {
		return errView(fmt.Errorf("failed to read workflow.mmd: %w", err)), nil
	}
	
	// Check if glow is available
	if _, err := exec.LookPath("glow"); err != nil {
		return errView(fmt.Errorf("glow command not found - please install glow to preview mermaid diagrams")), nil
	}
	
	md := "```mermaid\n" + string(raw) + "\n```"
	cmd := exec.Command("glow", "-")
	cmd.Stdin = strings.NewReader(md)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return errView(fmt.Errorf("failed to run glow: %w", err)), nil
	}
	return NewMenu(), nil
}

func dagToCategories(g *graph.DAG) []pipeline.Category {
	if g.MaxX == 0 {
		return []pipeline.Category{}
	}
	
	var cats []pipeline.Category
	
	// Use execution order from matrix positioning
	executionOrder := g.GetExecutionOrder()
	
	for stepNum, nodeGroup := range executionOrder {
		if len(nodeGroup) == 0 {
			continue
		}
		
		var tools []pipeline.Tool
		categoryName := fmt.Sprintf("step-%d", stepNum+1)
		
		// Check if this is a parallel group
		isParallel := len(nodeGroup) > 1
		if !isParallel && len(nodeGroup) == 1 {
			if node, exists := g.Nodes[nodeGroup[0]]; exists {
				isParallel = node.Parallel
			}
		}
		
		for _, nodeID := range nodeGroup {
			if node, exists := g.Nodes[nodeID]; exists && node.ID != g.Root {
				tools = append(tools, pipeline.Tool{
					Name:     node.ID,
					Command:  node.Tool,
					Args:     strings.Fields(node.Args),
					Output:   fmt.Sprintf("%s_%s.txt", node.Tool, node.ID),
					Parallel: isParallel,
				})
			}
		}
		
		if len(tools) > 0 {
			// Add layer info to category name for clarity
			if len(nodeGroup) > 0 {
				if node, exists := g.Nodes[nodeGroup[0]]; exists {
					categoryName = fmt.Sprintf("layer-%d-step-%d", node.Layer, stepNum+1)
				}
			}
			
			cats = append(cats, pipeline.Category{
				Name:  categoryName,
				Tools: tools,
			})
		}
	}
	
	return cats
}

/*───────── New menu methods ─────────────────────────────────────────────────*/

func (m MenuModel) getStatusInfo() string {
	var status []string
	
	// Check workflow status
	if _, err := os.Stat("workflow.json"); err == nil {
		status = append(status, "✓ Default workflow ready")
	} else {
		status = append(status, "⚠ No default workflow")
	}
	
	// Count templates
	if files, err := filepath.Glob("workflows/*.json"); err == nil && len(files) > 0 {
		status = append(status, fmt.Sprintf("✓ %d templates available", len(files)))
	} else {
		status = append(status, "⚠ No templates found")
	}
	
	// Check for recent results
	if _, err := os.Stat("workdir"); err == nil {
		status = append(status, "✓ Previous results available")
	}
	
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Render(strings.Join(status, " | "))
}

func (m MenuModel) viewResults() (tea.Model, tea.Cmd) {
	// Check if workdir exists
	if _, err := os.Stat("workdir"); os.IsNotExist(err) {
		return errView(fmt.Errorf("no results found - run a workflow first")), nil
	}
	
	// Open file browser or list recent runs
	return errView(fmt.Errorf("results viewer not yet implemented - check ./workdir manually")), nil
}

func (m MenuModel) cleanWorkdir() (tea.Model, tea.Cmd) {
	if err := os.RemoveAll("workdir"); err != nil {
		return errView(fmt.Errorf("failed to clean workdir: %w", err)), nil
	}
	
	// Also clean log files
	if logs, err := filepath.Glob("run-*.log"); err == nil {
		for _, log := range logs {
			os.Remove(log)
		}
	}
	
	return errView(fmt.Errorf("workdir cleaned successfully")), nil
}

/*───────── errorModel ───────────────────────────────────────────────────────*/

type errorModel struct{ err error }

func errView(e error) tea.Model { return errorModel{e} }

func (e errorModel) Init() tea.Cmd                       { return nil }
func (e errorModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return e, tea.Quit }
func (e errorModel) View() string                        { return "Error: " + e.err.Error() }
