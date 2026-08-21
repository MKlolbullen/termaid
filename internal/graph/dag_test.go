package graph

import (
	"strings"
	"testing"
)

func TestAddNodeCreatesEdgeAndLayer(t *testing.T) {
	g := NewDAG()
	if err := g.AddNode("input", "subfinder-1", "subfinder", "-d {{domain}}", 1); err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	if err := g.AddNode("subfinder-1", "httpx-1", "httpx", "-l {{input}}", 2); err != nil {
		t.Fatalf("AddNode: %v", err)
	}

	if got := len(g.Nodes); got != 3 {
		t.Fatalf("node count = %d, want 3", got)
	}
	if g.MaxLayer() != 2 {
		t.Fatalf("MaxLayer = %d, want 2", g.MaxLayer())
	}
	if kids := g.Nodes["input"].Children; len(kids) != 1 || kids[0] != "subfinder-1" {
		t.Fatalf("input children = %v, want [subfinder-1]", kids)
	}
	if len(g.Edges) != 2 {
		t.Fatalf("semantic edge count = %d, want 2", len(g.Edges))
	}
}

func TestAddNodeRejectsDuplicatesAndMissingParent(t *testing.T) {
	g := NewDAG()
	_ = g.AddNode("input", "a-1", "a", "", 1)
	if err := g.AddNode("input", "a-1", "a", "", 1); err == nil {
		t.Fatal("expected error for duplicate node id")
	}
	if err := g.AddNode("ghost", "b-1", "b", "", 1); err == nil {
		t.Fatal("expected error for missing parent")
	}
}

func TestRemoveNodePrunesEdges(t *testing.T) {
	g := NewDAG()
	_ = g.AddNode("input", "a-1", "a", "", 1)
	_ = g.AddNode("a-1", "b-1", "b", "", 2)

	if err := g.RemoveNode("b-1"); err != nil {
		t.Fatalf("RemoveNode: %v", err)
	}
	if _, ok := g.Nodes["b-1"]; ok {
		t.Fatal("b-1 still present after removal")
	}
	for _, c := range g.Nodes["a-1"].Children {
		if c == "b-1" {
			t.Fatal("dangling child edge to b-1")
		}
	}
	for _, e := range g.Edges {
		if e.From == "b-1" || e.To == "b-1" {
			t.Fatalf("dangling semantic edge after removal: %+v", e)
		}
	}
	if err := g.RemoveNode("input"); err == nil {
		t.Fatal("expected error removing root")
	}
}

func TestGetParallelNodesGroupsWholeLayer(t *testing.T) {
	g := NewDAG()
	_ = g.AddNodeAtPosition("input", "a-1", "a", "", 1, 0, "", true)
	_ = g.AddNodeAtPosition("input", "b-1", "b", "", 1, 1, "", true)
	_ = g.AddNodeAtPosition("input", "c-1", "c", "", 1, 2, "", true)

	groups := g.GetParallelNodes(1)
	if len(groups) != 1 {
		t.Fatalf("want 1 parallel group, got %d: %v", len(groups), groups)
	}
	if len(groups[0]) != 3 {
		t.Fatalf("want 3 nodes in the parallel group, got %d", len(groups[0]))
	}

	_ = g.AddNodeAtPosition("input", "d-1", "d", "", 1, 3, "", false)
	groups = g.GetParallelNodes(1)
	if len(groups) != 2 {
		t.Fatalf("want 2 groups (1 sequential + 1 parallel), got %d: %v", len(groups), groups)
	}
}

func TestExecutionOrderFollowsLayers(t *testing.T) {
	g := NewDAG()
	_ = g.AddNode("input", "subfinder-1", "subfinder", "", 1)
	_ = g.AddNode("subfinder-1", "httpx-1", "httpx", "", 2)
	_ = g.AddNode("httpx-1", "nuclei-1", "nuclei", "", 3)

	order := g.GetExecutionOrder()
	pos := map[string]int{}
	for i, group := range order {
		for _, id := range group {
			pos[id] = i
		}
	}
	if !(pos["subfinder-1"] < pos["httpx-1"] && pos["httpx-1"] < pos["nuclei-1"]) {
		t.Fatalf("bad legacy execution ordering: %v", pos)
	}
}

func TestTopologicalOrderIgnoresVisualLayerOrdering(t *testing.T) {
	g := NewDAG()
	_ = g.AddNodeAtPosition("input", "discover", "discover", "", 9, 0, "", true)
	_ = g.AddNodeAtPosition("discover", "verify", "verify", "", 1, 0, "", false)

	order, err := g.TopologicalOrder()
	if err != nil {
		t.Fatalf("TopologicalOrder: %v", err)
	}
	index := map[string]int{}
	for i, id := range order {
		index[id] = i
	}
	if index["discover"] >= index["verify"] {
		t.Fatalf("dependency order ignored edge: %v", order)
	}
}

func TestInsertAtLayerRepositions(t *testing.T) {
	g := NewDAG()
	_ = g.AddNodeAtPosition("input", "a-1", "a", "", 1, 0, "", false)
	g.RemoveFromLayer("a-1")
	g.InsertAtLayer("a-1", 3, 2)

	if n := g.Nodes["a-1"]; n.Layer != 3 || n.Position != 2 {
		t.Fatalf("node position = (%d,%d), want (3,2)", n.Layer, n.Position)
	}
	if g.MaxLayer() != 3 {
		t.Fatalf("MaxLayer = %d, want 3", g.MaxLayer())
	}
}

func TestTypedArtifactValidationRejectsDataMismatch(t *testing.T) {
	g := NewDAG()
	producer := &Node{ID: "hosts", Tool: "subfinder", Kind: NodeKindWorker, Outputs: []ArtifactType{ArtifactHost}, Layer: 1, Position: 0}
	consumer := &Node{ID: "scanner", Tool: "nuclei", Kind: NodeKindWorker, Inputs: []ArtifactType{ArtifactURL}, Layer: 2, Position: 0}
	g.Nodes[producer.ID] = producer
	g.Nodes[consumer.ID] = consumer
	g.Matrix[Coordinate{X: 1, Y: 0}] = []*Node{producer}
	g.Matrix[Coordinate{X: 2, Y: 0}] = []*Node{consumer}
	g.UpdateBounds(2, 0)
	_ = g.AddEdge("input", "hosts", "", "")
	_ = g.AddEdge("hosts", "scanner", "", "")

	if err := g.Validate(); err == nil || !strings.Contains(err.Error(), "artifact contract mismatch") {
		t.Fatalf("expected artifact mismatch, got %v", err)
	}
}

func TestControlEdgeMayCrossArtifactTypes(t *testing.T) {
	g := NewDAG()
	urls := &Node{ID: "urls", Tool: "httpx", Kind: NodeKindWorker, Inputs: []ArtifactType{ArtifactDomain}, Outputs: []ArtifactType{ArtifactURL}, Layer: 1, Position: 0}
	tech := &Node{ID: "tech", Tool: "whatweb", Kind: NodeKindWorker, Inputs: []ArtifactType{ArtifactDomain}, Outputs: []ArtifactType{ArtifactTechnology}, Layer: 1, Position: 1, Parallel: true}
	wp := &Node{ID: "wp", Tool: "nuclei", Kind: NodeKindWorker, Inputs: []ArtifactType{ArtifactURL}, Outputs: []ArtifactType{ArtifactFinding}, Layer: 2, Position: 0}
	for _, n := range []*Node{urls, tech, wp} {
		g.Nodes[n.ID] = n
		g.Matrix[Coordinate{X: n.Layer, Y: n.Position}] = []*Node{n}
		g.UpdateBounds(n.Layer, n.Position)
	}
	_ = g.AddEdge("input", "urls", "", "")
	_ = g.AddEdge("input", "tech", "", "")
	_ = g.AddEdge("urls", "wp", "", "")
	_ = g.AddEdge("tech", "wp", "contains:wordpress", "WordPress detected")
	g.Edges[len(g.Edges)-1].Control = true

	if err := g.Validate(); err != nil {
		t.Fatalf("control edge should not require artifact compatibility: %v", err)
	}
}

func TestValidateRejectsCycle(t *testing.T) {
	g := NewDAG()
	_ = g.AddNode("input", "a", "a", "", 1)
	_ = g.AddNode("a", "b", "b", "", 2)
	if err := g.AddEdge("b", "a", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := g.Validate(); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected cycle error, got %v", err)
	}
}

func TestToMermaidAndJSONV3(t *testing.T) {
	g := NewDAG()
	_ = g.AddNode("input", "subfinder-1", "subfinder", "-d {{domain}}", 1)
	g.Nodes["subfinder-1"].Outputs = []ArtifactType{ArtifactHost}

	mmd := g.ToMermaid()
	if !strings.HasPrefix(mmd, "flowchart LR") {
		t.Fatalf("mermaid missing v3 header: %q", mmd)
	}
	if !strings.Contains(mmd, "→ host") {
		t.Fatalf("mermaid missing artifact annotation: %s", mmd)
	}

	js := g.ToJSON()
	if !strings.Contains(js, `"subfinder-1"`) || !strings.Contains(js, `"version": "3.0"`) || !strings.Contains(js, `"edges"`) {
		t.Fatalf("json missing expected v3 content: %s", js)
	}
}
