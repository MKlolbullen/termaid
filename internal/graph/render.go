package graph

import (
	"fmt"
	"sort"
	"strings"
)

// ToMermaid converts the DAG to Mermaid graph TD format.
func (g *DAG) ToMermaid() string {
	var b strings.Builder
	b.WriteString("graph TD\n")

	// stable ordering
	ids := make([]string, 0, len(g.Nodes))
	for id := range g.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	// group by layer (subgraph)
	layerMap := map[int][]string{}
	for _, id := range ids {
		n := g.Nodes[id]
		layerMap[n.Layer] = append(layerMap[n.Layer], id)
	}

	for l := 0; l <= len(layerMap); l++ {
		if nodes, ok := layerMap[l]; ok {
			fmt.Fprintf(&b, "  subgraph L%d\n", l)
			for _, id := range nodes {
				fmt.Fprintf(&b, "    %s\n", id)
			}
			b.WriteString("  end\n")
		}
	}

	// edges
	for _, n := range g.Nodes {
		for _, c := range n.Children {
			fmt.Fprintf(&b, "  %s --> %s\n", n.ID, c)
		}
	}
	return b.String()
}

// ToJSON emits a simple JSON structure identical to saved templates.
func (g *DAG) ToJSON() string {
	var b strings.Builder
	b.WriteString("{\n  \"workflow\": [\n")
	first := true
	for _, n := range g.Nodes {
		if n.ID == g.Root {
			continue // skip input root for export
		}
		if !first {
			b.WriteString(",\n")
		}
		first = false
		fmt.Fprintf(&b,
			"    {\"id\":\"%s\",\"tool\":\"%s\",\"args\":\"%s\",\"children\":%s,\"layer\":%d}",
			n.ID, n.Tool, escapeJSON(n.Args), childrenJSON(n.Children), n.Layer)
	}
	b.WriteString("\n  ]\n}")
	return b.String()
}
func escapeJSON(s string) string { return strings.ReplaceAll(s, `"`, `\"`) }
func childrenJSON(c []string) string {
	if len(c) == 0 {
		return "[]"
	}
	return `["` + strings.Join(c, `","`) + `"]`
}
