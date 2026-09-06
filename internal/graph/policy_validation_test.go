package graph

import (
	"strings"
	"testing"
)

func TestValidateRejectsTypedNodeWithOnlyControlInput(t *testing.T) {
	g := NewDAG()
	if err := g.AddNode("input", "tech", "fingerprint", "", 1); err != nil {
		t.Fatal(err)
	}
	if err := g.AddNode("tech", "adaptive", "scanner", "", 2); err != nil {
		t.Fatal(err)
	}
	g.Nodes["tech"].Outputs = []ArtifactType{ArtifactTechnology}
	g.Nodes["adaptive"].Inputs = []ArtifactType{ArtifactURL}
	g.Nodes["adaptive"].Outputs = []ArtifactType{ArtifactFinding}
	for i := range g.Edges {
		if g.Edges[i].From == "tech" && g.Edges[i].To == "adaptive" {
			g.Edges[i].Control = true
			g.Edges[i].Condition = "contains:wordpress"
		}
	}

	err := g.Validate()
	if err == nil || !strings.Contains(err.Error(), "only control edges") {
		t.Fatalf("expected control-only typed input error, got %v", err)
	}
}

func TestValidateAllowsDataAndControlInputsTogether(t *testing.T) {
	g := NewDAG()
	if err := g.AddNode("input", "surface", "surface", "", 1); err != nil {
		t.Fatal(err)
	}
	if err := g.AddNode("input", "tech", "fingerprint", "", 1); err != nil {
		t.Fatal(err)
	}
	if err := g.AddNode("surface", "adaptive", "scanner", "", 2); err != nil {
		t.Fatal(err)
	}
	g.Nodes["surface"].Outputs = []ArtifactType{ArtifactURL}
	g.Nodes["tech"].Outputs = []ArtifactType{ArtifactTechnology}
	g.Nodes["adaptive"].Inputs = []ArtifactType{ArtifactURL}
	g.Nodes["adaptive"].Outputs = []ArtifactType{ArtifactFinding}
	g.Edges = append(g.Edges, Edge{
		From: "tech", To: "adaptive", Condition: "contains:wordpress",
		Label: "WordPress detected", Control: true,
	})

	if err := g.Validate(); err != nil {
		t.Fatalf("valid data+control adaptive branch rejected: %v", err)
	}
}

func TestValidateRejectsUnguardedIntrusiveNode(t *testing.T) {
	g := NewDAG()
	if err := g.AddNode("input", "active", "active-tool", "", 1); err != nil {
		t.Fatal(err)
	}
	g.Nodes["active"].Policy = NodePolicy{Intrusive: true, Approval: "active-validation"}

	err := g.Validate()
	if err == nil || !strings.Contains(err.Error(), "not behind an approval gate") {
		t.Fatalf("expected approval-boundary error, got %v", err)
	}
}

func TestValidateAcceptsMatchingApprovalBoundary(t *testing.T) {
	g := NewDAG()
	if err := g.AddNode("input", "approval-gate", "approval", "", 1); err != nil {
		t.Fatal(err)
	}
	if err := g.AddNode("approval-gate", "active", "active-tool", "", 2); err != nil {
		t.Fatal(err)
	}
	gate := g.Nodes["approval-gate"]
	gate.Kind = NodeKindGate
	gate.Inputs = []ArtifactType{ArtifactDomain}
	gate.Outputs = []ArtifactType{ArtifactParameter}
	gate.Policy = NodePolicy{RequiresApproval: true, Approval: "active-validation"}
	active := g.Nodes["active"]
	active.Inputs = []ArtifactType{ArtifactParameter}
	active.Outputs = []ArtifactType{ArtifactFinding}
	active.Policy = NodePolicy{Intrusive: true, Approval: "active-validation"}

	if err := g.Validate(); err != nil {
		t.Fatalf("matching approval boundary rejected: %v", err)
	}
}

func TestValidateRejectsMismatchedApprovalBoundary(t *testing.T) {
	g := NewDAG()
	if err := g.AddNode("input", "approval-gate", "approval", "", 1); err != nil {
		t.Fatal(err)
	}
	if err := g.AddNode("approval-gate", "active", "active-tool", "", 2); err != nil {
		t.Fatal(err)
	}
	gate := g.Nodes["approval-gate"]
	gate.Kind = NodeKindGate
	gate.Policy = NodePolicy{RequiresApproval: true, Approval: "manual-review"}
	active := g.Nodes["active"]
	active.Policy = NodePolicy{Intrusive: true, Approval: "active-validation"}

	err := g.Validate()
	if err == nil || !strings.Contains(err.Error(), "active-validation") {
		t.Fatalf("expected mismatched approval error, got %v", err)
	}
}
