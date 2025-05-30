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
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/MKlolbullen/termaid/internal/graph"
	"github.com/MKlolbullen/termaid/internal/pipeline"
)

/* list.Item wrapper */


/* Menu model */

type MenuModel struct{ choices list.Model }

func NewMenu() MenuModel {
	l := list.New([]list.Item{
		entryItem{"Run Workflow", "Execute workflow.json"},
		entryItem{"Run Template", "Select from /workflows"},
		entryItem{"Preview Workflow", "Show Mermaid DAG"},
		entryItem{"Create Workflow", "Open DAG builder"},
		entryItem{"Exit", "Quit"},
	}, list.NewDefaultDelegate(), 32, 12)
	l.Title = "Main Menu"
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

		case "Run Workflow":
			return runWorkflow("workflow.json")

		case "Run Template":
			files, _ := filepath.Glob("workflows/*.json")
			return newTmplPicker(files), nil

		case "Preview Workflow":
			return previewMermaid()

		case "Create Workflow":
			return NewBuilder(catalogueNames()), nil

		case "Exit":
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.choices, cmd = m.choices.Update(msg)
	return m, cmd
}

func (m MenuModel) View() string {
	title := lipgloss.NewStyle().Bold(true).
		Foreground(lipgloss.Color("14")).Render("termaid")
	return title + "\n\n" + m.choices.View()
}

/* helpers ------------------------------------------------------- */

func catalogueNames() []string {
	out := make([]string, len(catalog))
	for i, c := range catalog {
		out[i] = c.Name
	}
	sort.Strings(out)
	return out
}

func loadWorkflow(path string) (*graph.DAG, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var wrap struct {
		Workflow []graph.Node `json:"workflow"`
	}
	if err := json.Unmarshal(data, &wrap); err != nil {
		return nil, err
	}
	g := &graph.DAG{Nodes: map[string]*graph.Node{
		"input": {ID: "input", Tool: "input"},
	}}
	for _, n := range wrap.Workflow {
		cp := n
		g.Nodes[n.ID] = &cp
	}
	return g, nil
}

func runWorkflow(path string) (tea.Model, tea.Cmd) {
	dag, err := loadWorkflow(path)
	if err != nil {
		return errView(err), nil
	}
	cats := dagToCategories(dag)

	ch := make(chan pipeline.Status, 128)
	go func() {
		_ = pipeline.Run(context.Background(), "input", "workdir", cats, 6, ch)
		close(ch)
	}()
	return New(cats, ch), nil
}

func previewMermaid() (tea.Model, tea.Cmd) {
	raw, err := os.ReadFile("workflow.mmd")
	if err != nil {
		return errView(err), nil
	}
	md := "```mermaid\n" + string(raw) + "\n```"
	cmd := exec.Command("glow", "-")
	cmd.Stdin = strings.NewReader(md)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	_ = cmd.Run()
	return NewMenu(), nil
}

func dagToCategories(g *graph.DAG) []pipeline.Category {
	max := 0
	for _, n := range g.Nodes {
		if n.Layer > max {
			max = n.Layer
		}
	}
	cats := make([]pipeline.Category, max)
	for l := 1; l <= max; l++ {
		var tools []pipeline.Tool
		for _, n := range g.Nodes {
			if n.Layer == l {
				tools = append(tools, pipeline.Tool{
					Name:     n.ID, // node ID
					Command:  n.Tool,
					Args:     strings.Fields(n.Args),
					Output:   fmt.Sprintf("%s_%s.txt", n.Tool, n.ID), // unique output
					Parallel: true,
				})
			}
		}
		cats[l-1] = pipeline.Category{
			Name:  fmt.Sprintf("layer-%d", l),
			Tools: tools,
		}
	}
	return cats
}

/* error model */

type errorModel struct{ err error }

func errView(e error) tea.Model { return errorModel{e} }

func (e errorModel) Init() tea.Cmd                       { return nil }
func (e errorModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return e, tea.Quit }
func (e errorModel) View() string                        { return "Error: " + e.err.Error() }
