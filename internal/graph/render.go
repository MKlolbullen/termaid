package graph

import (
	"fmt"
	"sort"
	"strings"
)

// ToMermaid converts the DAG to Mermaid graph LR format with matrix positioning.
func (g *DAG) ToMermaid() string {
	var b strings.Builder
	b.WriteString("graph LR\n")

	// Generate subgraphs first
	g.generateSubgraphs(&b)

	// Generate main layer structure
	g.generateLayers(&b)

	// Generate edges
	g.generateEdges(&b)

	return b.String()
}

// generateSubgraphs creates subgraph definitions for parallel execution groups
func (g *DAG) generateSubgraphs(b *strings.Builder) {
	for sgID, sg := range g.Subgraphs {
		if len(sg.Nodes) > 0 {
			fmt.Fprintf(b, "  subgraph %s[\"%s\"]\n", sgID, sg.Name)
			
			// Sort nodes by subgraph coordinates
			nodes := g.GetSubgraphNodes(sgID)
			for _, node := range nodes {
				fmt.Fprintf(b, "    %s[\"%s\\n%s\"]\n", node.ID, node.Tool, truncateArgs(node.Args))
			}
			
			b.WriteString("  end\n")
		}
	}
}

// generateLayers creates layer-based node definitions with matrix positioning
func (g *DAG) generateLayers(b *strings.Builder) {
	for layer := 0; layer <= g.MaxX; layer++ {
		layerMatrix := g.GetLayerMatrix(layer)
		
		if len(layerMatrix) == 0 {
			continue
		}
		
		// Create layer subgraph
		fmt.Fprintf(b, "  subgraph L%d[\"Layer %d\"]\n", layer, layer)
		
		// Process positions in order
		for pos := 0; pos <= g.MaxY; pos++ {
			if nodes, exists := layerMatrix[pos]; exists {
				if len(nodes) == 1 {
					// Single node at position
					node := nodes[0]
					if node.Subgraph == "" { // Only render if not in a subgraph
						fmt.Fprintf(b, "    %s[\"%s\\n%s\"]\n", 
							node.ID, node.Tool, truncateArgs(node.Args))
					}
				} else if len(nodes) > 1 {
					// Multiple nodes at same position (parallel)
					fmt.Fprintf(b, "    subgraph P%d_%d[\"Parallel Group\"]\n", layer, pos)
					for _, node := range nodes {
						if node.Subgraph == "" {
							fmt.Fprintf(b, "      %s[\"%s\\n%s\"]\n", 
								node.ID, node.Tool, truncateArgs(node.Args))
						}
					}
					b.WriteString("    end\n")
				}
			}
		}
		
		b.WriteString("  end\n")
	}
}

// generateEdges creates all the connections between nodes
func (g *DAG) generateEdges(b *strings.Builder) {
	// Sort nodes for consistent edge ordering
	var sortedNodes []*Node
	for _, node := range g.Nodes {
		sortedNodes = append(sortedNodes, node)
	}
	
	// Sort by layer then position
	sort.Slice(sortedNodes, func(i, j int) bool {
		if sortedNodes[i].Layer != sortedNodes[j].Layer {
			return sortedNodes[i].Layer < sortedNodes[j].Layer
		}
		return sortedNodes[i].Position < sortedNodes[j].Position
	})
	
	for _, node := range sortedNodes {
		for _, childID := range node.Children {
			if child, exists := g.Nodes[childID]; exists {
				// Style edge based on relationship type
				edgeStyle := "-->"
				if child.Parallel && len(node.Children) > 1 {
					edgeStyle = "-.->|parallel|"
				} else if child.Layer == node.Layer + 1 {
					edgeStyle = "-->|sequential|"
				}
				
				fmt.Fprintf(b, "  %s %s %s\n", node.ID, edgeStyle, childID)
			}
		}
	}
}

// truncateArgs shortens long argument strings for display
func truncateArgs(args string) string {
	if len(args) > 30 {
		return args[:27] + "..."
	}
	return args
}

// ToJSON emits enhanced JSON structure with matrix positioning and subgraphs.
func (g *DAG) ToJSON() string {
	var b strings.Builder
	b.WriteString("{\n")
	b.WriteString("  \"version\": \"2.0\",\n")
	b.WriteString("  \"matrix\": {\n")
	b.WriteString(fmt.Sprintf("    \"max_x\": %d,\n", g.MaxX))
	b.WriteString(fmt.Sprintf("    \"max_y\": %d\n", g.MaxY))
	b.WriteString("  },\n")
	
	// Export subgraphs
	if len(g.Subgraphs) > 0 {
		b.WriteString("  \"subgraphs\": [\n")
		first := true
		for _, sg := range g.Subgraphs {
			if !first {
				b.WriteString(",\n")
			}
			first = false
			fmt.Fprintf(&b, "    {\"id\":\"%s\",\"name\":\"%s\",\"parallel\":%t,\"nodes\":%s}",
				sg.ID, escapeJSON(sg.Name), sg.Parallel, stringArrayJSON(sg.Nodes))
		}
		b.WriteString("\n  ],\n")
	}
	
	// Export workflow nodes
	b.WriteString("  \"workflow\": [\n")
	first := true
	
	// Sort nodes by layer then position for consistent output
	var sortedNodes []*Node
	for _, n := range g.Nodes {
		if n.ID != g.Root {
			sortedNodes = append(sortedNodes, n)
		}
	}
	
	sort.Slice(sortedNodes, func(i, j int) bool {
		if sortedNodes[i].Layer != sortedNodes[j].Layer {
			return sortedNodes[i].Layer < sortedNodes[j].Layer
		}
		return sortedNodes[i].Position < sortedNodes[j].Position
	})
	
	for _, n := range sortedNodes {
		if !first {
			b.WriteString(",\n")
		}
		first = false
		
		subgraphStr := ""
		if n.Subgraph != "" {
			subgraphStr = fmt.Sprintf(",\"subgraph\":\"%s\",\"sub_x\":%d,\"sub_y\":%d", 
				n.Subgraph, n.SubX, n.SubY)
		}
		
		fmt.Fprintf(&b,
			"    {\"id\":\"%s\",\"tool\":\"%s\",\"args\":\"%s\",\"children\":%s,\"layer\":%d,\"position\":%d,\"parallel\":%t%s}",
			n.ID, n.Tool, escapeJSON(n.Args), childrenJSON(n.Children), 
			n.Layer, n.Position, n.Parallel, subgraphStr)
	}
	b.WriteString("\n  ]\n}")
	return b.String()
}
func escapeJSON(s string) string { 
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}

func childrenJSON(c []string) string {
	if len(c) == 0 {
		return "[]"
	}
	return `["` + strings.Join(c, `","`) + `"]`
}

func stringArrayJSON(arr []string) string {
	if len(arr) == 0 {
		return "[]"
	}
	return `["` + strings.Join(arr, `","`) + `"]`
}

// ToCompactMermaid generates a simplified left-to-right Mermaid diagram
func (g *DAG) ToCompactMermaid() string {
	var b strings.Builder
	b.WriteString("graph LR\n")
	
	// Simple node definitions
	for _, node := range g.Nodes {
		if node.ID == g.Root {
			fmt.Fprintf(&b, "  %s([Start])\n", node.ID)
		} else {
			fmt.Fprintf(&b, "  %s[%s]\n", node.ID, node.Tool)
		}
	}
	
	// Simple edges
	for _, node := range g.Nodes {
		for _, childID := range node.Children {
			fmt.Fprintf(&b, "  %s --> %s\n", node.ID, childID)
		}
	}
	
	return b.String()
}

// ToExecutionPlan generates a human-readable execution plan
func (g *DAG) ToExecutionPlan() string {
	var b strings.Builder
	b.WriteString("Execution Plan:\n")
	b.WriteString("==============\n\n")
	
	executionOrder := g.GetExecutionOrder()
	
	for stepNum, group := range executionOrder {
		fmt.Fprintf(&b, "Step %d:\n", stepNum+1)
		
		if len(group) == 1 {
			if node, exists := g.Nodes[group[0]]; exists {
				fmt.Fprintf(&b, "  → %s (%s)\n", node.Tool, node.ID)
				if node.Args != "" {
					fmt.Fprintf(&b, "    Args: %s\n", node.Args)
				}
			}
		} else {
			b.WriteString("  Parallel execution:\n")
			for _, nodeID := range group {
				if node, exists := g.Nodes[nodeID]; exists {
					fmt.Fprintf(&b, "  → %s (%s)\n", node.Tool, node.ID)
					if node.Args != "" {
						fmt.Fprintf(&b, "    Args: %s\n", node.Args)
					}
				}
			}
		}
		b.WriteString("\n")
	}
	
	return b.String()
}
