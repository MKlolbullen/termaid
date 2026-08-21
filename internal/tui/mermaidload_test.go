package tui

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/MKlolbullen/termaid/internal/pipeline"
)

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// A chart that labels a node with a tool but no args should inherit that tool's
// catalog default args, so a minimally authored chart is still runnable.
func TestLoadWorkflowAnyFillsArgsFromCatalog(t *testing.T) {
	path := writeTemp(t, "chart.mmd", "flowchart LR\n  input([Start]) --> sub[\"subfinder\"]\n")
	dag, err := LoadWorkflowAny(path)
	if err != nil {
		t.Fatalf("LoadWorkflowAny: %v", err)
	}
	n := dag.Nodes["sub"]
	if n == nil || n.Tool != "subfinder" {
		t.Fatalf("node sub missing or wrong tool: %#v", n)
	}
	want := defaultArgs("subfinder")
	if want == "" {
		t.Skip("subfinder has no catalog default args")
	}
	if n.Args != want {
		t.Errorf("args = %q, want catalog default %q", n.Args, want)
	}
}

// A bare, unlabelled node id like "subfinder-1" should resolve to the catalog
// tool "subfinder" so the chart validates and runs.
func TestLoadWorkflowAnyDerivesToolFromID(t *testing.T) {
	path := writeTemp(t, "chart.mmd", "flowchart LR\n  input --> subfinder-1\n")
	dag, err := LoadWorkflowAny(path)
	if err != nil {
		t.Fatalf("LoadWorkflowAny: %v", err)
	}
	n := dag.Nodes["subfinder-1"]
	if n == nil {
		t.Fatal("node subfinder-1 missing")
	}
	if n.Tool != "subfinder" {
		t.Errorf("tool = %q, want subfinder (derived from id)", n.Tool)
	}
	if err := dag.Validate(); err != nil {
		t.Fatalf("Validate after hydration: %v", err)
	}
}

// The headline flow: author a Mermaid chart, load it, and run it. Uses `echo`
// (always on PATH) so the pipeline actually executes without recon tools.
func TestLoadWorkflowAnyMermaidRunsWithEcho(t *testing.T) {
	mmd := "flowchart LR\n" +
		"  input([Start]) --> probe[\"echo\\nsubdomains-for {{domain}}\"]\n" +
		"  probe -->|sequential| tag[\"echo\\nprocessed {{input}}\"]\n"
	path := writeTemp(t, "chart.mmd", mmd)

	dag, err := LoadWorkflowAny(path)
	if err != nil {
		t.Fatalf("LoadWorkflowAny: %v", err)
	}
	if err := dag.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	workdir := t.TempDir()
	ch := make(chan pipeline.Status, 128)
	errCh := make(chan error, 1)
	go func() {
		errCh <- pipeline.RunDAG(context.Background(), "example.com", workdir, dag, pipeline.RunConfig{Concurrency: 2}, ch)
		close(ch)
	}()

	finished := map[string]bool{}
	var runErrors int
	for st := range ch {
		switch st.Type {
		case pipeline.StatusFinish:
			finished[st.Tool] = true
		case pipeline.StatusError:
			runErrors++
			t.Errorf("unexpected node error on %s: %v", st.Tool, st.Err)
		}
	}
	if err := <-errCh; err != nil {
		t.Fatalf("RunDAG: %v", err)
	}
	if runErrors != 0 {
		t.Fatalf("expected 0 node errors, got %d", runErrors)
	}
	for _, id := range []string{"probe", "tag"} {
		if !finished[id] {
			t.Errorf("node %q did not finish", id)
		}
	}
}

// A .json workflow must still load through the JSON path unchanged.
func TestLoadWorkflowAnyJSONUnchanged(t *testing.T) {
	json := `{"version":"2.0","workflow":[{"id":"first","tool":"subfinder","children":[],"layer":1,"position":0}]}`
	path := writeTemp(t, "wf.json", json)
	dag, err := LoadWorkflowAny(path)
	if err != nil {
		t.Fatalf("LoadWorkflowAny(json): %v", err)
	}
	if dag.Nodes["first"] == nil {
		t.Fatal("json workflow node missing")
	}
}
