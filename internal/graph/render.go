package graph

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ToMermaid converts the DAG to a semantic Mermaid flowchart. Matrix/subgraph
// placement remains a presentation concern; execution is represented by the
// actual dependency edges and their optional conditions.
func (g *DAG) ToMermaid() string {
	g.EnsureEdges()
	var b strings.Builder
	b.WriteString("flowchart LR\n")
	b.WriteString("  classDef worker fill:#16202a,stroke:#8aa4b8,color:#e6edf3\n")
	b.WriteString("  classDef merge fill:#14251f,stroke:#3fb950,color:#e6edf3\n")
	b.WriteString("  classDef gate fill:#2b2414,stroke:#d29922,color:#e6edf3\n")
	b.WriteString("  classDef transform fill:#17243a,stroke:#58a6ff,color:#e6edf3\n")
	b.WriteString("  classDef checkpoint fill:#241b31,stroke:#a371f7,color:#e6edf3\n")
	b.WriteString("  classDef sink fill:#2b1d22,stroke:#f778ba,color:#e6edf3\n")
	b.WriteString("  classDef manual fill:#312219,stroke:#ffa657,color:#e6edf3\n")
	b.WriteString("  classDef source fill:#19232d,stroke:#79c0ff,color:#e6edf3\n")

	g.generateSubgraphs(&b)
	g.generateLayers(&b)
	g.generateEdges(&b)
	g.generateClasses(&b)
	return b.String()
}

func (g *DAG) generateSubgraphs(b *strings.Builder) {
	ids := make([]string, 0, len(g.Subgraphs))
	for id := range g.Subgraphs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, sgID := range ids {
		sg := g.Subgraphs[sgID]
		if len(sg.Nodes) == 0 {
			continue
		}
		fmt.Fprintf(b, "  subgraph %s[\"%s\"]\n", safeMermaidID(sgID), escapeMermaid(sg.Name))
		if sg.Parallel {
			b.WriteString("    direction TB\n")
		}
		for _, node := range g.GetSubgraphNodes(sgID) {
			writeNode(b, node, "    ")
		}
		b.WriteString("  end\n")
	}
}

func (g *DAG) generateLayers(b *strings.Builder) {
	for layer := 0; layer <= g.MaxX; layer++ {
		layerMatrix := g.GetLayerMatrix(layer)
		if len(layerMatrix) == 0 {
			continue
		}
		var rendered []*Node
		for _, nodes := range layerMatrix {
			for _, n := range nodes {
				if n.Subgraph == "" {
					rendered = append(rendered, n)
				}
			}
		}
		if len(rendered) == 0 {
			continue
		}
		sort.Slice(rendered, func(i, j int) bool {
			if rendered[i].Position != rendered[j].Position {
				return rendered[i].Position < rendered[j].Position
			}
			return rendered[i].ID < rendered[j].ID
		})
		fmt.Fprintf(b, "  subgraph L%d[\"Layer %d\"]\n", layer, layer)
		b.WriteString("    direction TB\n")
		for _, n := range rendered {
			writeNode(b, n, "    ")
		}
		b.WriteString("  end\n")
	}
}

func writeNode(b *strings.Builder, node *Node, indent string) {
	label := nodeLabel(node)
	id := safeMermaidID(node.ID)
	switch node.EffectiveKind() {
	case NodeKindGate:
		fmt.Fprintf(b, "%s%s{\"%s\"}\n", indent, id, label)
	case NodeKindMerge:
		fmt.Fprintf(b, "%s%s((\"%s\"))\n", indent, id, label)
	case NodeKindCheckpoint:
		fmt.Fprintf(b, "%s%s([\"%s\"])\n", indent, id, label)
	case NodeKindSink:
		fmt.Fprintf(b, "%s%s[[\"%s\"]]\n", indent, id, label)
	default:
		fmt.Fprintf(b, "%s%s[\"%s\"]\n", indent, id, label)
	}
}

func nodeLabel(node *Node) string {
	if node.ID == "input" || node.EffectiveKind() == NodeKindSource {
		return "Target seed"
	}
	name := node.Tool
	if strings.TrimSpace(name) == "" {
		name = string(node.EffectiveKind())
	}
	parts := []string{escapeMermaid(name)}
	if node.EffectiveKind() != NodeKindWorker {
		parts = append(parts, "«"+string(node.EffectiveKind())+"»")
	}
	if len(node.Outputs) > 0 {
		outs := make([]string, len(node.Outputs))
		for i, t := range node.Outputs {
			outs[i] = string(t)
		}
		parts = append(parts, "→ "+strings.Join(outs, ","))
	} else if node.Args != "" {
		parts = append(parts, truncateArgs(node.Args))
	}
	return strings.Join(parts, "\\n")
}

func (g *DAG) generateEdges(b *strings.Builder) {
	edges := append([]Edge(nil), g.Edges...)
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		return edges[i].To < edges[j].To
	})
	for _, e := range edges {
		from, to := safeMermaidID(e.From), safeMermaidID(e.To)
		label := strings.TrimSpace(e.Label)
		if label == "" {
			label = strings.TrimSpace(e.Condition)
		}
		if label == "" || label == "always" {
			fmt.Fprintf(b, "  %s --> %s\n", from, to)
		} else {
			fmt.Fprintf(b, "  %s -. \"%s\" .-> %s\n", from, escapeMermaid(label), to)
		}
	}
}

func (g *DAG) generateClasses(b *strings.Builder) {
	ids := make([]string, 0, len(g.Nodes))
	for id := range g.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		n := g.Nodes[id]
		kind := n.EffectiveKind()
		if id == g.Root {
			kind = NodeKindSource
		}
		fmt.Fprintf(b, "  class %s %s\n", safeMermaidID(id), kind)
	}
}

func truncateArgs(args string) string {
	args = strings.ReplaceAll(args, "\"", "'")
	if len(args) > 42 {
		return args[:39] + "..."
	}
	return args
}

// ToJSON emits the v3 workflow format. Using encoding/json instead of manual
// string construction ensures new semantic fields cannot silently disappear.
func (g *DAG) ToJSON() string {
	g.EnsureEdges()
	type matrixExport struct {
		MaxX int `json:"max_x"`
		MaxY int `json:"max_y"`
	}
	type subgraphExport struct {
		ID          string   `json:"id"`
		Name        string   `json:"name"`
		Description string   `json:"description,omitempty"`
		Parallel    bool     `json:"parallel"`
		Nodes       []string `json:"nodes"`
	}
	type workflowExport struct {
		Version   string             `json:"version"`
		Matrix    matrixExport       `json:"matrix"`
		Policy    WorkflowPolicy     `json:"policy,omitempty"`
		Subgraphs []subgraphExport   `json:"subgraphs,omitempty"`
		Edges     []Edge             `json:"edges,omitempty"`
		Workflow  []*Node            `json:"workflow"`
	}

	var subgraphs []subgraphExport
	sgIDs := make([]string, 0, len(g.Subgraphs))
	for id := range g.Subgraphs {
		sgIDs = append(sgIDs, id)
	}
	sort.Strings(sgIDs)
	for _, id := range sgIDs {
		sg := g.Subgraphs[id]
		nodes := append([]string(nil), sg.Nodes...)
		sort.Strings(nodes)
		subgraphs = append(subgraphs, subgraphExport{ID: sg.ID, Name: sg.Name, Description: sg.Description, Parallel: sg.Parallel, Nodes: nodes})
	}

	var nodes []*Node
	for _, n := range g.Nodes {
		if n.ID != g.Root {
			nodes = append(nodes, n)
		}
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Layer != nodes[j].Layer {
			return nodes[i].Layer < nodes[j].Layer
		}
		if nodes[i].Position != nodes[j].Position {
			return nodes[i].Position < nodes[j].Position
		}
		return nodes[i].ID < nodes[j].ID
	})

	edges := append([]Edge(nil), g.Edges...)
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		return edges[i].To < edges[j].To
	})

	payload := workflowExport{
		Version:   "3.0",
		Matrix:    matrixExport{MaxX: g.MaxX, MaxY: g.MaxY},
		Policy:    g.Policy,
		Subgraphs: subgraphs,
		Edges:     edges,
		Workflow:  nodes,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"version":"3.0","error":%q}`, err.Error())
	}
	return string(data)
}

// ToCompactMermaid generates a minimal dependency-only diagram.
func (g *DAG) ToCompactMermaid() string {
	g.EnsureEdges()
	var b strings.Builder
	b.WriteString("flowchart LR\n")
	ids := make([]string, 0, len(g.Nodes))
	for id := range g.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		n := g.Nodes[id]
		if id == g.Root {
			fmt.Fprintf(&b, "  %s([Target])\n", safeMermaidID(id))
		} else {
			fmt.Fprintf(&b, "  %s[%s]\n", safeMermaidID(id), escapeMermaid(n.Tool))
		}
	}
	for _, e := range g.Edges {
		fmt.Fprintf(&b, "  %s --> %s\n", safeMermaidID(e.From), safeMermaidID(e.To))
	}
	return b.String()
}

// ToExecutionPlan reports the dependency-based order rather than implying
// that visual layers are execution barriers.
func (g *DAG) ToExecutionPlan() string {
	order, err := g.TopologicalOrder()
	if err != nil {
		return "Execution Plan:\n==============\n\nERROR: " + err.Error() + "\n"
	}
	var b strings.Builder
	b.WriteString("Execution Plan:\n==============\n\n")
	step := 0
	for _, id := range order {
		if id == g.Root {
			continue
		}
		step++
		n := g.Nodes[id]
		fmt.Fprintf(&b, "Step %d: %s (%s, %s)\n", step, n.Tool, id, n.EffectiveKind())
		if parents := g.Parents(id); len(parents) > 0 {
			fmt.Fprintf(&b, "  depends on: %s\n", strings.Join(parents, ", "))
		}
		if n.Condition != "" {
			fmt.Fprintf(&b, "  condition: %s\n", n.Condition)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func safeMermaidID(s string) string {
	var b strings.Builder
	for i, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' || (i > 0 && r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	if b.Len() == 0 {
		return "node"
	}
	return b.String()
}

func escapeMermaid(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
