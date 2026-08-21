package graph

import (
	"fmt"
	"sort"
)

// Node represents a workflow vertex with 2D matrix positioning and execution
// semantics. Older v2 workflows only populate the legacy fields; the helpers
// in this package transparently treat an empty Kind as a worker node.
type Node struct {
	ID       string   `json:"id"`
	Tool     string   `json:"tool,omitempty"`
	Args     string   `json:"args,omitempty"`
	Children []string `json:"children,omitempty"`
	Layer    int      `json:"layer"`
	Position int      `json:"position"`
	Subgraph string   `json:"subgraph,omitempty"`
	SubX     int      `json:"sub_x,omitempty"`
	SubY     int      `json:"sub_y,omitempty"`
	Parallel bool     `json:"parallel,omitempty"`

	// v3 execution semantics.
	Kind      NodeKind       `json:"kind,omitempty"`
	Inputs    []ArtifactType `json:"inputs,omitempty"`
	Outputs   []ArtifactType `json:"outputs,omitempty"`
	Transform string         `json:"transform,omitempty"`
	Condition string         `json:"condition,omitempty"`
	Policy    NodePolicy     `json:"policy,omitempty"`
	Execution NodeExecution  `json:"execution,omitempty"`
	Tags      []string       `json:"tags,omitempty"`
}

// EffectiveKind preserves backwards compatibility with v2 workflow files.
func (n *Node) EffectiveKind() NodeKind {
	if n == nil || n.Kind == "" {
		return NodeKindWorker
	}
	return n.Kind
}

// Coordinate represents a 2D position in the workflow matrix.
type Coordinate struct {
	X int
	Y int
}

// SubgraphInfo contains metadata about a subgraph.
type SubgraphInfo struct {
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	Description string                `json:"description,omitempty"`
	Nodes       []string              `json:"nodes"`
	Parallel    bool                  `json:"parallel"`
	Matrix      map[string]Coordinate `json:"matrix,omitempty"`
}

// DAG is a directed acyclic graph of workflow nodes. Children is retained on
// Node for v2 compatibility; Edges is authoritative for v3 semantics and may
// carry conditions and labels.
type DAG struct {
	Version   string                   `json:"version,omitempty"`
	Nodes     map[string]*Node         `json:"nodes"`
	Root      string                   `json:"root"`
	Matrix    map[Coordinate][]*Node   `json:"matrix"`
	Subgraphs map[string]*SubgraphInfo `json:"subgraphs"`
	Edges     []Edge                   `json:"edges,omitempty"`
	Policy    WorkflowPolicy           `json:"policy,omitempty"`
	MaxX      int                      `json:"max_x"`
	MaxY      int                      `json:"max_y"`
}

// NewDAG creates a v3 DAG with an implicit typed input root.
func NewDAG() *DAG {
	g := &DAG{
		Version:   "3.0",
		Nodes:     make(map[string]*Node),
		Matrix:    make(map[Coordinate][]*Node),
		Subgraphs: make(map[string]*SubgraphInfo),
		Edges:     []Edge{},
	}
	g.Root = "input"
	rootNode := &Node{
		ID:       g.Root,
		Tool:     "input",
		Kind:     NodeKindSource,
		Outputs:  []ArtifactType{ArtifactDomain},
		Layer:    0,
		Position: 0,
	}
	g.Nodes[g.Root] = rootNode
	g.addToMatrix(rootNode)
	return g
}

// AddNode attaches a new worker node under parentID.
func (g *DAG) AddNode(parentID, nodeID, tool, args string, layer int) error {
	return g.AddNodeAtPosition(parentID, nodeID, tool, args, layer, -1, "", false)
}

// AddNodeAtPosition adds a worker node with matrix positioning.
func (g *DAG) AddNodeAtPosition(parentID, nodeID, tool, args string, layer, position int, subgraph string, parallel bool) error {
	if _, ok := g.Nodes[parentID]; !ok {
		return fmt.Errorf("parent %q not found", parentID)
	}
	if _, dup := g.Nodes[nodeID]; dup {
		return fmt.Errorf("node %q already exists", nodeID)
	}
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
		Kind:     NodeKindWorker,
	}

	if subgraph != "" {
		if sg, exists := g.Subgraphs[subgraph]; exists {
			node.SubX = len(sg.Nodes)
			sg.Nodes = append(sg.Nodes, nodeID)
		} else {
			g.Subgraphs[subgraph] = &SubgraphInfo{
				ID:       subgraph,
				Name:     subgraph,
				Nodes:    []string{nodeID},
				Parallel: parallel,
				Matrix:   make(map[string]Coordinate),
			}
		}
		g.Subgraphs[subgraph].Matrix[nodeID] = Coordinate{X: node.SubX, Y: node.SubY}
	}

	g.Nodes[nodeID] = node
	g.addToMatrix(node)
	g.updateBounds(layer, position)
	return g.AddEdge(parentID, nodeID, "", "")
}

// AddEdge creates an explicit v3 edge while maintaining the legacy Children
// list so v2 callers and the visual builder continue to work.
func (g *DAG) AddEdge(from, to, condition, label string) error {
	if _, ok := g.Nodes[from]; !ok {
		return fmt.Errorf("edge source %q not found", from)
	}
	if _, ok := g.Nodes[to]; !ok {
		return fmt.Errorf("edge destination %q not found", to)
	}
	for _, e := range g.Edges {
		if e.From == from && e.To == to && e.Condition == condition {
			return nil
		}
	}
	g.Edges = append(g.Edges, Edge{From: from, To: to, Condition: condition, Label: label})
	if !containsString(g.Nodes[from].Children, to) {
		g.Nodes[from].Children = append(g.Nodes[from].Children, to)
	}
	return nil
}

// EnsureEdges upgrades legacy Children relationships into explicit v3 edges.
func (g *DAG) EnsureEdges() {
	for from, node := range g.Nodes {
		for _, to := range node.Children {
			found := false
			for _, e := range g.Edges {
				if e.From == from && e.To == to {
					found = true
					break
				}
			}
			if !found {
				g.Edges = append(g.Edges, Edge{From: from, To: to})
			}
		}
	}
}

// Parents returns direct upstream node IDs in deterministic order.
func (g *DAG) Parents(nodeID string) []string {
	g.EnsureEdges()
	seen := map[string]struct{}{}
	var out []string
	for _, e := range g.Edges {
		if e.To == nodeID {
			if _, ok := seen[e.From]; !ok {
				seen[e.From] = struct{}{}
				out = append(out, e.From)
			}
		}
	}
	sort.Strings(out)
	return out
}

// IncomingEdges returns direct incoming edges for nodeID.
func (g *DAG) IncomingEdges(nodeID string) []Edge {
	g.EnsureEdges()
	var out []Edge
	for _, e := range g.Edges {
		if e.To == nodeID {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].From < out[j].From })
	return out
}

// OutgoingEdges returns direct outgoing edges for nodeID.
func (g *DAG) OutgoingEdges(nodeID string) []Edge {
	g.EnsureEdges()
	var out []Edge
	for _, e := range g.Edges {
		if e.From == nodeID {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].To < out[j].To })
	return out
}

func (g *DAG) addToMatrix(node *Node) {
	coord := Coordinate{X: node.Layer, Y: node.Position}
	g.Matrix[coord] = append(g.Matrix[coord], node)
}

func (g *DAG) removeFromMatrix(node *Node) {
	coord := Coordinate{X: node.Layer, Y: node.Position}
	if nodes, exists := g.Matrix[coord]; exists {
		for i, n := range nodes {
			if n.ID == node.ID {
				g.Matrix[coord] = append(nodes[:i], nodes[i+1:]...)
				break
			}
		}
		if len(g.Matrix[coord]) == 0 {
			delete(g.Matrix, coord)
		}
	}
}

func (g *DAG) getNextPosition(layer int, subgraph string) int {
	maxPos := -1
	for _, node := range g.Nodes {
		if node.Layer == layer && node.Subgraph == subgraph && node.Position > maxPos {
			maxPos = node.Position
		}
	}
	return maxPos + 1
}

func (g *DAG) updateBounds(layer, position int) {
	if layer > g.MaxX {
		g.MaxX = layer
	}
	if position > g.MaxY {
		g.MaxY = position
	}
}

func (g *DAG) recalculateBounds() {
	g.MaxX, g.MaxY = 0, 0
	for _, node := range g.Nodes {
		g.updateBounds(node.Layer, node.Position)
	}
}

// MoveNode changes a node's matrix position.
func (g *DAG) MoveNode(nodeID string, newLayer, newPosition int) error {
	node, exists := g.Nodes[nodeID]
	if !exists {
		return fmt.Errorf("node %q not found", nodeID)
	}
	g.removeFromMatrix(node)
	node.Layer, node.Position = newLayer, newPosition
	g.addToMatrix(node)
	g.recalculateBounds()
	return nil
}

// CompactLayer removes gaps in positions within a layer.
func (g *DAG) CompactLayer(layer int) {
	var nodes []*Node
	for _, node := range g.Nodes {
		if node.Layer == layer {
			nodes = append(nodes, node)
		}
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Position < nodes[j].Position })
	for i, node := range nodes {
		g.removeFromMatrix(node)
		node.Position = i
		g.addToMatrix(node)
	}
	g.recalculateBounds()
}

// GetExecutionOrder is the legacy matrix/layer execution plan. The v3 runtime
// uses dependency readiness instead; this remains for previews and v2 callers.
func (g *DAG) GetExecutionOrder() [][]string {
	var order [][]string
	for layer := 0; layer <= g.MaxX; layer++ {
		for _, group := range g.GetParallelNodes(layer) {
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

// ValidateMatrix ensures the visual matrix is consistent.
func (g *DAG) ValidateMatrix() error {
	for coord, nodes := range g.Matrix {
		if len(nodes) > 1 {
			for _, node := range nodes {
				if !node.Parallel {
					return fmt.Errorf("non-parallel node %s conflicts at coordinate (%d,%d)", node.ID, coord.X, coord.Y)
				}
			}
		}
	}
	for _, node := range g.Nodes {
		coord := Coordinate{X: node.Layer, Y: node.Position}
		found := false
		for _, matrixNode := range g.Matrix[coord] {
			if matrixNode.ID == node.ID {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("node %s not found in matrix at coordinate (%d,%d)", node.ID, coord.X, coord.Y)
		}
	}
	return nil
}

// RemoveNode deletes a node and every inbound/outbound edge.
func (g *DAG) RemoveNode(id string) error {
	if id == g.Root {
		return fmt.Errorf("cannot remove root")
	}
	node, exists := g.Nodes[id]
	if !exists {
		return fmt.Errorf("node %q not found", id)
	}
	g.removeFromMatrix(node)
	if node.Subgraph != "" {
		if sg, exists := g.Subgraphs[node.Subgraph]; exists {
			for i, nodeID := range sg.Nodes {
				if nodeID == id {
					sg.Nodes = append(sg.Nodes[:i], sg.Nodes[i+1:]...)
					break
				}
			}
			delete(sg.Matrix, id)
			if len(sg.Nodes) == 0 {
				delete(g.Subgraphs, node.Subgraph)
			}
		}
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
	filtered := g.Edges[:0]
	for _, e := range g.Edges {
		if e.From != id && e.To != id {
			filtered = append(filtered, e)
		}
	}
	g.Edges = filtered
	g.recalculateBounds()
	return nil
}

// GetLayer returns node IDs at layer l, sorted by visual position.
func (g *DAG) GetLayer(l int) []string {
	var nodes []*Node
	for _, n := range g.Nodes {
		if n.Layer == l {
			nodes = append(nodes, n)
		}
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Position < nodes[j].Position })
	ids := make([]string, len(nodes))
	for i, n := range nodes {
		ids[i] = n.ID
	}
	return ids
}

func (g *DAG) GetLayerMatrix(l int) map[int][]*Node {
	matrix := make(map[int][]*Node)
	for _, n := range g.Nodes {
		if n.Layer == l {
			matrix[n.Position] = append(matrix[n.Position], n)
		}
	}
	return matrix
}

func (g *DAG) GetCoordinate(nodeID string) (Coordinate, bool) {
	if node, exists := g.Nodes[nodeID]; exists {
		return Coordinate{X: node.Layer, Y: node.Position}, true
	}
	return Coordinate{}, false
}

func (g *DAG) GetNodesAtCoordinate(coord Coordinate) []*Node {
	if nodes, exists := g.Matrix[coord]; exists {
		return nodes
	}
	return []*Node{}
}

func (g *DAG) GetNextPosition(layer int, subgraph string) int {
	return g.getNextPosition(layer, subgraph)
}

func (g *DAG) UpdateBounds(layer, position int) { g.updateBounds(layer, position) }
func (g *DAG) MaxLayer() int                    { return g.MaxX }

func (g *DAG) RemoveFromLayer(id string) {
	if node, ok := g.Nodes[id]; ok {
		g.removeFromMatrix(node)
	}
}

func (g *DAG) InsertAtLayer(id string, layer, position int) {
	node, ok := g.Nodes[id]
	if !ok {
		return
	}
	node.Layer, node.Position = layer, position
	g.addToMatrix(node)
	g.recalculateBounds()
}

// GetParallelNodes is retained for the visual/matrix execution planner.
func (g *DAG) GetParallelNodes(layer int) [][]*Node {
	layerMatrix := g.GetLayerMatrix(layer)
	var groups [][]*Node
	var parallelGroup []*Node
	for pos := 0; pos <= g.MaxY; pos++ {
		nodes, exists := layerMatrix[pos]
		if !exists {
			continue
		}
		for _, node := range nodes {
			if node.Parallel {
				parallelGroup = append(parallelGroup, node)
			} else {
				groups = append(groups, []*Node{node})
			}
		}
	}
	if len(parallelGroup) > 0 {
		groups = append(groups, parallelGroup)
	}
	return groups
}

func (g *DAG) GetSubgraphNodes(subgraphID string) []*Node {
	sg, exists := g.Subgraphs[subgraphID]
	if !exists {
		return []*Node{}
	}
	var nodes []*Node
	for _, nodeID := range sg.Nodes {
		if node, ok := g.Nodes[nodeID]; ok {
			nodes = append(nodes, node)
		}
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].SubX != nodes[j].SubX {
			return nodes[i].SubX < nodes[j].SubX
		}
		return nodes[i].SubY < nodes[j].SubY
	})
	return nodes
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
