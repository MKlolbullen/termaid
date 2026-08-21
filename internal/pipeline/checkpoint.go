package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const checkpointFileName = "checkpoint.json"

// RunCheckpoint is the durable state required to resume a DAG run without
// rerunning completed nodes.
type RunCheckpoint struct {
	Version     string                 `json:"version"`
	SavedAt     time.Time              `json:"saved_at"`
	GlobalState *GlobalState           `json:"global_state"`
	NodeOutputs map[string]*NodeOutput `json:"node_outputs"`
}

// SaveCheckpoint atomically persists scheduler state after each completed
// execution wave. A temporary file is renamed into place to avoid partial JSON
// after cancellation or power loss.
func (df *DataFlow) SaveCheckpoint() error {
	if df == nil || df.GlobalState == nil {
		return fmt.Errorf("nil data flow")
	}
	payload := RunCheckpoint{
		Version:     "1.0",
		SavedAt:     time.Now().UTC(),
		GlobalState: df.GlobalState,
		NodeOutputs: df.NodeOutputs,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal checkpoint: %w", err)
	}
	runDir := filepath.Join(df.WorkDir, df.RunID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return err
	}
	tmp := filepath.Join(runDir, checkpointFileName+".tmp")
	final := filepath.Join(runDir, checkpointFileName)
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, final); err != nil {
		return err
	}
	return nil
}

// ResumeDataFlow restores a prior run. Completed node outputs are retained only
// when their referenced files still exist; missing artifacts are demoted to
// pending so the scheduler can reproduce them.
func ResumeDataFlow(workDir, runID, domain string) (*DataFlow, error) {
	if runID == "" {
		return nil, fmt.Errorf("resume run id is empty")
	}
	path := filepath.Join(workDir, runID, checkpointFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read checkpoint %s: %w", path, err)
	}
	var cp RunCheckpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, fmt.Errorf("decode checkpoint: %w", err)
	}
	if cp.GlobalState == nil {
		return nil, fmt.Errorf("checkpoint has no global state")
	}
	if domain != "" && cp.GlobalState.Domain != "" && domain != cp.GlobalState.Domain {
		return nil, fmt.Errorf("checkpoint target %q does not match requested target %q", cp.GlobalState.Domain, domain)
	}
	if cp.NodeOutputs == nil {
		cp.NodeOutputs = make(map[string]*NodeOutput)
	}
	if cp.GlobalState.NodeStates == nil {
		cp.GlobalState.NodeStates = make(map[string]NodeStatus)
	}
	if cp.GlobalState.DataLinks == nil {
		cp.GlobalState.DataLinks = make(map[string][]string)
	}
	if cp.GlobalState.Statistics == nil {
		cp.GlobalState.Statistics = &ExecutionStatistics{}
	}

	df := &DataFlow{WorkDir: workDir, RunID: runID, NodeOutputs: cp.NodeOutputs, GlobalState: cp.GlobalState}
	for nodeID, output := range df.NodeOutputs {
		if df.GlobalState.NodeStates[nodeID] != NodeCompleted {
			continue
		}
		valid := len(output.OutputFiles) > 0
		for _, file := range output.OutputFiles {
			if _, err := os.Stat(file); err != nil {
				valid = false
				break
			}
		}
		if !valid {
			df.GlobalState.NodeStates[nodeID] = NodePending
			delete(df.NodeOutputs, nodeID)
		}
	}
	return df, nil
}

// RecordSkipped records a policy/condition decision without pretending that a
// tool failed. Downstream scheduling can then distinguish deliberate pruning
// from an execution error.
func (df *DataFlow) RecordSkipped(nodeID, reason string) {
	if df.GlobalState.NodeStates == nil {
		df.GlobalState.NodeStates = make(map[string]NodeStatus)
	}
	df.GlobalState.NodeStates[nodeID] = NodeSkipped
	df.NodeOutputs[nodeID] = &NodeOutput{
		NodeID:    nodeID,
		Tool:      "builtin:skip",
		StartTime: time.Now(),
		EndTime:   time.Now(),
		ExitCode:  0,
		Metadata:  map[string]string{"skip_reason": reason},
	}
}
