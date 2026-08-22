package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/MKlolbullen/termaid/internal/graph"
)

// CorrelationKey groups tool observations that describe the same underlying
// issue. Structured metadata wins; raw-value hashing is a safe fallback for
// legacy/unstructured tool output.
func CorrelationKey(record DataRecord) string {
	parts := []string{
		strings.ToLower(strings.TrimSpace(record.Metadata["asset"])),
		strings.ToLower(strings.TrimSpace(record.Metadata["location"])),
		strings.ToLower(strings.TrimSpace(record.Metadata["weakness"])),
		strings.ToLower(strings.TrimSpace(record.Metadata["evidence"])),
	}
	structured := false
	for _, p := range parts {
		if p != "" {
			structured = true
			break
		}
	}
	if !structured {
		parts = []string{strings.ToLower(strings.TrimSpace(record.Value))}
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:16])
}

// CorrelationGroup preserves every raw observation contributing to one logical
// finding/evidence item while still selecting a representative record for
// reporting. This prevents de-duplication from destroying source evidence.
type CorrelationGroup struct {
	Key            string       `json:"key"`
	Representative DataRecord   `json:"representative"`
	Sources        []string     `json:"sources"`
	Observations   []DataRecord `json:"observations"`
}

// CorrelateGroups returns stable correlation groups with complete observations.
func CorrelateGroups(records []DataRecord) []CorrelationGroup {
	type mutableGroup struct {
		best         DataRecord
		observations []DataRecord
		sources      map[string]struct{}
	}
	groups := make(map[string]*mutableGroup)
	for _, record := range records {
		if record.Metadata == nil {
			record.Metadata = make(map[string]string)
		}
		key := CorrelationKey(record)
		g, ok := groups[key]
		if !ok {
			g = &mutableGroup{best: cloneRecord(record), sources: make(map[string]struct{})}
			groups[key] = g
		}
		g.observations = append(g.observations, cloneRecord(record))
		if record.Source != "" {
			g.sources[record.Source] = struct{}{}
		}
		if record.Confidence > g.best.Confidence {
			g.best = cloneRecord(record)
		}
	}

	out := make([]CorrelationGroup, 0, len(groups))
	for key, g := range groups {
		var sources []string
		for source := range g.sources {
			sources = append(sources, source)
		}
		sort.Strings(sources)
		sort.SliceStable(g.observations, func(i, j int) bool {
			if g.observations[i].Source != g.observations[j].Source {
				return g.observations[i].Source < g.observations[j].Source
			}
			return g.observations[i].Value < g.observations[j].Value
		})
		if g.best.Metadata == nil {
			g.best.Metadata = make(map[string]string)
		}
		g.best.Metadata["correlation_key"] = key
		g.best.Metadata["sources"] = strings.Join(sources, ",")
		g.best.Metadata["observation_count"] = strconv.Itoa(len(g.observations))
		out = append(out, CorrelationGroup{
			Key: key, Representative: g.best, Sources: sources, Observations: g.observations,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// CorrelateRecords is the compact compatibility view used by existing callers.
func CorrelateRecords(records []DataRecord) []DataRecord {
	groups := CorrelateGroups(records)
	out := make([]DataRecord, 0, len(groups))
	for _, group := range groups {
		out = append(out, group.Representative)
	}
	return out
}

func cloneRecord(record DataRecord) DataRecord {
	copyRecord := record
	if record.Metadata != nil {
		copyRecord.Metadata = make(map[string]string, len(record.Metadata))
		for k, v := range record.Metadata {
			copyRecord.Metadata[k] = v
		}
	}
	return copyRecord
}

// writeCorrelationSnapshot correlates a merge node from its raw parent outputs,
// not from the already-normalized merged file. That retains source identity and
// gives report generation a candidate/evidence snapshot created before the sink.
func writeCorrelationSnapshot(df *DataFlow, node *graph.Node, parents []string) error {
	output := df.NodeOutputs[node.ID]
	if output == nil {
		return fmt.Errorf("node %s has no recorded output for correlation", node.ID)
	}
	var records []DataRecord
	for _, parentID := range parents {
		for _, file := range outputFiles(df, parentID) {
			parsed, err := df.parseFile(file, parentID)
			if err != nil {
				continue
			}
			records = append(records, parsed...)
		}
	}
	groups := CorrelateGroups(records)
	payload := struct {
		NodeID      string             `json:"node_id"`
		Artifact    string             `json:"artifact"`
		GeneratedAt time.Time          `json:"generated_at"`
		Groups      []CorrelationGroup `json:"groups"`
	}{
		NodeID: node.ID, Artifact: artifactTypes(node.Outputs), GeneratedAt: time.Now().UTC(), Groups: groups,
	}
	path := filepath.Join(df.WorkDir, df.RunID, "analysis", fmt.Sprintf("%s-correlation.json", node.ID))
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	if output.Metadata == nil {
		output.Metadata = make(map[string]string)
	}
	output.Metadata["correlation_file"] = path
	output.Metadata["correlation_groups"] = strconv.Itoa(len(groups))
	return nil
}

// CorrelationReport is the report sink's structured output. Candidate findings
// and verification evidence stay separate lifecycle stages while each retains
// its raw observations.
type CorrelationReport struct {
	Version     string             `json:"version"`
	GeneratedAt time.Time          `json:"generated_at"`
	Candidates  []CorrelationGroup `json:"candidates"`
	Evidence    []CorrelationGroup `json:"evidence"`
}

func writeCorrelationReport(df *DataFlow, node *graph.Node, fallbackInput string) (string, error) {
	var candidateRecords []DataRecord
	var evidenceRecords []DataRecord
	for _, output := range df.NodeOutputs {
		path := output.Metadata["correlation_file"]
		if path == "" {
			continue
		}
		groups, err := readCorrelationGroups(path)
		if err != nil {
			continue
		}
		for _, group := range groups {
			if strings.Contains(output.Metadata["outputs"], string(graph.ArtifactFinding)) {
				candidateRecords = append(candidateRecords, group.Observations...)
			}
			if strings.Contains(output.Metadata["outputs"], string(graph.ArtifactEvidence)) {
				evidenceRecords = append(evidenceRecords, group.Observations...)
			}
		}
	}
	if len(evidenceRecords) == 0 && fallbackInput != "" {
		records, err := df.parseFile(fallbackInput, node.ID)
		if err == nil {
			evidenceRecords = append(evidenceRecords, records...)
		}
	}

	report := CorrelationReport{
		Version: "1.0", GeneratedAt: time.Now().UTC(),
		Candidates: CorrelateGroups(candidateRecords), Evidence: CorrelateGroups(evidenceRecords),
	}
	path := filepath.Join(df.WorkDir, df.RunID, "analysis", fmt.Sprintf("%s-correlated-report.json", node.ID))
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	return path, os.WriteFile(path, data, 0o600)
}

func readCorrelationGroups(path string) ([]CorrelationGroup, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Groups []CorrelationGroup `json:"groups"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	return payload.Groups, nil
}
