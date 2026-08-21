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

	if got := len(g.Nodes); got != 3 { // input + 2
		t.Fatalf("node count = %d, want 3", got)
	}
	if g.MaxLayer() != 2 {
		t.Fatalf("MaxLayer = %d, want 2", g.MaxLayer())
	}
	if kids := g.Nodes["input"].Children; len(kids) != 1 || kids[0] != "subfinder-1" {
		t.Fatalf("input children = %v, want [subfinder-1]", kids)
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

	// Add a sequential node in the same layer: it must form its own group.
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
	// Flatten and check subfinder precedes httpx precedes nuclei.
	pos := map[string]int{}
	for i, group := range order {
		for _, id := range group {
			pos[id] = i
		}
	}
	if !(pos["subfinder-1"] < pos["httpx-1"] && pos["httpx-1"] < pos["nuclei-1"]) {
		t.Fatalf("bad execution ordering: %v", pos)
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

func TestToMermaidAndJSON(t *testing.T) {
	g := NewDAG()
	_ = g.AddNode("input", "subfinder-1", "subfinder", "-d {{domain}}", 1)

	mmd := g.ToMermaid()
	if !strings.HasPrefix(mmd, "graph LR") {
		t.Fatalf("mermaid missing header: %q", mmd)
	}

	js := g.ToJSON()
	if !strings.Contains(js, `"subfinder-1"`) || !strings.Contains(js, `"version": "2.0"`) {
		t.Fatalf("json missing expected content: %s", js)
	}
}
