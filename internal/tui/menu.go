package tui

import (
	"context"
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

type MenuModel struct{ choices list.Model }

func NewMenu() MenuModel {
	templateCount := 0
	if files, err := filepath.Glob("workflows/*.json"); err == nil {
		templateCount = len(files)
	}
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
			if _, err := os.Stat("workflow.json"); os.IsNotExist(err) {
				return errView(fmt.Errorf("workflow.json not found - please create a workflow first or use a template")), nil
			}
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
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14")).Render("Termaid v1.2") + " " +
		lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("- Typed DAG Security Automation")
	statusInfo := m.getStatusInfo()
	footer := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("↑/↓ navigate • enter select • q quit")
	return header + "\n" + statusInfo + "\n" + m.choices.View() + "\n" + footer
}

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

func catalogueNames() []string {
	out := make([]string, len(catalog))
	for i, c := range catalog {
		out[i] = c.Name
	}
	sort.Strings(out)
	return out
}

// LoadWorkflow remains the compatibility entry point used by the visual
// builder/tests, but delegates to the v2/v3 semantic loader.
func LoadWorkflow(path string) (*graph.DAG, error) { return LoadWorkflowV3(path) }

func runWorkflow(path string) (tea.Model, tea.Cmd) { return runWorkflowWithDomain(path, "") }

func runWorkflowWithDomain(path, domain string) (tea.Model, tea.Cmd) {
	if domain == "" {
		return errView(fmt.Errorf("domain cannot be empty")), nil
	}
	dag, err := LoadWorkflowV3(path)
	if err != nil {
		if os.IsNotExist(err) {
			return errView(fmt.Errorf("workflow file '%s' not found - please create a workflow first", path)), nil
		}
		return errView(fmt.Errorf("failed to load workflow '%s': %w", path, err)), nil
	}
	if err := dag.Validate(); err != nil {
		return errView(fmt.Errorf("workflow '%s' is invalid: %w", path, err)), nil
	}
	cats := dagToCategories(dag)
	if len(cats) == 0 {
		return errView(fmt.Errorf("workflow '%s' contains no executable nodes", path)), nil
	}
	ch := make(chan pipeline.Status, 128)
	go func() {
		if err := pipeline.RunDAG(context.Background(), domain, "workdir", dag, pipeline.RunConfig{Concurrency: 6}, ch); err != nil {
			ch <- pipeline.Status{Type: pipeline.StatusError, Tool: "pipeline", Category: "scheduler", Err: err}
		}
		close(ch)
	}()
	return New(cats, ch), nil
}

func previewMermaid() (tea.Model, tea.Cmd) {
	// Prefer the JSON source so semantic conditions/kinds are visible. Fall back
	// to workflow.mmd for legacy projects.
	if _, err := os.Stat("workflow.json"); err == nil {
		mmd, loadErr := MermaidForWorkflow("workflow.json")
		if loadErr == nil {
			return previewMermaidText(mmd)
		}
	}
	raw, err := os.ReadFile("workflow.mmd")
	if err != nil {
		return errView(fmt.Errorf("failed to read workflow.mmd: %w", err)), nil
	}
	return previewMermaidText(string(raw))
}

func previewMermaidText(raw string) (tea.Model, tea.Cmd) {
	if _, err := exec.LookPath("glow"); err != nil {
		return errView(fmt.Errorf("glow command not found - please install glow to preview mermaid diagrams")), nil
	}
	md := "```mermaid\n" + raw + "\n```"
	cmd := exec.Command("glow", "-")
	cmd.Stdin = strings.NewReader(md)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return errView(fmt.Errorf("failed to run glow: %w", err)), nil
	}
	return NewMenu(), nil
}

// dagToCategories is now presentation-only. Execution order comes from RunDAG.
func dagToCategories(g *graph.DAG) []pipeline.Category {
	var cats []pipeline.Category
	for layer := 1; layer <= g.MaxX; layer++ {
		ids := g.GetLayer(layer)
		if len(ids) == 0 {
			continue
		}
		var tools []pipeline.Tool
		for _, id := range ids {
			n := g.Nodes[id]
			if n == nil || id == g.Root {
				continue
			}
			command := n.Tool
			if command == "" {
				command = "builtin:" + string(n.EffectiveKind())
			}
			tools = append(tools, pipeline.Tool{
				Name: id, Command: command, Args: strings.Fields(n.Args),
				Output: fmt.Sprintf("%s_%s.txt", command, id), Parallel: len(ids) > 1,
			})
		}
		if len(tools) > 0 {
			cats = append(cats, pipeline.Category{Name: fmt.Sprintf("layer-%d", layer), Tools: tools})
		}
	}
	return cats
}

func (m MenuModel) getStatusInfo() string {
	var status []string
	if _, err := os.Stat("workflow.json"); err == nil {
		status = append(status, "✓ Default workflow ready")
	} else {
		status = append(status, "⚠ No default workflow")
	}
	if files, err := filepath.Glob("workflows/*.json"); err == nil && len(files) > 0 {
		status = append(status, fmt.Sprintf("✓ %d templates available", len(files)))
	} else {
		status = append(status, "⚠ No templates found")
	}
	if _, err := os.Stat("workdir"); err == nil {
		status = append(status, "✓ Previous results available")
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(strings.Join(status, " | "))
}

func (m MenuModel) viewResults() (tea.Model, tea.Cmd) {
	if _, err := os.Stat("workdir"); os.IsNotExist(err) {
		return errView(fmt.Errorf("no results found - run a workflow first")), nil
	}
	return errView(fmt.Errorf("results viewer not yet implemented - check ./workdir manually")), nil
}

func (m MenuModel) cleanWorkdir() (tea.Model, tea.Cmd) {
	if err := os.RemoveAll("workdir"); err != nil {
		return errView(fmt.Errorf("failed to clean workdir: %w", err)), nil
	}
	if logs, err := filepath.Glob("run-*.log"); err == nil {
		for _, log := range logs {
			_ = os.Remove(log)
		}
	}
	return errView(fmt.Errorf("workdir cleaned successfully")), nil
}

type errorModel struct{ err error }
func errView(e error) tea.Model { return errorModel{e} }
func (e errorModel) Init() tea.Cmd                       { return nil }
func (e errorModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return e, tea.Quit }
func (e errorModel) View() string                        { return "Error: " + e.err.Error() }
