package graph

import (
	"fmt"
	"sort"
	"strings"
)

// NodeKind describes how a workflow node executes.
type NodeKind string

const (
	NodeKindWorker     NodeKind = "worker"
	NodeKindMerge      NodeKind = "merge"
	NodeKindGate       NodeKind = "gate"
	NodeKindTransform  NodeKind = "transform"
	NodeKindCheckpoint NodeKind = "checkpoint"
	NodeKindSink       NodeKind = "sink"
	NodeKindSource     NodeKind = "source"
	NodeKindManual     NodeKind = "manual"
)

// ArtifactType is the typed contract passed between workflow nodes.
type ArtifactType string

const (
	ArtifactAny        ArtifactType = "any"
	ArtifactDomain     ArtifactType = "domain"
	ArtifactHost       ArtifactType = "host"
	ArtifactService    ArtifactType = "service"
	ArtifactURL        ArtifactType = "url"
	ArtifactParameter  ArtifactType = "parameter"
	ArtifactTechnology ArtifactType = "technology"
	ArtifactFinding    ArtifactType = "finding"
	ArtifactEvidence   ArtifactType = "evidence"
	ArtifactReport     ArtifactType = "report"
)

// Edge is a first-class dependency. Condition uses the compact condition DSL
// implemented by the runtime (always, nonempty, has_type:<type>, contains:<s>,
// approved:<gate>, parent_success:<node>). Empty means always.
type Edge struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Condition string `json:"condition,omitempty"`
	Label     string `json:"label,omitempty"`
}

// NodePolicy lets a workflow distinguish passive/discovery work from actions
// that require an explicit authorization decision at run time.
type NodePolicy struct {
	Intrusive        bool   `json:"intrusive,omitempty"`
	RequiresApproval bool   `json:"requires_approval,omitempty"`
	Approval         string `json:"approval,omitempty"`
	ContinueOnError  bool   `json:"continue_on_error,omitempty"`
}

// NodeExecution defines per-node reliability controls.
type NodeExecution struct {
	TimeoutSeconds int `json:"timeout_seconds,omitempty"`
	Retries        int `json:"retries,omitempty"`
	RetryBackoffMS int `json:"retry_backoff_ms,omitempty"`
}

// WorkflowPolicy applies guardrails to the entire DAG.
type WorkflowPolicy struct {
	AllowedRoots   []string `json:"allowed_roots,omitempty"`
	Excluded       []string `json:"excluded,omitempty"`
	AllowIntrusive bool     `json:"allow_intrusive,omitempty"`
	MaxConcurrency int      `json:"max_concurrency,omitempty"`
}

// Validate checks both the visual matrix and executable graph semantics.
func (g *DAG) Validate() error {
	if err := g.ValidateMatrix(); err != nil {
		return err
	}
	g.EnsureEdges()
	if _, ok := g.Nodes[g.Root]; !ok {
		return fmt.Errorf("root node %q not found", g.Root)
	}

	for _, e := range g.Edges {
		from, ok := g.Nodes[e.From]
		if !ok {
			return fmt.Errorf("edge source %q not found", e.From)
		}
		to, ok := g.Nodes[e.To]
		if !ok {
			return fmt.Errorf("edge destination %q not found", e.To)
		}
		if e.From == e.To {
			return fmt.Errorf("self edge on node %q", e.From)
		}
		if !artifactCompatible(from.Outputs, to.Inputs) {
			return fmt.Errorf("artifact contract mismatch %s -> %s: outputs=%v inputs=%v", e.From, e.To, from.Outputs, to.Inputs)
		}
	}

	for id, n := range g.Nodes {
		if id == g.Root {
			continue
		}
		switch n.EffectiveKind() {
		case NodeKindWorker, NodeKindMerge, NodeKindGate, NodeKindTransform, NodeKindCheckpoint, NodeKindSink, NodeKindManual:
		default:
			return fmt.Errorf("node %q has unknown kind %q", id, n.Kind)
		}
		if n.EffectiveKind() == NodeKindWorker && strings.TrimSpace(n.Tool) == "" {
			return fmt.Errorf("worker node %q has no tool", id)
		}
		if n.Execution.TimeoutSeconds < 0 || n.Execution.Retries < 0 || n.Execution.RetryBackoffMS < 0 {
			return fmt.Errorf("node %q has negative execution controls", id)
		}
	}

	if cycle := g.findCycle(); len(cycle) > 0 {
		return fmt.Errorf("workflow contains a cycle: %s", strings.Join(cycle, " -> "))
	}
	return nil
}

// TopologicalOrder returns deterministic dependency order independent of the
// visual matrix. It is useful for validation, reporting and schedulers.
func (g *DAG) TopologicalOrder() ([]string, error) {
	g.EnsureEdges()
	indegree := make(map[string]int, len(g.Nodes))
	for id := range g.Nodes {
		indegree[id] = 0
	}
	for _, e := range g.Edges {
		indegree[e.To]++
	}
	var ready []string
	for id, degree := range indegree {
		if degree == 0 {
			ready = append(ready, id)
		}
	}
	sort.Strings(ready)

	var order []string
	for len(ready) > 0 {
		id := ready[0]
		ready = ready[1:]
		order = append(order, id)
		for _, e := range g.OutgoingEdges(id) {
			indegree[e.To]--
			if indegree[e.To] == 0 {
				ready = append(ready, e.To)
				sort.Strings(ready)
			}
		}
	}
	if len(order) != len(g.Nodes) {
		return nil, fmt.Errorf("workflow is not acyclic")
	}
	return order, nil
}

func artifactCompatible(outputs, inputs []ArtifactType) bool {
	if len(outputs) == 0 || len(inputs) == 0 {
		return true // v2/untyped compatibility
	}
	for _, out := range outputs {
		for _, in := range inputs {
			if out == ArtifactAny || in == ArtifactAny || out == in {
				return true
			}
		}
	}
	return false
}

func (g *DAG) findCycle() []string {
	const (
		white = iota
		gray
		black
	)
	state := map[string]int{}
	stack := []string{}
	var cycle []string
	var visit func(string) bool
	visit = func(id string) bool {
		state[id] = gray
		stack = append(stack, id)
		for _, e := range g.OutgoingEdges(id) {
			next := e.To
			switch state[next] {
			case gray:
				start := 0
				for i, v := range stack {
					if v == next {
						start = i
						break
					}
				}
				cycle = append(append([]string{}, stack[start:]...), next)
				return true
			case white:
				if visit(next) {
					return true
				}
			}
		}
		stack = stack[:len(stack)-1]
		state[id] = black
		return false
	}

	ids := make([]string, 0, len(g.Nodes))
	for id := range g.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if state[id] == white && visit(id) {
			return cycle
		}
	}
	return nil
}
