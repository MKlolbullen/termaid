package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/MKlolbullen/termaid/internal/graph"
)

// LoadWorkflowAny loads a workflow from either a semantic JSON file or a Mermaid
// chart (.mmd/.mermaid). Mermaid charts are parsed via graph.ParseMermaid and
// then hydrated from the tool catalog so a minimally labelled chart still runs.
// This is the entry point every run/validate/preview path should use.
func LoadWorkflowAny(path string) (*graph.DAG, error) {
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, ".mmd") || strings.HasSuffix(lower, ".mermaid") {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		g, err := graph.ParseMermaid(string(data))
		if err != nil {
			return nil, fmt.Errorf("parse mermaid %q: %w", path, err)
		}
		hydrateFromCatalog(g)
		return g, nil
	}
	return LoadWorkflowV3(path)
}

// hydrateFromCatalog fills in the tool/args a Mermaid chart may leave implicit:
// a node id like "assetfinder-1" resolves to the "assetfinder" tool, and an empty
// argument string inherits that tool's catalog default. Nodes that already carry
// a tool/args are left untouched.
func hydrateFromCatalog(g *graph.DAG) {
	for _, n := range g.Nodes {
		if n.ID == g.Root || n.EffectiveKind() != graph.NodeKindWorker {
			continue
		}
		if strings.TrimSpace(n.Tool) == "" {
			if cand := stripToolSuffix(n.ID); cand != "" {
				if _, ok := catalogMap[cand]; ok {
					n.Tool = cand
				}
			}
		}
		if strings.TrimSpace(n.Args) == "" {
			if def := defaultArgs(n.Tool); def != "" {
				n.Args = def
			}
		}
	}
}

// stripToolSuffix removes a trailing "-<number>" occurrence suffix from a node
// id (e.g. "subfinder-1" -> "subfinder"), matching how the builder names nodes.
func stripToolSuffix(id string) string {
	i := strings.LastIndex(id, "-")
	if i <= 0 || i == len(id)-1 {
		return id
	}
	for _, r := range id[i+1:] {
		if r < '0' || r > '9' {
			return id
		}
	}
	return id[:i]
}

// suffixNumber returns the numeric value of a trailing "-<number>" in an id (e.g.
// "subfinder-3" -> 3), or 0 when there is none. Used to reseed the builder's
// per-tool occurrence counter after loading a workflow.
func suffixNumber(id string) int {
	i := strings.LastIndex(id, "-")
	if i <= 0 || i == len(id)-1 {
		return 0
	}
	n := 0
	for _, r := range id[i+1:] {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// LoadWorkflowV3 loads both legacy v2 workflows and semantic v3 workflows.
// v2 Children relationships are promoted to explicit edges automatically.
func LoadWorkflowV3(path string) (*graph.DAG, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Version string               `json:"version"`
		Policy  graph.WorkflowPolicy `json:"policy"`
		Matrix  struct {
			MaxX int `json:"max_x"`
			MaxY int `json:"max_y"`
		} `json:"matrix"`
		Subgraphs []struct {
			ID          string   `json:"id"`
			Name        string   `json:"name"`
			Description string   `json:"description"`
			Parallel    bool     `json:"parallel"`
			Nodes       []string `json:"nodes"`
		} `json:"subgraphs"`
		Edges    []graph.Edge `json:"edges"`
		Workflow []graph.Node `json:"workflow"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("decode workflow: %w", err)
	}
	if len(raw.Workflow) == 0 {
		return nil, fmt.Errorf("workflow contains no nodes")
	}

	g := graph.NewDAG()
	if raw.Version != "" {
		g.Version = raw.Version
	}
	g.Policy = raw.Policy
	g.MaxX, g.MaxY = raw.Matrix.MaxX, raw.Matrix.MaxY

	for _, sg := range raw.Subgraphs {
		g.Subgraphs[sg.ID] = &graph.SubgraphInfo{
			ID:          sg.ID,
			Name:        sg.Name,
			Description: sg.Description,
			Parallel:    sg.Parallel,
			Nodes:       append([]string(nil), sg.Nodes...),
			Matrix:      make(map[string]graph.Coordinate),
		}
	}

	for i := range raw.Workflow {
		n := raw.Workflow[i]
		if n.ID == "" {
			return nil, fmt.Errorf("workflow node %d has empty id", i)
		}
		if n.Kind == "" {
			n.Kind = graph.NodeKindWorker
		}
		cp := n
		g.Nodes[cp.ID] = &cp
		g.Matrix[graph.Coordinate{X: cp.Layer, Y: cp.Position}] = append(g.Matrix[graph.Coordinate{X: cp.Layer, Y: cp.Position}], &cp)
		g.UpdateBounds(cp.Layer, cp.Position)
		if cp.Subgraph != "" {
			if sg := g.Subgraphs[cp.Subgraph]; sg != nil {
				sg.Matrix[cp.ID] = graph.Coordinate{X: cp.SubX, Y: cp.SubY}
				if !containsNodeID(sg.Nodes, cp.ID) {
					sg.Nodes = append(sg.Nodes, cp.ID)
				}
			}
		}
	}

	g.Edges = append([]graph.Edge(nil), raw.Edges...)
	g.EnsureEdges()
	return g, nil
}

func containsNodeID(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
