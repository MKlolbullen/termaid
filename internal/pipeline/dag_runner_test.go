package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MKlolbullen/termaid/internal/graph"
)

func TestConditionMatches(t *testing.T) {
	df, err := NewDataFlow(t.TempDir(), "example.com")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(df.WorkDir, df.RunID, "raw", "condition.txt")
	if err := os.WriteFile(path, []byte("WordPress 6.7\nhttps://example.com/?id=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := RunConfig{Approvals: map[string]bool{"active": true}}

	cases := []struct {
		condition string
		want      bool
	}{
		{"nonempty", true},
		{"contains:wordpress", true},
		{"contains:drupal", false},
		{"approved:active", true},
		{"approved:manual", false},
		{"contains:wordpress && approved:active", true},
		{"contains:drupal || approved:active", true},
	}
	for _, tc := range cases {
		if got := conditionMatches(tc.condition, []string{path}, df, cfg); got != tc.want {
			t.Errorf("condition %q = %v, want %v", tc.condition, got, tc.want)
		}
	}
}

func TestPolicyAllowsIntrusiveOnlyWithApproval(t *testing.T) {
	node := &graph.Node{
		ID: "sqlmap-1",
		Policy: graph.NodePolicy{Intrusive: true, Approval: "active-validation"},
	}
	workflow := graph.WorkflowPolicy{AllowIntrusive: false}

	if ok, _ := policyAllows(node, workflow, RunConfig{Approvals: map[string]bool{}}); ok {
		t.Fatal("intrusive node should be denied without approval")
	}
	if ok, reason := policyAllows(node, workflow, RunConfig{Approvals: map[string]bool{"active-validation": true}}); !ok {
		t.Fatalf("approved intrusive node denied: %s", reason)
	}
}

func TestTargetPolicyHonorsRootsAndExclusions(t *testing.T) {
	policy := graph.WorkflowPolicy{
		AllowedRoots: []string{"{{domain}}"},
		Excluded:     []string{"admin.example.com"},
	}
	if !targetAllowed("example.com", policy) {
		t.Fatal("root target should be allowed")
	}
	if artifactInScope("https://admin.example.com/panel", "example.com", policy) {
		t.Fatal("excluded subdomain should be rejected")
	}
	if !artifactInScope("https://api.example.com/v1", "example.com", policy) {
		t.Fatal("allowed subdomain should be in scope")
	}
	if artifactInScope("https://example.net", "example.com", policy) {
		t.Fatal("different root should be out of scope")
	}
}

func TestCorrelateRecordsPreservesSources(t *testing.T) {
	records := []DataRecord{
		{Value: "same finding", Type: "finding", Source: "nuclei", Confidence: 0.7, Metadata: map[string]string{"asset": "example.com", "location": "/admin", "weakness": "exposure", "evidence": "200"}},
		{Value: "same finding with richer text", Type: "finding", Source: "jaeles", Confidence: 0.9, Metadata: map[string]string{"asset": "example.com", "location": "/admin", "weakness": "exposure", "evidence": "200"}},
	}
	out := CorrelateRecords(records)
	if len(out) != 1 {
		t.Fatalf("correlated count = %d, want 1", len(out))
	}
	if out[0].Confidence != 0.9 {
		t.Fatalf("confidence = %v, want 0.9", out[0].Confidence)
	}
	sources := out[0].Metadata["sources"]
	if !strings.Contains(sources, "nuclei") || !strings.Contains(sources, "jaeles") {
		t.Fatalf("provenance sources lost: %q", sources)
	}
	if out[0].Metadata["correlation_key"] == "" {
		t.Fatal("missing correlation key")
	}
}

func TestCheckpointRoundTrip(t *testing.T) {
	workdir := t.TempDir()
	df, err := NewDataFlow(workdir, "example.com")
	if err != nil {
		t.Fatal(err)
	}
	seed, err := df.CreateSeedFile()
	if err != nil {
		t.Fatal(err)
	}
	df.NodeOutputs["done"] = &NodeOutput{
		NodeID:      "done",
		Tool:        "builtin:test",
		StartTime:   time.Now(),
		EndTime:     time.Now(),
		ExitCode:    0,
		OutputFiles: []string{seed},
		Metadata:    map[string]string{"kind": "checkpoint"},
	}
	df.GlobalState.NodeStates["done"] = NodeCompleted
	if err := df.SaveCheckpoint(); err != nil {
		t.Fatal(err)
	}

	resumed, err := ResumeDataFlow(workdir, df.RunID, "example.com")
	if err != nil {
		t.Fatal(err)
	}
	if resumed.GlobalState.NodeStates["done"] != NodeCompleted {
		t.Fatalf("resumed state = %v, want completed", resumed.GlobalState.NodeStates["done"])
	}
	if resumed.NodeOutputs["done"] == nil {
		t.Fatal("resumed output missing")
	}
}

func TestRunDAGWithBuiltinPrimitives(t *testing.T) {
	g := graph.NewDAG()
	if err := g.AddNode("input", "scope", "scope", "", 1); err != nil {
		t.Fatal(err)
	}
	if err := g.AddNode("scope", "checkpoint", "checkpoint", "", 2); err != nil {
		t.Fatal(err)
	}
	if err := g.AddNode("checkpoint", "report", "report", "", 3); err != nil {
		t.Fatal(err)
	}

	g.Policy = graph.WorkflowPolicy{AllowedRoots: []string{"{{domain}}"}, MaxConcurrency: 2}
	g.Nodes["scope"].Kind = graph.NodeKindGate
	g.Nodes["scope"].Transform = "scope"
	g.Nodes["scope"].Inputs = []graph.ArtifactType{graph.ArtifactDomain}
	g.Nodes["scope"].Outputs = []graph.ArtifactType{graph.ArtifactDomain}
	g.Nodes["checkpoint"].Kind = graph.NodeKindCheckpoint
	g.Nodes["checkpoint"].Inputs = []graph.ArtifactType{graph.ArtifactDomain}
	g.Nodes["checkpoint"].Outputs = []graph.ArtifactType{graph.ArtifactDomain}
	g.Nodes["report"].Kind = graph.NodeKindSink
	g.Nodes["report"].Inputs = []graph.ArtifactType{graph.ArtifactDomain}
	g.Nodes["report"].Outputs = []graph.ArtifactType{graph.ArtifactReport}

	workdir := t.TempDir()
	status := make(chan Status, 32)
	if err := RunDAG(context.Background(), "example.com", workdir, g, RunConfig{Concurrency: 8}, status); err != nil {
		t.Fatalf("RunDAG: %v", err)
	}
	close(status)

	entries, err := os.ReadDir(workdir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || !entries[0].IsDir() {
		t.Fatalf("expected one run directory, got %v", entries)
	}
	runID := entries[0].Name()
	if _, err := os.Stat(filepath.Join(workdir, runID, checkpointFileName)); err != nil {
		t.Fatalf("checkpoint missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workdir, runID, "execution-report.json")); err != nil {
		t.Fatalf("execution report missing: %v", err)
	}

	resumed, err := ResumeDataFlow(workdir, runID, "example.com")
	if err != nil {
		t.Fatal(err)
	}
	if resumed.GlobalState.NodeStates["report"] != NodeCompleted {
		t.Fatalf("report state = %v, want completed", resumed.GlobalState.NodeStates["report"])
	}
}
