package graph

import (
	"strings"
	"testing"
)

func TestParseMermaidShapes(t *testing.T) {
	src := `flowchart LR
  w["worker\n-x"]
  g{"gate"}
  m(("merge"))
  c(["checkpoint"])
  s[["sink"]]
  r("rounded")
  w --> g
  g --> m
  m --> c
  c --> s
  s --> r`
	dag, err := ParseMermaid(src)
	if err != nil {
		t.Fatalf("ParseMermaid: %v", err)
	}
	want := map[string]NodeKind{
		"w": NodeKindWorker,
		"g": NodeKindGate,
		"m": NodeKindMerge,
		"c": NodeKindCheckpoint,
		"s": NodeKindSink,
		"r": NodeKindWorker,
	}
	for id, kind := range want {
		n, ok := dag.Nodes[id]
		if !ok {
			t.Fatalf("node %q missing", id)
		}
		if n.EffectiveKind() != kind {
			t.Errorf("node %q kind = %q, want %q", id, n.EffectiveKind(), kind)
		}
	}
}

func TestParseMermaidHyphenIDsAndArgs(t *testing.T) {
	src := `graph LR
  subfinder-1["subfinder\n-d {{domain}} -silent -o {{output}}"]`
	dag, err := ParseMermaid(src)
	if err != nil {
		t.Fatalf("ParseMermaid: %v", err)
	}
	n, ok := dag.Nodes["subfinder-1"]
	if !ok {
		t.Fatalf("node subfinder-1 missing (ids: %v)", nodeIDs(dag))
	}
	if n.Tool != "subfinder" {
		t.Errorf("tool = %q, want subfinder", n.Tool)
	}
	if n.Args != "-d {{domain}} -silent -o {{output}}" {
		t.Errorf("args = %q", n.Args)
	}
}

func TestParseMermaidInlineDefEdgeMapsRoot(t *testing.T) {
	src := `graph LR
  input([Start]) --> subfinder-1["subfinder\n-d {{domain}}"]`
	dag, err := ParseMermaid(src)
	if err != nil {
		t.Fatalf("ParseMermaid: %v", err)
	}
	// input must resolve to the existing root, not a new node.
	if _, ok := dag.Nodes["input"]; !ok {
		t.Fatal("root input missing")
	}
	if len(dag.Nodes) != 2 { // input + subfinder-1
		t.Fatalf("node count = %d, want 2 (%v)", len(dag.Nodes), nodeIDs(dag))
	}
	parents := dag.Parents("subfinder-1")
	if len(parents) != 1 || parents[0] != "input" {
		t.Errorf("parents(subfinder-1) = %v, want [input]", parents)
	}
}

func TestParseMermaidChainedEdges(t *testing.T) {
	src := `flowchart LR
  a["echo"] --> b["echo"] --> c["echo"]`
	dag, err := ParseMermaid(src)
	if err != nil {
		t.Fatalf("ParseMermaid: %v", err)
	}
	if got := dag.Parents("b"); len(got) != 1 || got[0] != "a" {
		t.Errorf("parents(b) = %v, want [a]", got)
	}
	if got := dag.Parents("c"); len(got) != 1 || got[0] != "b" {
		t.Errorf("parents(c) = %v, want [b]", got)
	}
	if dag.Nodes["a"].Layer != 1 || dag.Nodes["b"].Layer != 2 || dag.Nodes["c"].Layer != 3 {
		t.Errorf("layers a/b/c = %d/%d/%d, want 1/2/3", dag.Nodes["a"].Layer, dag.Nodes["b"].Layer, dag.Nodes["c"].Layer)
	}
}

// The most important regression: a decorative pipe label must NOT become a
// condition, or RunDAG's readiness gating would skip the whole downstream graph.
func TestParseMermaidLabelIsNotCondition(t *testing.T) {
	src := `graph LR
  input --> subfinder-1["subfinder\n-x"]
  subfinder-1 -->|sequential| httpx-1["httpx\n-y"]`
	dag, err := ParseMermaid(src)
	if err != nil {
		t.Fatalf("ParseMermaid: %v", err)
	}
	var found bool
	for _, e := range dag.Edges {
		if e.From == "subfinder-1" && e.To == "httpx-1" {
			found = true
			if e.Label != "sequential" {
				t.Errorf("edge label = %q, want sequential", e.Label)
			}
			if e.Condition != "" {
				t.Errorf("edge condition = %q, want empty", e.Condition)
			}
		}
	}
	if !found {
		t.Fatalf("edge subfinder-1->httpx-1 missing (edges: %v)", dag.Edges)
	}
}

func TestParseMermaidDottedConditionEdge(t *testing.T) {
	src := `graph LR
  input --> a["nuclei\n-x"]
  a -. "nonempty" .-> b["httpx\n-y"]`
	dag, err := ParseMermaid(src)
	if err != nil {
		t.Fatalf("ParseMermaid: %v", err)
	}
	for _, e := range dag.Edges {
		if e.From == "a" && e.To == "b" {
			if e.Condition != "nonempty" {
				t.Errorf("condition = %q, want nonempty", e.Condition)
			}
			return
		}
	}
	t.Fatalf("edge a->b missing (edges: %v)", dag.Edges)
}

func TestParseMermaidEntityUnescape(t *testing.T) {
	src := `flowchart LR
  n["assetfinder\n--subs-only {{domain}} &gt; {{output}} &amp;&amp; echo done"]`
	dag, err := ParseMermaid(src)
	if err != nil {
		t.Fatalf("ParseMermaid: %v", err)
	}
	args := dag.Nodes["n"].Args
	if !strings.Contains(args, "> {{output}}") {
		t.Errorf("args missing unescaped '>': %q", args)
	}
	if !strings.Contains(args, "&& echo done") {
		t.Errorf("args missing unescaped '&&': %q", args)
	}
}

func TestParseMermaidSubgraphSkipsLayerWrappers(t *testing.T) {
	dag, err := ParseMermaid(sampleWorkflowMMD)
	if err != nil {
		t.Fatalf("ParseMermaid: %v", err)
	}
	if _, ok := dag.Subgraphs["subdomain_parallel"]; !ok {
		t.Error("real subgraph subdomain_parallel missing")
	}
	if _, ok := dag.Subgraphs["scanning_parallel"]; !ok {
		t.Error("real subgraph scanning_parallel missing")
	}
	if _, ok := dag.Subgraphs["L2"]; ok {
		t.Error("layer wrapper L2 should not be a subgraph")
	}
	// A node inside the L2 wrapper stays top-level.
	if n := dag.Nodes["httpx-1"]; n == nil || n.Subgraph != "" {
		t.Errorf("httpx-1 should be top-level, got subgraph %q", subgraphOf(dag, "httpx-1"))
	}
}

func TestParseMermaidOrphanAttachedToRoot(t *testing.T) {
	dag, err := ParseMermaid(sampleWorkflowMMD)
	if err != nil {
		t.Fatalf("ParseMermaid: %v", err)
	}
	// ffuf-1 has no incoming edge in the sample; it must be seeded from root.
	if got := dag.Parents("ffuf-1"); len(got) != 1 || got[0] != "input" {
		t.Errorf("parents(ffuf-1) = %v, want [input]", got)
	}
}

func TestParseMermaidSampleValidates(t *testing.T) {
	dag, err := ParseMermaid(sampleWorkflowMMD)
	if err != nil {
		t.Fatalf("ParseMermaid: %v", err)
	}
	if err := dag.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if err := dag.ValidateMatrix(); err != nil {
		t.Fatalf("ValidateMatrix: %v", err)
	}
	// No coordinate may hold more than one node.
	for coord, nodes := range dag.Matrix {
		if len(nodes) > 1 {
			t.Errorf("coordinate %v holds %d nodes", coord, len(nodes))
		}
	}
}

func TestParseMermaidErrors(t *testing.T) {
	cases := map[string]string{
		"unmatched end":     "flowchart LR\n  a[\"echo\"]\n  end",
		"unclosed subgraph": "flowchart LR\n  subgraph grp[\"G\"]\n    a[\"echo\"]",
		"no nodes":          "flowchart LR\n  classDef x fill:#000",
	}
	for name, src := range cases {
		if _, err := ParseMermaid(src); err == nil {
			t.Errorf("%s: expected error, got nil", name)
		}
	}
}

func TestParseMermaidRoundTripSimple(t *testing.T) {
	// Underscore ids avoid safeMermaidID rewriting during ToMermaid.
	g := NewDAG()
	if err := g.AddNodeAtPosition(g.Root, "sub_1", "subfinder", "-d {{domain}}", 1, 0, "", false); err != nil {
		t.Fatal(err)
	}
	if err := g.AddNodeAtPosition("sub_1", "http_1", "httpx", "-silent", 2, 0, "", false); err != nil {
		t.Fatal(err)
	}
	round, err := ParseMermaid(g.ToMermaid())
	if err != nil {
		t.Fatalf("ParseMermaid(ToMermaid): %v", err)
	}
	for _, id := range []string{"sub_1", "http_1"} {
		if _, ok := round.Nodes[id]; !ok {
			t.Errorf("round-trip lost node %q (%v)", id, nodeIDs(round))
		}
	}
	if got := round.Parents("http_1"); len(got) != 1 || got[0] != "sub_1" {
		t.Errorf("round-trip parents(http_1) = %v, want [sub_1]", got)
	}
}

/*──────────────────────── helpers ─────────────────────────*/

func nodeIDs(g *DAG) []string {
	ids := make([]string, 0, len(g.Nodes))
	for id := range g.Nodes {
		ids = append(ids, id)
	}
	return ids
}

func subgraphOf(g *DAG, id string) string {
	if n := g.Nodes[id]; n != nil {
		return n.Subgraph
	}
	return ""
}

const sampleWorkflowMMD = `graph LR
  subgraph subdomain_parallel["Parallel Subdomain Discovery"]
    subfinder-1["subfinder\n-d {{domain}} -silent -o {{output}}"]
    assetfinder-1["assetfinder\n--subs-only {{domain}} -o {{output}}"]
  end

  subgraph L2["Layer 2"]
    httpx-1["httpx\n-l {{input}} -title -tech-detect -json -silent -o {{output}}"]
    ffuf-1["ffuf\n-u {{input}}/FUZZ -mc 200 -o {{output}}"]
  end

  subgraph scanning_parallel["Parallel Vulnerability Scanning"]
    nuclei-1["nuclei\n-l {{input}} -severity medium,high,critical -silent -o {{output}}"]
    nuclei-2["nuclei\n-l {{input}} -severity high,critical -silent -o {{output}}"]
  end

  input([Start]) --> subfinder-1
  input --> assetfinder-1
  subfinder-1 -->|sequential| httpx-1
  assetfinder-1 -->|sequential| httpx-1
  httpx-1 -->|sequential| nuclei-1
  ffuf-1 -->|sequential| nuclei-2`
