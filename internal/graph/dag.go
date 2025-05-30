package graph

import "fmt"

// Node represents a workflow vertex with 2D matrix positioning.
type Node struct {
	ID       string   `json:"id"`       // unique within the DAG  (e.g. nuclei-2)
	Tool     string   `json:"tool"`     // executable name       (e.g. nuclei)
	Args     string   `json:"args"`     // raw args (may contain placeholders)
	Children []string `json:"children"` // downstream node IDs
	Layer    int      `json:"layer"`    // horizontal layer index (X-axis)
	Position int      `json:"position"` // vertical position in layer (Y-axis)
	Subgraph string   `json:"subgraph"` // subgraph ID for grouping (empty = main graph)
	SubX     int      `json:"sub_x"`    // X position within subgraph
	SubY     int      `json:"sub_y"`    // Y position within subgraph
	Parallel bool     `json:"parallel"` // can run in parallel with other nodes
}

// Coordinate represents a 2D position in the workflow matrix
type Coordinate struct {
	X int // Layer (horizontal)
	Y int // Position (vertical)
}

// SubgraphInfo contains metadata about a subgraph
type SubgraphInfo struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Nodes       []string          `json:"nodes"`
	Parallel    bool              `json:"parallel"`
	Matrix      map[string]Coordinate `json:"matrix"` // node_id -> local coordinate
}

// DAG is a directed acyclic graph of nodes with matrix positioning.
type DAG struct {
	Nodes     map[string]*Node            `json:"nodes"`
	Root      string                      `json:"root"`
	Matrix    map[Coordinate][]*Node      `json:"matrix"`    // coordinate -> nodes at position
	Subgraphs map[string]*SubgraphInfo    `json:"subgraphs"` // subgraph_id -> info
	MaxX      int                         `json:"max_x"`     // maximum layer
	MaxY      int                         `json:"max_y"`     // maximum position in any layer
}

// NewDAG with an implicit "input" root.
func NewDAG() *DAG {
	g := &DAG{
		Nodes:     make(map[string]*Node),
		Matrix:    make(map[Coordinate][]*Node),
		Subgraphs: make(map[string]*SubgraphInfo),
		MaxX:      0,
		MaxY:      0,
	}
	g.Root = "input"
	rootNode := &Node{
		ID:       g.Root,
		Tool:     "input",
		Layer:    0,
		Position: 0,
		Parallel: false,
	}
	g.Nodes[g.Root] = rootNode
	g.addToMatrix(rootNode)
	return g
}

// AddNode attaches a new nodeID under parentID with matrix positioning.
func (g *DAG) AddNode(parentID, nodeID, tool, args string, layer int) error {
	return g.AddNodeAtPosition(parentID, nodeID, tool, args, layer, -1, "", false)
}

// AddNodeAtPosition adds a node with specific positioning and subgraph.
func (g *DAG) AddNodeAtPosition(parentID, nodeID, tool, args string, layer, position int, subgraph string, parallel bool) error {
	if _, ok := g.Nodes[parentID]; !ok {
		return fmt.Errorf("parent %q not found", parentID)
	}
	if _, dup := g.Nodes[nodeID]; dup {
		return fmt.Errorf("node %q already exists", nodeID)
	}
	
	// Auto-assign position if not specified
	if position == -1 {
		position = g.getNextPosition(layer, subgraph)
	}
	
	node := &Node{
		ID:       nodeID,
		Tool:     tool,
		Args:     args,
		Children: []string{},
		Layer:    layer,
		Position: position,
		Subgraph: subgraph,
		Parallel: parallel,
	}
	
	// Set subgraph coordinates if in subgraph
	if subgraph != "" {
		if sg, exists := g.Subgraphs[subgraph]; exists {
			node.SubX = len(sg.Nodes)
			node.SubY = 0
			sg.Nodes = append(sg.Nodes, nodeID)
		} else {
			// Create new subgraph
			g.Subgraphs[subgraph] = &SubgraphInfo{
				ID:       subgraph,
				Name:     subgraph,
				Nodes:    []string{nodeID},
				Parallel: parallel,
				Matrix:   make(map[string]Coordinate),
			}
			node.SubX = 0
			node.SubY = 0
		}
		g.Subgraphs[subgraph].Matrix[nodeID] = Coordinate{X: node.SubX, Y: node.SubY}
	}
	
	g.Nodes[nodeID] = node
	g.Nodes[parentID].Children = append(g.Nodes[parentID].Children, nodeID)
	g.addToMatrix(node)
	g.updateBounds(layer, position)
	
	return nil
}

// Helper methods for matrix management

// addToMatrix adds a node to the coordinate matrix.
func (g *DAG) addToMatrix(node *Node) {
	coord := Coordinate{X: node.Layer, Y: node.Position}
	g.Matrix[coord] = append(g.Matrix[coord], node)
}

// removeFromMatrix removes a node from the coordinate matrix.
func (g *DAG) removeFromMatrix(node *Node) {
	coord := Coordinate{X: node.Layer, Y: node.Position}
	if nodes, exists := g.Matrix[coord]; exists {
		for i, n := range nodes {
			if n.ID == node.ID {
				g.Matrix[coord] = append(nodes[:i], nodes[i+1:]...)
				break
			}
		}
		// Remove coordinate if no nodes left
		if len(g.Matrix[coord]) == 0 {
			delete(g.Matrix, coord)
		}
	}
}

// getNextPosition finds the next available position in a layer.
func (g *DAG) getNextPosition(layer int, subgraph string) int {
	maxPos := -1
	for _, node := range g.Nodes {
		if node.Layer == layer && node.Subgraph == subgraph {
			if node.Position > maxPos {
				maxPos = node.Position
			}
		}
	}
	return maxPos + 1
}

// updateBounds updates the maximum X and Y coordinates.
func (g *DAG) updateBounds(layer, position int) {
	if layer > g.MaxX {
		g.MaxX = layer
	}
	if position > g.MaxY {
		g.MaxY = position
	}
}

// recalculateBounds recalculates the maximum X and Y coordinates.
func (g *DAG) recalculateBounds() {
	g.MaxX = 0
	g.MaxY = 0
	for _, node := range g.Nodes {
		if node.Layer > g.MaxX {
			g.MaxX = node.Layer
		}
		if node.Position > g.MaxY {
			g.MaxY = node.Position
		}
	}
}

// MoveNode changes the position of a node.
func (g *DAG) MoveNode(nodeID string, newLayer, newPosition int) error {
	node, exists := g.Nodes[nodeID]
	if !exists {
		return fmt.Errorf("node %q not found", nodeID)
	}
	
	// Remove from current position
	g.removeFromMatrix(node)
	
	// Update coordinates
	node.Layer = newLayer
	node.Position = newPosition
	
	// Add to new position
	g.addToMatrix(node)
	g.updateBounds(newLayer, newPosition)
	
	return nil
}

// CompactLayer removes gaps in positions within a layer.
func (g *DAG) CompactLayer(layer int) {
	nodes := []*Node{}
	for _, node := range g.Nodes {
		if node.Layer == layer {
			nodes = append(nodes, node)
		}
	}
	
	// Sort by current position
	for i := 0; i < len(nodes)-1; i++ {
		for j := i + 1; j < len(nodes); j++ {
			if nodes[i].Position > nodes[j].Position {
				nodes[i], nodes[j] = nodes[j], nodes[i]
			}
		}
	}
	
	// Reassign positions sequentially
	for i, node := range nodes {
		g.removeFromMatrix(node)
		node.Position = i
		g.addToMatrix(node)
	}
	
	g.recalculateBounds()
}

// GetExecutionOrder returns the optimal execution order considering matrix positioning.
func (g *DAG) GetExecutionOrder() [][]string {
	var order [][]string
	
	for layer := 0; layer <= g.MaxX; layer++ {
		layerGroups := g.GetParallelNodes(layer)
		for _, group := range layerGroups {
			nodeIDs := make([]string, len(group))
			for i, node := range group {
				nodeIDs[i] = node.ID
			}
			if len(nodeIDs) > 0 {
				order = append(order, nodeIDs)
			}
		}
	}
	
	return order
}

// ValidateMatrix ensures the matrix is consistent and valid.
func (g *DAG) ValidateMatrix() error {
	// Check for coordinate conflicts
	for coord, nodes := range g.Matrix {
		if len(nodes) > 1 {
			// Multiple nodes at same coordinate - check if they're all parallel
			for _, node := range nodes {
				if !node.Parallel {
					return fmt.Errorf("non-parallel node %s conflicts with other nodes at coordinate (%d,%d)", 
						node.ID, coord.X, coord.Y)
				}
			}
		}
	}
	
	// Check if all nodes are in matrix
	for _, node := range g.Nodes {
		coord := Coordinate{X: node.Layer, Y: node.Position}
		found := false
		if matrixNodes, exists := g.Matrix[coord]; exists {
			for _, matrixNode := range matrixNodes {
				if matrixNode.ID == node.ID {
					found = true
					break
				}
			}
		}
		if !found {
			return fmt.Errorf("node %s not found in matrix at coordinate (%d,%d)", 
				node.ID, coord.X, coord.Y)
		}
	}
	
	return nil
}

// RemoveNode deletes node and edges, updating matrix.
func (g *DAG) RemoveNode(id string) error {
	if id == g.Root {
		return fmt.Errorf("cannot remove root")
	}
	
	// Get node before deletion
	node, exists := g.Nodes[id]
	if !exists {
		return fmt.Errorf("node %q not found", id)
	}
	
	// Remove from matrix
	g.removeFromMatrix(node)
	
	// Remove from subgraph if applicable
	if node.Subgraph != "" {
		if sg, exists := g.Subgraphs[node.Subgraph]; exists {
			// Remove from subgraph nodes list
			for i, nodeID := range sg.Nodes {
				if nodeID == id {
					sg.Nodes = append(sg.Nodes[:i], sg.Nodes[i+1:]...)
					break
				}
			}
			delete(sg.Matrix, id)
			
			// Remove subgraph if empty
			if len(sg.Nodes) == 0 {
				delete(g.Subgraphs, node.Subgraph)
			}
		}
	}
	
	// Remove node
	delete(g.Nodes, id)
	
	// Remove from all children lists
	for _, n := range g.Nodes {
		dst := n.Children[:0]
		for _, c := range n.Children {
			if c != id {
				dst = append(dst, c)
			}
		}
		n.Children = dst
	}
	
	// Recalculate bounds
	g.recalculateBounds()
	
	return nil
}

// GetLayer returns node IDs at layer l, sorted by position.
func (g *DAG) GetLayer(l int) []string {
	var nodes []*Node
	for _, n := range g.Nodes {
		if n.Layer == l {
			nodes = append(nodes, n)
		}
	}
	
	// Sort by position (Y coordinate)
	for i := 0; i < len(nodes)-1; i++ {
		for j := i + 1; j < len(nodes); j++ {
			if nodes[i].Position > nodes[j].Position {
				nodes[i], nodes[j] = nodes[j], nodes[i]
			}
		}
	}
	
	ids := make([]string, len(nodes))
	for i, n := range nodes {
		ids[i] = n.ID
	}
	return ids
}

// GetLayerMatrix returns nodes at layer l organized by position.
func (g *DAG) GetLayerMatrix(l int) map[int][]*Node {
	matrix := make(map[int][]*Node)
	for _, n := range g.Nodes {
		if n.Layer == l {
			matrix[n.Position] = append(matrix[n.Position], n)
		}
	}
	return matrix
}

// GetCoordinate returns the coordinate of a node.
func (g *DAG) GetCoordinate(nodeID string) (Coordinate, bool) {
	if node, exists := g.Nodes[nodeID]; exists {
		return Coordinate{X: node.Layer, Y: node.Position}, true
	}
	return Coordinate{}, false
}

// GetNodesAtCoordinate returns all nodes at a specific coordinate.
func (g *DAG) GetNodesAtCoordinate(coord Coordinate) []*Node {
	if nodes, exists := g.Matrix[coord]; exists {
		return nodes
	}
	return []*Node{}
}

// GetNextPosition finds the next available position in a layer and subgraph.
func (g *DAG) GetNextPosition(layer int, subgraph string) int {
	maxPos := -1
	for _, node := range g.Nodes {
		if node.Layer == layer && node.Subgraph == subgraph {
			if node.Position > maxPos {
				maxPos = node.Position
			}
		}
	}
	return maxPos + 1
}

// UpdateBounds updates the maximum X and Y coordinates.
func (g *DAG) UpdateBounds(layer, position int) {
	if layer > g.MaxX {
		g.MaxX = layer
	}
	if position > g.MaxY {
		g.MaxY = position
	}
}

// GetParallelNodes returns nodes that can run in parallel at the same layer.
func (g *DAG) GetParallelNodes(layer int) [][]*Node {
	layerMatrix := g.GetLayerMatrix(layer)
	var groups [][]*Node
	
	for pos := 0; pos <= g.MaxY; pos++ {
		if nodes, exists := layerMatrix[pos]; exists {
			parallelGroup := []*Node{}
			for _, node := range nodes {
				if node.Parallel {
					parallelGroup = append(parallelGroup, node)
				} else {
					// Non-parallel nodes get their own group
					groups = append(groups, []*Node{node})
				}
			}
			if len(parallelGroup) > 0 {
				groups = append(groups, parallelGroup)
			}
		}
	}
	
	return groups
}

// GetSubgraphNodes returns all nodes in a subgraph, sorted by their subgraph coordinates.
func (g *DAG) GetSubgraphNodes(subgraphID string) []*Node {
	if sg, exists := g.Subgraphs[subgraphID]; exists {
		var nodes []*Node
		for _, nodeID := range sg.Nodes {
			if node, exists := g.Nodes[nodeID]; exists {
				nodes = append(nodes, node)
			}
		}
		
		// Sort by subgraph coordinates
		for i := 0; i < len(nodes)-1; i++ {
			for j := i + 1; j < len(nodes); j++ {
				if nodes[i].SubX > nodes[j].SubX || 
				   (nodes[i].SubX == nodes[j].SubX && nodes[i].SubY > nodes[j].SubY) {
					nodes[i], nodes[j] = nodes[j], nodes[i]
				}
			}
		}
		
		return nodes
	}
	return []*Node{}
}
