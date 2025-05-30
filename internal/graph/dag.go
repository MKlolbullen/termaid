package graph

import "fmt"

// Node represents a workflow vertex.
type Node struct {
	ID       string   `json:"id"`       // unique within the DAG  (e.g. nuclei-2)
	Tool     string   `json:"tool"`     // executable name       (e.g. nuclei)
	Args     string   `json:"args"`     // raw args (may contain placeholders)
	Children []string `json:"children"` // downstream node IDs
	Layer    int      `json:"layer"`    // vertical layer index
}

// DAG is a directed acyclic graph of nodes.
type DAG struct {
	Nodes map[string]*Node
	Root  string
}

// NewDAG with an implicit “input” root.
func NewDAG() *DAG {
	g := &DAG{Nodes: make(map[string]*Node)}
	g.Root = "input"
	g.Nodes[g.Root] = &Node{
		ID:    g.Root,
		Tool:  "input",
		Layer: 0,
	}
	return g
}

// AddNode attaches a new nodeID under parentID.
func (g *DAG) AddNode(parentID, nodeID, tool, args string, layer int) error {
	if _, ok := g.Nodes[parentID]; !ok {
		return fmt.Errorf("parent %q not found", parentID)
	}
	if _, dup := g.Nodes[nodeID]; dup {
		return fmt.Errorf("node %q already exists", nodeID)
	}
	g.Nodes[nodeID] = &Node{
		ID:       nodeID,
		Tool:     tool,
		Args:     args,
		Children: []string{},
		Layer:    layer,
	}
	g.Nodes[parentID].Children = append(g.Nodes[parentID].Children, nodeID)
	return nil
}

// RemoveNode deletes node and edges.
func (g *DAG) RemoveNode(id string) error {
	if id == g.Root {
		return fmt.Errorf("cannot remove root")
	}
	delete(g.Nodes, id)
	for _, n := range g.Nodes {
		dst := n.Children[:0]
		for _, c := range n.Children {
			if c != id {
				dst = append(dst, c)
			}
		}
		n.Children = dst
	}
	return nil
}

// GetLayer returns node IDs at layer l.
func (g *DAG) GetLayer(l int) []string {
	ids := []string{}
	for _, n := range g.Nodes {
		if n.Layer == l {
			ids = append(ids, n.ID)
		}
	}
	return ids
}
