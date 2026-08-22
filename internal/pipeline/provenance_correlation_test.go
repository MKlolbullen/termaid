package pipeline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCorrelateGroupsPreservesEveryObservation(t *testing.T) {
	records := []DataRecord{
		{Value: "scanner A observation", Source: "scanner-a", Confidence: 0.7, Metadata: map[string]string{"asset": "example.com", "location": "/admin", "weakness": "exposure", "evidence": "200"}},
		{Value: "scanner B richer observation", Source: "scanner-b", Confidence: 0.9, Metadata: map[string]string{"asset": "example.com", "location": "/admin", "weakness": "exposure", "evidence": "200"}},
	}
	groups := CorrelateGroups(records)
	if len(groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(groups))
	}
	if len(groups[0].Observations) != 2 {
		t.Fatalf("observations = %d, want 2", len(groups[0].Observations))
	}
	if len(groups[0].Sources) != 2 {
		t.Fatalf("sources = %v, want both scanners", groups[0].Sources)
	}
	if groups[0].Representative.Source != "scanner-b" {
		t.Fatalf("representative source = %q, want scanner-b", groups[0].Representative.Source)
	}
}

func TestRecordedMergeKeepsRawProvenanceAndCorrelation(t *testing.T) {
	df, err := NewDataFlow(t.TempDir(), "example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := df.CreateSeedFile(); err != nil {
		t.Fatal(err)
	}

	a := filepath.Join(df.WorkDir, df.RunID, "raw", "scanner-a.txt")
	b := filepath.Join(df.WorkDir, df.RunID, "raw", "scanner-b.txt")
	if err := os.WriteFile(a, []byte("same finding\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("same finding\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := df.RecordNodeOutput("scanner-a", "scanner-a", now, now, 0, []string{a}, ""); err != nil {
		t.Fatal(err)
	}
	df.NodeOutputs["scanner-a"].Metadata["outputs"] = "finding"
	if err := df.RecordNodeOutput("scanner-b", "scanner-b", now, now, 0, []string{b}, ""); err != nil {
		t.Fatal(err)
	}
	df.NodeOutputs["scanner-b"].Metadata["outputs"] = "finding"

	merged := filepath.Join(df.WorkDir, df.RunID, "processed", "finding-merge.txt")
	if err := os.WriteFile(merged, []byte("same finding\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	df.GlobalState.DataLinks["finding-merge"] = []string{a, b}
	if err := df.RecordNodeOutput("finding-merge", "builtin:merge", now, now, 0, []string{merged}, ""); err != nil {
		t.Fatal(err)
	}
	df.NodeOutputs["finding-merge"].Metadata["outputs"] = "finding"

	correlationPath := df.NodeOutputs["finding-merge"].Metadata["correlation_file"]
	if correlationPath == "" {
		t.Fatal("finding merge did not produce a correlation sidecar")
	}
	groups, err := readCorrelationGroups(correlationPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || len(groups[0].Observations) != 2 {
		t.Fatalf("correlation groups = %#v, want one group with two raw observations", groups)
	}
	if !strings.Contains(strings.Join(groups[0].Sources, ","), "scanner-a") || !strings.Contains(strings.Join(groups[0].Sources, ","), "scanner-b") {
		t.Fatalf("correlation sources lost: %v", groups[0].Sources)
	}

	provenancePath := df.NodeOutputs["finding-merge"].Metadata["provenance_file"]
	data, err := os.ReadFile(provenancePath)
	if err != nil {
		t.Fatal(err)
	}
	var prov NodeProvenance
	if err := json.Unmarshal(data, &prov); err != nil {
		t.Fatal(err)
	}
	if len(prov.InputArtifacts) != 2 {
		t.Fatalf("provenance inputs = %d, want 2", len(prov.InputArtifacts))
	}
	for _, ref := range prov.InputArtifacts {
		if ref.SHA256 == "" || ref.SourceNode == "" {
			t.Fatalf("incomplete provenance ref: %#v", ref)
		}
	}
}

func TestSinkReportSeparatesCandidatesAndEvidence(t *testing.T) {
	df, err := NewDataFlow(t.TempDir(), "example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := df.CreateSeedFile(); err != nil {
		t.Fatal(err)
	}
	now := time.Now()

	candidateA := writeTestArtifact(t, df, "candidate-a.txt", "candidate issue\n")
	candidateB := writeTestArtifact(t, df, "candidate-b.txt", "candidate issue\n")
	recordTypedOutput(t, df, "candidate-a", "scanner-a", candidateA, "finding", now)
	recordTypedOutput(t, df, "candidate-b", "scanner-b", candidateB, "finding", now)
	candidateMerge := writeTestArtifact(t, df, "candidate-merge.txt", "candidate issue\n")
	df.GlobalState.DataLinks["finding-merge"] = []string{candidateA, candidateB}
	if err := df.RecordNodeOutput("finding-merge", "builtin:merge", now, now, 0, []string{candidateMerge}, ""); err != nil {
		t.Fatal(err)
	}
	df.NodeOutputs["finding-merge"].Metadata["outputs"] = "finding"

	evidenceA := writeTestArtifact(t, df, "evidence-a.txt", "verified issue\n")
	evidenceB := writeTestArtifact(t, df, "evidence-b.txt", "verified issue\n")
	recordTypedOutput(t, df, "evidence-a", "verify-a", evidenceA, "evidence", now)
	recordTypedOutput(t, df, "evidence-b", "verify-b", evidenceB, "evidence", now)
	evidenceMerge := writeTestArtifact(t, df, "evidence-merge.txt", "verified issue\n")
	df.GlobalState.DataLinks["evidence-merge"] = []string{evidenceA, evidenceB}
	if err := df.RecordNodeOutput("evidence-merge", "builtin:merge", now, now, 0, []string{evidenceMerge}, ""); err != nil {
		t.Fatal(err)
	}
	df.NodeOutputs["evidence-merge"].Metadata["outputs"] = "evidence"

	sinkFile := writeTestArtifact(t, df, "report-sink.txt", "verified issue\n")
	df.GlobalState.DataLinks["report-sink"] = []string{evidenceMerge}
	if err := df.RecordNodeOutput("report-sink", "builtin:sink", now, now, 0, []string{sinkFile}, ""); err != nil {
		t.Fatal(err)
	}
	reportPath := df.NodeOutputs["report-sink"].Metadata["correlation_report_file"]
	if reportPath == "" {
		t.Fatal("sink did not produce correlation report")
	}
	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	var report CorrelationReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Candidates) != 1 || len(report.Candidates[0].Observations) != 2 {
		t.Fatalf("candidate correlation lost observations: %#v", report.Candidates)
	}
	if len(report.Evidence) != 1 || len(report.Evidence[0].Observations) != 2 {
		t.Fatalf("evidence correlation lost observations: %#v", report.Evidence)
	}
	if df.NodeOutputs["report-sink"].Metadata["correlation_report_sha256"] == "" {
		t.Fatal("correlation report hash missing")
	}
}

func writeTestArtifact(t *testing.T, df *DataFlow, name, content string) string {
	t.Helper()
	path := filepath.Join(df.WorkDir, df.RunID, "raw", name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func recordTypedOutput(t *testing.T, df *DataFlow, nodeID, tool, path, outputType string, now time.Time) {
	t.Helper()
	if err := df.RecordNodeOutput(nodeID, tool, now, now, 0, []string{path}, ""); err != nil {
		t.Fatal(err)
	}
	df.NodeOutputs[nodeID].Metadata["outputs"] = outputType
}
