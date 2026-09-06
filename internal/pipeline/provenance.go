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
	"time"
)

// ArtifactRef is an immutable description of a file that participated in a
// workflow step. Hashes let later evidence/reporting code prove which exact
// bytes were consumed or produced without replacing the original artifact.
type ArtifactRef struct {
	Path       string `json:"path"`
	Role       string `json:"role"`
	SourceNode string `json:"source_node,omitempty"`
	SHA256     string `json:"sha256,omitempty"`
	Size       int64  `json:"size"`
	LineCount  int    `json:"line_count"`
	Format     string `json:"format,omitempty"`
}

// NodeProvenance links one execution result to its exact file inputs and
// outputs. Graph-level parent/control/approval metadata remains in NodeOutput
// metadata and the checkpoint; this file preserves the byte-level evidence.
type NodeProvenance struct {
	NodeID          string        `json:"node_id"`
	Tool            string        `json:"tool"`
	RecordedAt      time.Time     `json:"recorded_at"`
	InputArtifacts  []ArtifactRef `json:"input_artifacts,omitempty"`
	OutputArtifacts []ArtifactRef `json:"output_artifacts,omitempty"`
}

// persistNodeProvenance writes a sidecar under analysis/ and links it from the
// NodeOutput metadata. Raw files themselves are never rewritten or deleted.
func persistNodeProvenance(df *DataFlow, output *NodeOutput) error {
	if df == nil || output == nil {
		return fmt.Errorf("cannot persist provenance for nil output")
	}
	prov := NodeProvenance{
		NodeID: output.NodeID, Tool: output.Tool, RecordedAt: time.Now().UTC(),
	}

	inputPaths := append([]string(nil), df.GlobalState.DataLinks[output.NodeID]...)
	// Root-level nodes receive the seed directly and historically had no
	// DataLinks entry. Preserve that seed relationship explicitly.
	if len(inputPaths) == 0 && output.NodeID != "seed" {
		if seed := df.NodeOutputs["seed"]; seed != nil {
			inputPaths = append(inputPaths, seed.OutputFiles...)
		}
	}
	inputPaths = uniqueSortedStrings(inputPaths)
	for _, path := range inputPaths {
		ref, err := artifactRef(df, path, "input")
		if err != nil {
			continue
		}
		ref.SourceNode = sourceNodeForArtifact(df, path)
		prov.InputArtifacts = append(prov.InputArtifacts, ref)
	}
	for _, path := range uniqueSortedStrings(output.OutputFiles) {
		ref, err := artifactRef(df, path, "output")
		if err != nil {
			continue
		}
		ref.SourceNode = output.NodeID
		prov.OutputArtifacts = append(prov.OutputArtifacts, ref)
	}

	path := filepath.Join(df.WorkDir, df.RunID, "analysis", fmt.Sprintf("%s-provenance.json", output.NodeID))
	data, err := json.MarshalIndent(prov, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal provenance for %s: %w", output.NodeID, err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write provenance for %s: %w", output.NodeID, err)
	}
	if output.Metadata == nil {
		output.Metadata = make(map[string]string)
	}
	output.Metadata["provenance_file"] = path
	output.Metadata["provenance_recorded_at"] = prov.RecordedAt.Format(time.RFC3339Nano)
	return nil
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

func sourceNodeForArtifact(df *DataFlow, path string) string {
	for nodeID, output := range df.NodeOutputs {
		if output == nil {
			continue
		}
		for _, candidate := range output.OutputFiles {
			if candidate == path {
				return nodeID
			}
		}
	}
	return ""
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
