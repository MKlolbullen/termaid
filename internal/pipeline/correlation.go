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
// reporting. De-duplication therefore never destroys source evidence.
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

// persistMergeCorrelation correlates directly from a merge node's raw parent
// files before the normalized merge artifact moves downstream. The sidecar is
// linked from NodeOutput metadata; raw parent files remain untouched.
func persistMergeCorrelation(df *DataFlow, output *NodeOutput) error {
	if df == nil || output == nil {
		return fmt.Errorf("cannot correlate nil merge output")
	}
	var records []DataRecord
	for _, file := range uniqueSortedStrings(df.GlobalState.DataLinks[output.NodeID]) {
		source := sourceNodeForArtifact(df, file)
		if source == "" {
			source = filepath.Base(file)
		}
		parsed, err := df.parseFile(file, source)
		if err != nil {
			continue
		}
		records = append(records, parsed...)
	}
	groups := CorrelateGroups(records)
	payload := struct {
		NodeID      string             `json:"node_id"`
		GeneratedAt time.Time          `json:"generated_at"`
		Groups      []CorrelationGroup `json:"groups"`
	}{NodeID: output.NodeID, GeneratedAt: time.Now().UTC(), Groups: groups}

	path := filepath.Join(df.WorkDir, df.RunID, "analysis", fmt.Sprintf("%s-correlation.json", output.NodeID))
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
// and verification evidence remain separate lifecycle stages while every group
// retains all raw observations.
type CorrelationReport struct {
	Version     string             `json:"version"`
	GeneratedAt time.Time          `json:"generated_at"`
	Candidates  []CorrelationGroup `json:"candidates"`
	Evidence    []CorrelationGroup `json:"evidence"`
}

// persistCorrelationReport is called only after upstream merge nodes have been
// decorated with their declared output type, so candidate and evidence
// snapshots can be classified without guessing from filenames.
func persistCorrelationReport(df *DataFlow, sinkOutput *NodeOutput) error {
	if df == nil || sinkOutput == nil {
		return fmt.Errorf("cannot create correlation report for nil sink")
	}
	var candidateRecords []DataRecord
	var evidenceRecords []DataRecord

	for nodeID, output := range df.NodeOutputs {
		if output == nil || nodeID == sinkOutput.NodeID {
			continue
		}
		path := output.Metadata["correlation_file"]
		if path == "" {
			continue
		}
		groups, err := readCorrelationGroups(path)
		if err != nil {
			continue
		}
		for _, group := range groups {
			switch {
			case metadataHasArtifact(output.Metadata, "finding"):
				candidateRecords = append(candidateRecords, group.Observations...)
			case metadataHasArtifact(output.Metadata, "evidence"):
				evidenceRecords = append(evidenceRecords, group.Observations...)
			}
		}
	}

	// A legacy/untyped workflow may not have an evidence-typed merge. Preserve a
	// useful report by correlating the sink's actual input as a fallback.
	if len(evidenceRecords) == 0 {
		for _, file := range df.GlobalState.DataLinks[sinkOutput.NodeID] {
			source := sourceNodeForArtifact(df, file)
			parsed, err := df.parseFile(file, source)
			if err == nil {
				evidenceRecords = append(evidenceRecords, parsed...)
			}
		}
	}

	report := CorrelationReport{
		Version: "1.0", GeneratedAt: time.Now().UTC(),
		Candidates: CorrelateGroups(candidateRecords),
		Evidence:   CorrelateGroups(evidenceRecords),
	}
	path := filepath.Join(df.WorkDir, df.RunID, "analysis", fmt.Sprintf("%s-correlation-report.json", sinkOutput.NodeID))
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	if sinkOutput.Metadata == nil {
		sinkOutput.Metadata = make(map[string]string)
	}
	sinkOutput.Metadata["correlation_report_file"] = path
	sinkOutput.OutputFiles = append(sinkOutput.OutputFiles, path)
	return nil
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

func metadataHasArtifact(metadata map[string]string, want string) bool {
	for _, item := range strings.Split(metadata["outputs"], ",") {
		if strings.EqualFold(strings.TrimSpace(item), want) {
			return true
		}
	}
	return false
}
