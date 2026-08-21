package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MKlolbullen/termaid/internal/graph"
)

func TestReferenceWebAssessmentV3IsValid(t *testing.T) {
	path := filepath.Join("..", "..", "templates", "webapp-comprehensive-scan.json")
	dag, err := LoadWorkflowV3(path)
	if err != nil {
		t.Fatalf("LoadWorkflowV3(%s): %v", path, err)
	}
	if dag.Version != "3.0" {
		t.Fatalf("version = %q, want 3.0", dag.Version)
	}
	if err := dag.Validate(); err != nil {
		t.Fatalf("reference v3 template is invalid: %v", err)
	}

	gate := dag.Nodes["active-validation-gate"]
	if gate == nil || gate.EffectiveKind() != graph.NodeKindGate {
		t.Fatalf("active-validation-gate is missing or not a gate: %#v", gate)
	}
	if !gate.Policy.RequiresApproval || gate.Policy.Approval != "active-validation" {
		t.Fatalf("active validation approval policy not enforced: %#v", gate.Policy)
	}

	var adaptiveControlEdges int
	for _, edge := range dag.Edges {
		if edge.Control && (edge.To == "wordpress-nuclei-1" || edge.To == "graphql-nuclei-1") {
			adaptiveControlEdges++
		}
	}
	if adaptiveControlEdges != 2 {
		t.Fatalf("adaptive control edge count = %d, want 2", adaptiveControlEdges)
	}
}

func TestLoadWorkflowV3PromotesLegacyChildren(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.json")
	legacy := `{
  "version": "2.0",
  "matrix": {"max_x": 2, "max_y": 0},
  "workflow": [
    {
      "id": "first",
      "tool": "subfinder",
      "children": ["second"],
      "layer": 1,
      "position": 0
    },
    {
      "id": "second",
      "tool": "httpx",
      "children": [],
      "layer": 2,
      "position": 0
    }
  ]
}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}

	dag, err := LoadWorkflowV3(path)
	if err != nil {
		t.Fatal(err)
	}
	if dag.Nodes["first"].EffectiveKind() != graph.NodeKindWorker {
		t.Fatalf("legacy kind = %q, want worker", dag.Nodes["first"].EffectiveKind())
	}

	found := false
	for _, edge := range dag.Edges {
		if edge.From == "first" && edge.To == "second" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("legacy child relationship was not promoted to a semantic edge")
	}
}
