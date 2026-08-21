package tui

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/MKlolbullen/termaid/internal/graph"
)

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
