package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/MKlolbullen/termaid/internal/graph"
)

// ArtifactRef is an immutable description of a file that participated in a
// workflow step. Hashes make later evidence/reporting code able to prove which
// exact bytes were consumed or produced without copying the raw artifact.
type ArtifactRef struct {
	Path      string `json:"path"`
	Role      string `json:"role"`
	SHA256    string `json:"sha256,omitempty"`
	Size      int64  `json:"size"`
	LineCount int    `json:"line_count"`
	Format    string `json:"format,omitempty"`
}

// ControlDecision records why a control-only dependency allowed or pruned a
// branch. Control edges affect dispatch but never become data input.
type ControlDecision struct {
	From        string     `json:"from"`
	To          string     `json:"to"`
	Condition   string     `json:"condition,omitempty"`
	Label       string     `json:"label,omitempty"`
	Matched     bool       `json:"matched"`
	ParentState NodeStatus `json:"parent_state"`
}

// ApprovalDecision records the authorization state used when dispatching a
// node. This is deliberately persisted with the run rather than inferred later.
type ApprovalDecision struct {
	Key                string `json:"key,omitempty"`
	RequiresApproval   bool   `json:"requires_approval"`
	Intrusive          bool   `json:"intrusive"`
	Approved           bool   `json:"approved"`
	BroadIntrusiveMode bool   `json:"broad_intrusive_mode"`
}

// NodeProvenance links a node to exact input/output artifacts, data parents,
// control-edge decisions, and approval state.
type NodeProvenance struct {
	RecordedAt       time.Time         `json:"recorded_at"`
	Parents          []string          `json:"parents,omitempty"`
	DataParents      []string          `json:"data_parents,omitempty"`
	InputArtifacts   []ArtifactRef     `json:"input_artifacts,omitempty"`
	PreparedInput    *ArtifactRef      `json:"prepared_input,omitempty"`
	OutputArtifacts  []ArtifactRef     `json:"output_artifacts,omitempty"`
	ControlDecisions []ControlDecision `json:"control_decisions,omitempty"`
	Approval         ApprovalDecision  `json:"approval"`
}

func attachNodeProvenance(df *DataFlow, node *graph.Node, dag *graph.DAG, cfg RunConfig, dataParents []string, preparedInput string) error {
	if df == nil || node == nil || dag == nil {
		return fmt.Errorf("cannot attach provenance to nil runtime object")
	}
	output := df.NodeOutputs[node.ID]
	if output == nil {
		return fmt.Errorf("node %s has no recorded output for provenance", node.ID)
	}

	parents := dag.Parents(node.ID)
	inputPaths := append([]string(nil), df.GlobalState.DataLinks[node.ID]...)
	if len(inputPaths) == 0 {
		for _, parentID := range dataParents {
			inputPaths = append(inputPaths, outputFiles(df, parentID)...)
		}
	}
	inputPaths = uniqueSortedStrings(inputPaths)

	prov := &NodeProvenance{
		RecordedAt:  time.Now().UTC(),
		Parents:     parents,
		DataParents: append([]string(nil), dataParents...),
		Approval:    approvalDecision(node, cfg),
	}
	sort.Strings(prov.DataParents)

	for _, path := range inputPaths {
		ref, err := artifactRef(df, path, "raw-input")
		if err != nil {
			continue
		}
		prov.InputArtifacts = append(prov.InputArtifacts, ref)
	}
	if strings.TrimSpace(preparedInput) != "" {
		if ref, err := artifactRef(df, preparedInput, "prepared-input"); err == nil {
			prov.PreparedInput = &ref
		}
	}
	for _, path := range output.OutputFiles {
		ref, err := artifactRef(df, path, "output")
		if err != nil {
			continue
		}
		prov.OutputArtifacts = append(prov.OutputArtifacts, ref)
	}
	for _, edge := range dag.IncomingEdges(node.ID) {
		if !edge.Control {
			continue
		}
		state := df.GlobalState.NodeStates[edge.From]
		matched := false
		if state == NodeCompleted {
			matched = conditionMatches(edge.Condition, outputFiles(df, edge.From), df, cfg)
		}
		prov.ControlDecisions = append(prov.ControlDecisions, ControlDecision{
			From: edge.From, To: edge.To, Condition: edge.Condition, Label: edge.Label,
			Matched: matched, ParentState: state,
		})
	}
	sort.Slice(prov.ControlDecisions, func(i, j int) bool {
		return prov.ControlDecisions[i].From < prov.ControlDecisions[j].From
	})

	path := filepath.Join(df.WorkDir, df.RunID, "analysis", fmt.Sprintf("%s-provenance.json", node.ID))
	data, err := json.MarshalIndent(prov, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal provenance for %s: %w", node.ID, err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write provenance for %s: %w", node.ID, err)
	}
	if output.Metadata == nil {
		output.Metadata = make(map[string]string)
	}
	output.Metadata["provenance_file"] = path
	output.Metadata["provenance_recorded_at"] = prov.RecordedAt.Format(time.RFC3339Nano)
	return nil
}

func approvalDecision(node *graph.Node, cfg RunConfig) ApprovalDecision {
	key := strings.TrimSpace(node.Policy.Approval)
	if key == "" && node.Policy.RequiresApproval {
		key = node.ID
	}
	approved := false
	if key != "" {
		approved = cfg.Approvals[key] || cfg.Approvals[node.ID]
	}
	if node.Policy.Intrusive && (cfg.AllowIntrusive || cfg.Approvals["intrusive"]) {
		approved = true
	}
	return ApprovalDecision{
		Key: key, RequiresApproval: node.Policy.RequiresApproval, Intrusive: node.Policy.Intrusive,
		Approved: approved, BroadIntrusiveMode: cfg.AllowIntrusive || cfg.Approvals["intrusive"],
	}
}

func artifactRef(df *DataFlow, path, role string) (ArtifactRef, error) {
	stat, err := os.Stat(path)
	if err != nil {
		return ArtifactRef{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return ArtifactRef{}, err
	}
	defer file.Close()
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return ArtifactRef{}, err
	}
	lines, _ := df.countLines(path)
	return ArtifactRef{
		Path: path, Role: role, SHA256: hex.EncodeToString(h.Sum(nil)), Size: stat.Size(),
		LineCount: lines, Format: df.detectFormat(path),
	}, nil
}

func uniqueSortedStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}
