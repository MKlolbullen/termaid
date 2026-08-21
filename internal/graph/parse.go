package graph

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// ParseMermaid converts a Mermaid `graph`/`flowchart` diagram into a DAG so that
// a chart authored by hand (e.g. in mermaid.live) or saved by the visual builder
// can be validated and executed. It is the inverse of DAG.ToMermaid.
//
// The parser is deliberately lenient: styling directives (classDef/class/style/
// linkStyle/click), the header line, and lines it does not understand are
// skipped rather than rejected. It errors only on structural problems it cannot
// recover from (an unmatched subgraph/end or a diagram with no nodes).
//
// Node semantics come from the Mermaid shape and label:
//
//	id["tool\nargs"]   worker   (also id("...") rounded)
//	id{"..."}          gate
//	id(("..."))        merge
//	id(["..."])        checkpoint
//	id[["..."]]        sink
//
// A label is parsed as "tool" on the first line and the remaining lines (split on
// a literal \n or <br>) as the argument string; «kind» and → output annotations
// emitted by ToMermaid are ignored. Typed artifact contracts (Inputs/Outputs) are
// intentionally left empty so a parsed graph never fails Validate on a contract
// mismatch. Full-fidelity persistence remains JSON (ToJSON/LoadWorkflowV3); a
// re-rendered .mmd is lossy (truncated args, sanitized ids).
func ParseMermaid(src string) (*DAG, error) {
	p := &mermaidParser{g: NewDAG()}
	if err := p.scan(src); err != nil {
		return nil, err
	}
	if len(p.g.Nodes) <= 1 { // only the implicit root
		return nil, fmt.Errorf("mermaid source contains no nodes")
	}
	p.attachOrphansToRoot()
	p.assignLayers()
	return p.g, nil
}

type mermaidParser struct {
	g     *DAG
	stack []subgraphFrame
}

type subgraphFrame struct {
	id   string
	real bool // false for layer-wrapper (L%d) or anonymous subgraphs
}

/*──────────────────────── line-level regexes ─────────────────────────*/

var (
	reHeader   = regexp.MustCompile(`^(?:graph|flowchart)\b`)
	reSkip     = regexp.MustCompile(`^(?:classDef|class|style|linkStyle|click)\b`)
	reSubgraph = regexp.MustCompile(`^subgraph\b\s*(.*)$`)
	reSubHead  = regexp.MustCompile(`^([A-Za-z0-9_-]+)?\s*(?:\[(.*)\])?\s*$`)

	// A standalone node token: an id with an optional shape body. Compound shapes
	// are listed before simple ones so the more specific delimiter wins.
	reNodeToken = regexp.MustCompile(`^([A-Za-z0-9_-]+)(\[\[.*\]\]|\(\(.*\)\)|\(\[.*\]\)|\[.*\]|\{.*\}|\(.*\))?$`)

	// Edge forms. Operators must be whitespace-delimited so the '-' inside ids
	// like "subfinder-1" is never mistaken for a link.
	rePipeLabel   = regexp.MustCompile(`^(.+?)\s+(?:--+>|==+>|-\.->)\s*\|([^|]*)\|\s*(.+)$`)
	reDotQuoted   = regexp.MustCompile(`^(.+?)\s+-\.\s+"([^"]*)"\s+\.->\s+(.+)$`)
	reDotUnquoted = regexp.MustCompile(`^(.+?)\s+-\.\s+([^|]+?)\s+\.->\s+(.+)$`)
	rePlainSplit  = regexp.MustCompile(`\s+(?:-\.->|--+>|==+>|-\.-|---+|===+)\s+`)
	// reLinkDetect decides whether a line is an edge. It covers plain/thick/dotted
	// arrows, pipe-labeled arrows, and the split dotted-label form `-. "x" .->`
	// (whose `.->` closer is matched on its own).
	reLinkDetect = regexp.MustCompile(`\s(?:-\.->|--+>|==+>|-\.-|---+|===+)\s|\s(?:--+>|==+>|-\.->)\s*\||\s\.->\s`)

	// Label line separators emitted by ToMermaid / used in hand-authored charts.
	reLabelSep = regexp.MustCompile(`\\n|<br\s*/?>`)
)

/*──────────────────────── scanning ─────────────────────────*/

func (p *mermaidParser) scan(src string) error {
	for _, raw := range strings.Split(src, "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case line == "":
			continue
		case strings.HasPrefix(line, "%%"): // comments and %%{init}%% directives
			continue
		case reHeader.MatchString(line), reSkip.MatchString(line):
			continue
		case strings.HasPrefix(line, "direction "):
			if id := p.currentSubgraph(); id != "" {
				if sg := p.g.Subgraphs[id]; sg != nil {
					sg.Parallel = true
				}
			}
			continue
		case line == "end":
			if len(p.stack) == 0 {
				return fmt.Errorf("mermaid: unmatched 'end'")
			}
			p.stack = p.stack[:len(p.stack)-1]
			continue
		case reSubgraph.MatchString(line):
			p.openSubgraph(reSubgraph.FindStringSubmatch(line)[1])
			continue
		}

		// A whitespace-bounded link operator marks an edge line (possibly with
		// inline node definitions on its endpoints). Checked before the node-def
		// case because a greedy shape body would otherwise swallow the arrows in
		// a chained line like `a["x"] --> b["y"] --> c["z"]`. Node labels do not
		// contain a spaced " --> ", so ordinary node lines fall through below.
		if reLinkDetect.MatchString(line) {
			p.parseEdgeLine(line, p.currentSubgraph())
			continue
		}
		// A whole-line single token (with or without a shape) is a node definition.
		if reNodeToken.MatchString(line) {
			p.registerNode(line, p.currentSubgraph())
			continue
		}
		// Anything else is ignored (lenient).
	}
	if len(p.stack) != 0 {
		return fmt.Errorf("mermaid: unclosed subgraph %q", p.stack[len(p.stack)-1].id)
	}
	return nil
}

func (p *mermaidParser) currentSubgraph() string {
	for i := len(p.stack) - 1; i >= 0; i-- {
		if p.stack[i].real {
			return p.stack[i].id
		}
	}
	return ""
}

func (p *mermaidParser) openSubgraph(rem string) {
	rem = strings.TrimSpace(rem)
	m := reSubHead.FindStringSubmatch(rem)
	if m == nil {
		p.stack = append(p.stack, subgraphFrame{real: false})
		return
	}
	id := m[1]
	title := stripQuotes(htmlUnescape(strings.TrimSpace(m[2])))
	// Layer wrappers (L0, L1, …) and anonymous subgraphs are presentation-only;
	// we recompute layers ourselves, so treat their contents as top-level.
	if id == "" || reLayerWrap.MatchString(id) {
		p.stack = append(p.stack, subgraphFrame{real: false})
		return
	}
	if p.g.Subgraphs[id] == nil {
		name := title
		if name == "" {
			name = id
		}
		p.g.Subgraphs[id] = &SubgraphInfo{
			ID:     id,
			Name:   name,
			Nodes:  []string{},
			Matrix: make(map[string]Coordinate),
		}
	}
	p.stack = append(p.stack, subgraphFrame{id: id, real: true})
}

var reLayerWrap = regexp.MustCompile(`^L\d+$`)

/*──────────────────────── node registration ─────────────────────────*/

// registerNode parses a single node token ("id" or "id[body]") and returns its
// canonical key, creating the node or upgrading it with a label if present. The
// implicit root ("input") is returned as-is and never duplicated.
func (p *mermaidParser) registerNode(token, subgraph string) string {
	m := reNodeToken.FindStringSubmatch(strings.TrimSpace(token))
	if m == nil {
		return ""
	}
	id, body := m[1], m[2]
	if id == p.g.Root {
		return p.g.Root
	}

	n, ok := p.g.Nodes[id]
	if !ok {
		n = &Node{ID: id, Children: []string{}}
		p.g.Nodes[id] = n
	}
	if body != "" {
		kind, label := parseBody(body)
		if n.Kind == "" {
			n.Kind = kind
		}
		if tool, args := parseLabel(label); tool != "" {
			n.Tool = tool
			if args != "" {
				n.Args = args
			}
		}
	}
	if subgraph != "" {
		p.attachSubgraph(id, subgraph)
	}
	return id
}

func (p *mermaidParser) attachSubgraph(id, subgraph string) {
	sg := p.g.Subgraphs[subgraph]
	if sg == nil {
		sg = &SubgraphInfo{ID: subgraph, Name: subgraph, Nodes: []string{}, Matrix: make(map[string]Coordinate)}
		p.g.Subgraphs[subgraph] = sg
	}
	if !containsString(sg.Nodes, id) {
		sg.Nodes = append(sg.Nodes, id)
	}
	if n := p.g.Nodes[id]; n != nil {
		n.Subgraph = subgraph
		n.SubX = len(sg.Nodes) - 1
		sg.Matrix[id] = Coordinate{X: n.SubX, Y: 0}
	}
}

// parseBody splits a shape body into its node kind and inner label.
func parseBody(body string) (NodeKind, string) {
	switch {
	case strings.HasPrefix(body, "[[") && strings.HasSuffix(body, "]]"):
		return NodeKindSink, strings.TrimSuffix(strings.TrimPrefix(body, "[["), "]]")
	case strings.HasPrefix(body, "((") && strings.HasSuffix(body, "))"):
		return NodeKindMerge, strings.TrimSuffix(strings.TrimPrefix(body, "(("), "))")
	case strings.HasPrefix(body, "([") && strings.HasSuffix(body, "])"):
		return NodeKindCheckpoint, strings.TrimSuffix(strings.TrimPrefix(body, "(["), "])")
	case strings.HasPrefix(body, "{") && strings.HasSuffix(body, "}"):
		return NodeKindGate, strings.TrimSuffix(strings.TrimPrefix(body, "{"), "}")
	case strings.HasPrefix(body, "[") && strings.HasSuffix(body, "]"):
		return NodeKindWorker, strings.TrimSuffix(strings.TrimPrefix(body, "["), "]")
	case strings.HasPrefix(body, "(") && strings.HasSuffix(body, ")"):
		return NodeKindWorker, strings.TrimSuffix(strings.TrimPrefix(body, "("), ")")
	default:
		return NodeKindWorker, body
	}
}

// parseLabel extracts the tool (first line) and arguments (remaining lines) from
// a node label, ignoring the «kind» and → outputs annotations ToMermaid adds.
func parseLabel(label string) (tool, args string) {
	label = stripQuotes(htmlUnescape(strings.TrimSpace(label)))
	segs := reLabelSep.Split(label, -1)
	var argParts []string
	for i, s := range segs {
		s = strings.TrimSpace(s)
		if i == 0 {
			tool = s
			continue
		}
		if s == "" || strings.HasPrefix(s, "«") || strings.HasPrefix(s, "→") {
			continue
		}
		argParts = append(argParts, s)
	}
	return tool, strings.Join(argParts, " ")
}

/*──────────────────────── edge parsing ─────────────────────────*/

func (p *mermaidParser) parseEdgeLine(line, subgraph string) {
	if m := rePipeLabel.FindStringSubmatch(line); m != nil {
		p.addEdge(m[1], m[3], m[2], subgraph)
		return
	}
	if m := reDotQuoted.FindStringSubmatch(line); m != nil {
		p.addEdge(m[1], m[3], m[2], subgraph)
		return
	}
	if m := reDotUnquoted.FindStringSubmatch(line); m != nil {
		p.addEdge(m[1], m[3], m[2], subgraph)
		return
	}
	tokens := rePlainSplit.Split(line, -1)
	for i := 0; i+1 < len(tokens); i++ {
		p.addEdge(tokens[i], tokens[i+1], "", subgraph)
	}
}

func (p *mermaidParser) addEdge(leftTok, rightTok, rawLabel, subgraph string) {
	from := p.registerNode(strings.TrimSpace(leftTok), subgraph)
	to := p.registerNode(strings.TrimSpace(rightTok), subgraph)
	if from == "" || to == "" || from == to {
		return
	}
	if to == p.g.Root { // edges into the source are meaningless
		return
	}
	label := htmlUnescape(strings.TrimSpace(rawLabel))
	condition := ""
	if looksLikeCondition(label) {
		condition = label
	}
	_ = p.g.AddEdge(from, to, condition, label)
}

// looksLikeCondition reports whether an edge label is a runtime condition
// expression rather than a cosmetic label. Only genuine conditions become
// Edge.Condition; everything else stays a plain Label so RunDAG does not gate
// (and skip) downstream nodes on a decorative word like "sequential".
func looksLikeCondition(label string) bool {
	label = strings.TrimSpace(label)
	if label == "" {
		return false
	}
	atoms := strings.Split(strings.ReplaceAll(label, "&&", "||"), "||")
	for _, atom := range atoms {
		a := strings.ToLower(strings.TrimSpace(atom))
		switch {
		case a == "always", a == "nonempty":
		case strings.HasPrefix(a, "approved:"), strings.HasPrefix(a, "has_type:"), strings.HasPrefix(a, "contains:"):
		default:
			return false
		}
	}
	return true
}

/*──────────────────────── layout post-processing ─────────────────────────*/

// attachOrphansToRoot connects every non-root node with no incoming edge to the
// implicit root, so RunDAG seeds it with the target domain and previews show it
// flowing from the start node.
func (p *mermaidParser) attachOrphansToRoot() {
	incoming := make(map[string]bool)
	for _, e := range p.g.Edges {
		incoming[e.To] = true
	}
	ids := make([]string, 0, len(p.g.Nodes))
	for id := range p.g.Nodes {
		if id != p.g.Root && !incoming[id] {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		_ = p.g.AddEdge(p.g.Root, id, "", "")
	}
}

// assignLayers derives each node's matrix position from its longest dependency
// path to the root (deterministic and cycle-safe), then places one node per
// coordinate so ValidateMatrix always passes.
func (p *mermaidParser) assignLayers() {
	memo := make(map[string]int)
	visiting := make(map[string]bool)
	var depth func(string) int
	depth = func(id string) int {
		if v, ok := memo[id]; ok {
			return v
		}
		if visiting[id] { // back-edge: contribute 0 rather than recurse (cycle-safe)
			return 0
		}
		visiting[id] = true
		best := 0
		for _, parent := range p.g.Parents(id) {
			if d := depth(parent) + 1; d > best {
				best = d
			}
		}
		visiting[id] = false
		memo[id] = best
		return best
	}

	byLayer := make(map[int][]string)
	for id := range p.g.Nodes {
		if id == p.g.Root {
			continue
		}
		layer := depth(id)
		p.g.Nodes[id].Layer = layer
		byLayer[layer] = append(byLayer[layer], id)
	}
	for layer, ids := range byLayer {
		sort.Strings(ids)
		for pos, id := range ids {
			n := p.g.Nodes[id]
			n.Position = pos
			p.g.addToMatrix(n)
			p.g.updateBounds(layer, pos)
		}
	}
}

/*──────────────────────── small helpers ─────────────────────────*/

func stripQuotes(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

func htmlUnescape(s string) string {
	s = strings.ReplaceAll(s, "&quot;", `"`)
	s = strings.ReplaceAll(s, "&#39;", "'")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&amp;", "&") // must be last
	return s
}
