package tui

import (
	"os"
	"testing"
)

// Exercises the builder's Save/Load wiring (the header buttons' real work)
// without the TTY event loop: build a graph, persist it, and reload it.
func TestBuilderSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(prev)

	b := NewBuilder(catalogueNames())
	if err := b.g.AddNode("input", "subfinder-1", "subfinder", "-d {{domain}} -o {{output}}", 1); err != nil {
		t.Fatal(err)
	}
	if err := b.g.AddNode("subfinder-1", "httpx-1", "httpx", "-l {{input}} -o {{output}}", 2); err != nil {
		t.Fatal(err)
	}

	if err := b.saveWorkflow(); err != nil {
		t.Fatalf("saveWorkflow: %v", err)
	}
	for _, f := range []string{defaultWorkflowFile, defaultMermaidFile} {
		if _, err := os.Stat(f); err != nil {
			t.Fatalf("expected %s to be written: %v", f, err)
		}
	}

	// Both artifacts must be independently loadable and valid.
	for _, f := range []string{defaultWorkflowFile, defaultMermaidFile} {
		dag, err := LoadWorkflowAny(f)
		if err != nil {
			t.Fatalf("LoadWorkflowAny(%s): %v", f, err)
		}
		if err := dag.Validate(); err != nil {
			t.Fatalf("%s did not validate: %v", f, err)
		}
		if workers := len(dag.Nodes) - 1; workers != 2 { // minus the implicit root
			t.Fatalf("%s has %d worker nodes after round-trip, want 2", f, workers)
		}
	}
	// JSON is the lossless source of truth: hyphenated ids survive exactly.
	// (ToMermaid sanitizes "subfinder-1" -> "subfinder_1", so .mmd ids may differ.)
	jsonDAG, err := LoadWorkflowAny(defaultWorkflowFile)
	if err != nil {
		t.Fatal(err)
	}
	if jsonDAG.Nodes["subfinder-1"] == nil || jsonDAG.Nodes["httpx-1"] == nil {
		t.Fatal("workflow.json lost its hyphenated node ids")
	}

	// Loading into a fresh builder repopulates the graph and the occurrence
	// counter so new nodes get non-colliding ids.
	b2 := NewBuilder(catalogueNames())
	if err := b2.loadWorkflow(); err != nil {
		t.Fatalf("loadWorkflow: %v", err)
	}
	if b2.g.Nodes["subfinder-1"] == nil {
		t.Fatal("loaded graph missing subfinder-1")
	}
	if b2.occ["subfinder"] != 1 || b2.occ["httpx"] != 1 {
		t.Errorf("occurrence counter = %v, want subfinder:1 httpx:1", b2.occ)
	}
}
