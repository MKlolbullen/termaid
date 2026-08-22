package pipeline

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/MKlolbullen/termaid/internal/graph"
)

// Additional status emitted by the DAG scheduler for deliberate pruning.
const StatusSkip StatusUpdateType = 3

// RunConfig controls the dependency scheduler independently from tool flags.
type RunConfig struct {
	Concurrency    int
	ResumeRunID    string
	Approvals      map[string]bool
	AllowIntrusive bool
}

type workerResult struct {
	nodeID      string
	tool        string
	start       time.Time
	end         time.Time
	exitCode    int
	outputFiles []string
	stderr      string
	attempts    int
	err         error
}

type preparedWorker struct {
	node      *graph.Node
	inputPath string
	parents   []string
}

// RunDAG executes nodes when their actual dependencies are terminal. Visual
// layers are not execution barriers: independent branches can run concurrently,
// converge through explicit merge nodes, and be pruned by conditions/policy.
func RunDAG(ctx context.Context, domain, workdir string, dag *graph.DAG, cfg RunConfig, out chan<- Status) error {
	if dag == nil {
		return fmt.Errorf("nil DAG")
	}
	if err := dag.Validate(); err != nil {
		return fmt.Errorf("invalid workflow: %w", err)
	}
	if strings.TrimSpace(domain) == "" {
		return fmt.Errorf("target domain must not be empty")
	}
	if !targetAllowed(domain, dag.Policy) {
		return fmt.Errorf("target %q is outside workflow policy", domain)
	}

	concurrency := cfg.Concurrency
	if concurrency < 1 {
		concurrency = 1
	}
	if dag.Policy.MaxConcurrency > 0 && concurrency > dag.Policy.MaxConcurrency {
		concurrency = dag.Policy.MaxConcurrency
	}
	if cfg.Approvals == nil {
		cfg.Approvals = make(map[string]bool)
	}

	var df *DataFlow
	var err error
	if cfg.ResumeRunID != "" {
		df, err = ResumeDataFlow(workdir, cfg.ResumeRunID, domain)
	} else {
		df, err = NewDataFlow(workdir, domain)
	}
	if err != nil {
		return err
	}

	seedPath := ""
	if seed := df.NodeOutputs["seed"]; seed != nil && len(seed.OutputFiles) > 0 {
		seedPath = seed.OutputFiles[0]
	}
	if seedPath == "" {
		seedPath, err = df.CreateSeedFile()
		if err != nil {
			return err
		}
	}

	pending := make(map[string]struct{})
	for id := range dag.Nodes {
		if id == dag.Root {
			continue
		}
		state := df.GlobalState.NodeStates[id]
		if state != NodeCompleted && state != NodeSkipped {
			pending[id] = struct{}{}
		}
	}
	if err := df.SaveCheckpoint(); err != nil {
		return fmt.Errorf("initial checkpoint: %w", err)
	}

	for len(pending) > 0 {
		if err := ctx.Err(); err != nil {
			_ = df.SaveCheckpoint()
			return err
		}

		ready := readyNodes(dag, pending, df.GlobalState.NodeStates)
		if len(ready) == 0 {
			return fmt.Errorf("scheduler deadlock: %d node(s) pending with unresolved dependencies", len(pending))
		}

		var workers []preparedWorker
		progress := false
		for _, nodeID := range ready {
			node := dag.Nodes[nodeID]
			parents := dag.Parents(nodeID)

			if blockedByFailedParent(node, parents, df.GlobalState.NodeStates) {
				reason := "upstream failure"
				df.RecordSkipped(nodeID, reason)
				out <- Status{Type: StatusSkip, Category: categoryForNode(node), Tool: nodeID, Err: fmt.Errorf("%s", reason)}
				delete(pending, nodeID)
				progress = true
				continue
			}

			eligible, inputFiles, incomingOK := eligibleParents(dag, node, df, cfg)
			if onlyRootParents(parents, dag.Root) {
				inputFiles = []string{seedPath}
			}
			if !incomingOK {
				reason := "incoming edge/control condition evaluated false"
				df.RecordSkipped(nodeID, reason)
				out <- Status{Type: StatusSkip, Category: categoryForNode(node), Tool: nodeID, Err: fmt.Errorf("%s", reason)}
				delete(pending, nodeID)
				progress = true
				continue
			}
			if !conditionMatches(node.Condition, inputFiles, df, cfg) {
				reason := "node condition evaluated false"
				df.RecordSkipped(nodeID, reason)
				out <- Status{Type: StatusSkip, Category: categoryForNode(node), Tool: nodeID, Err: fmt.Errorf("%s", reason)}
				delete(pending, nodeID)
				progress = true
				continue
			}
			if ok, reason := policyAllows(node, dag.Policy, cfg); !ok {
				df.RecordSkipped(nodeID, reason)
				out <- Status{Type: StatusSkip, Category: categoryForNode(node), Tool: nodeID, Err: fmt.Errorf("%s", reason)}
				delete(pending, nodeID)
				progress = true
				continue
			}

			inputPath, prepErr := prepareInput(df, node, eligible, seedPath)
			if prepErr != nil {
				return fmt.Errorf("prepare %s input: %w", nodeID, prepErr)
			}

			switch node.EffectiveKind() {
			case graph.NodeKindWorker:
				df.GlobalState.NodeStates[nodeID] = NodeRunning
				workers = append(workers, preparedWorker{node: node, inputPath: inputPath, parents: eligible})
			case graph.NodeKindMerge, graph.NodeKindGate, graph.NodeKindTransform, graph.NodeKindCheckpoint, graph.NodeKindSink, graph.NodeKindManual:
				out <- Status{Type: StatusStart, Category: categoryForNode(node), Tool: nodeID}
				result := executeBuiltin(node, inputPath, domain, df, dag.Policy)
				if recErr := df.RecordNodeOutput(nodeID, result.tool, result.start, result.end, result.exitCode, result.outputFiles, result.stderr); recErr != nil {
					return fmt.Errorf("record %s output: %w", nodeID, recErr)
				}
				decorateOutput(df, node, eligible, result.attempts)
				if result.err != nil {
					out <- Status{Type: StatusError, Category: categoryForNode(node), Tool: nodeID, Err: result.err}
				} else {
					out <- Status{Type: StatusFinish, Category: categoryForNode(node), Tool: nodeID}
				}
				delete(pending, nodeID)
				progress = true
			default:
				return fmt.Errorf("unsupported node kind %q on %s", node.Kind, nodeID)
			}
		}

		if len(workers) > 0 {
			results := executeWorkers(ctx, workers, domain, filepath.Join(workdir, df.RunID, "raw"), concurrency, out)
			for _, result := range results {
				node := dag.Nodes[result.nodeID]
				if recErr := df.RecordNodeOutput(result.nodeID, result.tool, result.start, result.end, result.exitCode, result.outputFiles, result.stderr); recErr != nil {
					return fmt.Errorf("record %s output: %w", result.nodeID, recErr)
				}
				decorateOutput(df, node, dag.Parents(result.nodeID), result.attempts)
				delete(pending, result.nodeID)
				progress = true
			}
		}

		if !progress {
			return fmt.Errorf("scheduler made no progress")
		}
		if err := df.SaveCheckpoint(); err != nil {
			return fmt.Errorf("save checkpoint: %w", err)
		}
	}

	if err := df.CreateExecutionReport(); err != nil {
		return fmt.Errorf("create execution report: %w", err)
	}
	return df.SaveCheckpoint()
}

func readyNodes(dag *graph.DAG, pending map[string]struct{}, states map[string]NodeStatus) []string {
	var ready []string
	for id := range pending {
		parents := dag.Parents(id)
		terminal := true
		for _, parent := range parents {
			if parent == dag.Root {
				continue
			}
			switch states[parent] {
			case NodeCompleted, NodeFailed, NodeSkipped:
			default:
				terminal = false
			}
		}
		if terminal {
			ready = append(ready, id)
		}
	}
	sort.Slice(ready, func(i, j int) bool {
		a, b := dag.Nodes[ready[i]], dag.Nodes[ready[j]]
		if a.Layer != b.Layer {
			return a.Layer < b.Layer
		}
		if a.Position != b.Position {
			return a.Position < b.Position
		}
		return a.ID < b.ID
	})
	return ready
}

func blockedByFailedParent(node *graph.Node, parents []string, states map[string]NodeStatus) bool {
	if node.Policy.ContinueOnError {
		return false
	}
	for _, parent := range parents {
		if states[parent] == NodeFailed {
			return true
		}
	}
	return false
}

// eligibleParents evaluates all incoming edges. Data edges whose conditions
// match contribute artifacts; control edges gate dispatch but never pollute the
// child's input stream.
func eligibleParents(dag *graph.DAG, node *graph.Node, df *DataFlow, cfg RunConfig) ([]string, []string, bool) {
	var eligible []string
	var files []string
	dataEdges := 0
	dataMatched := 0
	controlOK := true

	for _, edge := range dag.IncomingEdges(node.ID) {
		if edge.From == dag.Root {
			continue
		}
		state := df.GlobalState.NodeStates[edge.From]
		if state != NodeCompleted {
			if edge.Control {
				controlOK = false
			}
			continue
		}
		parentFiles := outputFiles(df, edge.From)
		matched := conditionMatches(edge.Condition, parentFiles, df, cfg)
		if edge.Control {
			if !matched {
				controlOK = false
			}
			continue
		}
		dataEdges++
		if matched {
			dataMatched++
			if !containsID(eligible, edge.From) {
				eligible = append(eligible, edge.From)
				files = append(files, parentFiles...)
			}
		}
	}

	if dataEdges > 0 && dataMatched == 0 {
		return eligible, files, false
	}
	return eligible, files, controlOK
}

func prepareInput(df *DataFlow, node *graph.Node, parents []string, seedPath string) (string, error) {
	if len(parents) == 0 {
		return seedPath, nil
	}
	return df.PrepareNodeInput(node.ID, parents, node.Layer)
}

func executeWorkers(ctx context.Context, workers []preparedWorker, domain, rawDir string, concurrency int, out chan<- Status) []workerResult {
	sem := make(chan struct{}, concurrency)
	results := make(chan workerResult, len(workers))
	var wg sync.WaitGroup
	for _, prepared := range workers {
		p := prepared
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results <- workerResult{nodeID: p.node.ID, tool: p.node.Tool, start: time.Now(), end: time.Now(), exitCode: 1, err: ctx.Err(), stderr: ctx.Err().Error()}
				return
			}
			out <- Status{Type: StatusStart, Category: categoryForNode(p.node), Tool: p.node.ID}
			result := executeWorker(ctx, p.node, domain, p.inputPath, rawDir)
			if result.err != nil {
				out <- Status{Type: StatusError, Category: categoryForNode(p.node), Tool: p.node.ID, Err: result.err}
			} else {
				out <- Status{Type: StatusFinish, Category: categoryForNode(p.node), Tool: p.node.ID}
			}
			results <- result
		}()
	}
	wg.Wait()
	close(results)
	outResults := make([]workerResult, 0, len(workers))
	for result := range results {
		outResults = append(outResults, result)
	}
	sort.Slice(outResults, func(i, j int) bool { return outResults[i].nodeID < outResults[j].nodeID })
	return outResults
}

func executeWorker(ctx context.Context, node *graph.Node, domain, inputPath, rawDir string) workerResult {
	start := time.Now()
	result := workerResult{nodeID: node.ID, tool: node.Tool, start: start, exitCode: 1}
	catDir := filepath.Join(rawDir, dirSafe(categoryForNode(node)))
	if err := os.MkdirAll(catDir, 0o755); err != nil {
		result.end, result.err, result.stderr = time.Now(), err, err.Error()
		return result
	}
	outputFile := filepath.Join(catDir, fmt.Sprintf("%s-%d.txt", node.ID, start.UnixNano()))
	result.outputFiles = []string{outputFile}

	if _, err := exec.LookPath(node.Tool); err != nil {
		result.end, result.err, result.stderr = time.Now(), fmt.Errorf("command not found: %s", node.Tool), err.Error()
		return result
	}

	invocation, prepErr := prepareCommandString(node.Args, domain, inputPath, outputFile)
	if prepErr != nil {
		result.end, result.err, result.stderr = time.Now(), prepErr, prepErr.Error()
		return result
	}
	args := invocation.Args
	maxAttempts := node.Execution.Retries + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	var lastErr error
	var stderrText string
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		result.attempts = attempt
		attemptCtx := ctx
		cancel := func() {}
		if node.Execution.TimeoutSeconds > 0 {
			attemptCtx, cancel = context.WithTimeout(ctx, time.Duration(node.Execution.TimeoutSeconds)*time.Second)
		}

		var stderr bytes.Buffer
		cmd := exec.CommandContext(attemptCtx, node.Tool, args...)
		cmd.Dir = catDir
		cmd.Env = append(os.Environ(), "TERM=xterm-256color", "PYTHONUNBUFFERED=1", "FORCE_COLOR=0")
		cmd.Stderr = &stderr

		var outF *os.File
		if invocation.CaptureStdout {
			var err error
			outF, err = os.Create(outputFile)
			if err != nil {
				cancel()
				result.end, result.err, result.stderr = time.Now(), err, err.Error()
				return result
			}
			cmd.Stdout = outF
		}

		lastErr = cmd.Run()
		if outF != nil {
			_ = outF.Close()
		}
		cancel()
		stderrText = stderr.String()
		if lastErr == nil {
			result.exitCode = 0
			break
		}
		if attempt < maxAttempts {
			backoff := node.Execution.RetryBackoffMS
			if backoff <= 0 {
				backoff = 250
			}
			select {
			case <-time.After(time.Duration(backoff) * time.Millisecond):
			case <-ctx.Done():
				lastErr = ctx.Err()
				attempt = maxAttempts
			}
		}
	}
	result.end = time.Now()
	result.stderr = stderrText
	result.err = lastErr
	if exitErr, ok := lastErr.(*exec.ExitError); ok {
		result.exitCode = exitErr.ExitCode()
	}
	return result
}

func executeBuiltin(node *graph.Node, inputPath, domain string, df *DataFlow, policy graph.WorkflowPolicy) workerResult {
	start := time.Now()
	result := workerResult{nodeID: node.ID, tool: "builtin:" + string(node.EffectiveKind()), start: start, attempts: 1}
	var outputPath string
	var err error
	switch node.EffectiveKind() {
	case graph.NodeKindGate:
		if strings.EqualFold(node.Transform, "scope") || hasTag(node, "scope") {
			outputPath, err = filterScopeArtifact(inputPath, df, node, domain, policy)
		} else {
			outputPath, err = copyBuiltinArtifact(inputPath, df, node, false)
		}
	case graph.NodeKindTransform:
		outputPath, err = transformArtifact(inputPath, df, node)
	case graph.NodeKindSink:
		outputPath, err = correlateArtifact(inputPath, df, node)
	case graph.NodeKindMerge, graph.NodeKindCheckpoint, graph.NodeKindManual:
		outputPath, err = copyBuiltinArtifact(inputPath, df, node, true)
	default:
		err = fmt.Errorf("unsupported builtin kind %q", node.EffectiveKind())
	}
	result.end = time.Now()
	if err != nil {
		result.exitCode, result.err, result.stderr = 1, err, err.Error()
		return result
	}
	result.exitCode = 0
	result.outputFiles = []string{outputPath}
	return result
}

func copyBuiltinArtifact(inputPath string, df *DataFlow, node *graph.Node, normalize bool) (string, error) {
	lines, err := parseOutputFile(inputPath)
	if err != nil {
		return "", err
	}
	seen := make(map[string]struct{})
	var out []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if normalize {
			line = normalizeOutput(line)
		}
		if line == "" {
			continue
		}
		if _, ok := seen[line]; ok {
			continue
		}
		seen[line] = struct{}{}
		out = append(out, line)
	}
	sort.Strings(out)
	path := filepath.Join(df.WorkDir, df.RunID, "processed", fmt.Sprintf("L%02d-%s.txt", node.Layer, node.ID))
	return path, writeLines(path, out)
}

func transformArtifact(inputPath string, df *DataFlow, node *graph.Node) (string, error) {
	lines, err := parseOutputFile(inputPath)
	if err != nil {
		return "", err
	}
	seen := map[string]struct{}{}
	var out []string
	mode := strings.ToLower(strings.TrimSpace(node.Transform))
	for _, raw := range lines {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		switch mode {
		case "host-to-url":
			host, port, splitErr := net.SplitHostPort(value)
			if splitErr != nil {
				host, port = value, ""
			}
			scheme := "http"
			if port == "443" || port == "8443" {
				scheme = "https"
			}
			if port == "" || (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
				value = scheme + "://" + host
			} else {
				value = scheme + "://" + net.JoinHostPort(host, port)
			}
		default:
			value = normalizeOutput(value)
		}
		if _, ok := seen[value]; !ok {
			seen[value] = struct{}{}
			out = append(out, value)
		}
	}
	sort.Strings(out)
	path := filepath.Join(df.WorkDir, df.RunID, "processed", fmt.Sprintf("L%02d-%s.txt", node.Layer, node.ID))
	return path, writeLines(path, out)
}

func filterScopeArtifact(inputPath string, df *DataFlow, node *graph.Node, domain string, policy graph.WorkflowPolicy) (string, error) {
	lines, err := parseOutputFile(inputPath)
	if err != nil {
		return "", err
	}
	if len(policy.AllowedRoots) == 0 {
		policy.AllowedRoots = []string{domain}
	}
	var out []string
	seen := map[string]struct{}{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || !artifactInScope(line, domain, policy) {
			continue
		}
		if _, ok := seen[line]; !ok {
			seen[line] = struct{}{}
			out = append(out, line)
		}
	}
	sort.Strings(out)
	path := filepath.Join(df.WorkDir, df.RunID, "processed", fmt.Sprintf("L%02d-%s-scope.txt", node.Layer, node.ID))
	return path, writeLines(path, out)
}

func correlateArtifact(inputPath string, df *DataFlow, node *graph.Node) (string, error) {
	records, err := df.parseFile(inputPath, node.ID)
	if err != nil {
		return "", err
	}
	correlated := CorrelateRecords(records)
	path := filepath.Join(df.WorkDir, df.RunID, "analysis", fmt.Sprintf("%s-correlated.json", node.ID))
	data, err := json.MarshalIndent(correlated, "", "  ")
	if err != nil {
		return "", err
	}
	return path, os.WriteFile(path, data, 0o644)
}

func writeLines(path string, lines []string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	writer := bufio.NewWriter(file)
	for _, line := range lines {
		if _, err := fmt.Fprintln(writer, line); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func conditionMatches(condition string, files []string, df *DataFlow, cfg RunConfig) bool {
	condition = strings.TrimSpace(condition)
	if condition == "" || strings.EqualFold(condition, "always") {
		return true
	}
	if strings.Contains(condition, "||") {
		for _, part := range strings.Split(condition, "||") {
			if conditionMatches(part, files, df, cfg) {
				return true
			}
		}
		return false
	}
	if strings.Contains(condition, "&&") {
		for _, part := range strings.Split(condition, "&&") {
			if !conditionMatches(part, files, df, cfg) {
				return false
			}
		}
		return true
	}
	lower := strings.ToLower(condition)
	switch {
	case lower == "nonempty":
		for _, file := range files {
			if stat, err := os.Stat(file); err == nil && stat.Size() > 0 {
				return true
			}
		}
		return false
	case strings.HasPrefix(lower, "approved:"):
		key := strings.TrimSpace(condition[len("approved:"):])
		return cfg.Approvals[key]
	case strings.HasPrefix(lower, "has_type:"):
		want := strings.TrimSpace(lower[len("has_type:"):])
		for _, file := range files {
			records, err := df.parseFile(file, "condition")
			if err != nil {
				continue
			}
			for _, record := range records {
				if strings.EqualFold(record.Type, want) {
					return true
				}
			}
		}
		return false
	case strings.HasPrefix(lower, "contains:"):
		needle := strings.ToLower(strings.TrimSpace(condition[len("contains:"):]))
		if needle == "" {
			return false
		}
		for _, file := range files {
			f, err := os.Open(file)
			if err != nil {
				continue
			}
			scanner := bufio.NewScanner(f)
			found := false
			for scanner.Scan() {
				if strings.Contains(strings.ToLower(scanner.Text()), needle) {
					found = true
					break
				}
			}
			_ = f.Close()
			if found {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func policyAllows(node *graph.Node, workflow graph.WorkflowPolicy, cfg RunConfig) (bool, string) {
	approvalKey := node.Policy.Approval
	if approvalKey == "" {
		approvalKey = node.ID
	}
	approved := cfg.Approvals[approvalKey] || cfg.Approvals[node.ID]
	if node.Policy.RequiresApproval && !approved {
		return false, "approval required: " + approvalKey
	}
	if node.Policy.Intrusive && !(workflow.AllowIntrusive || cfg.AllowIntrusive || approved || cfg.Approvals["intrusive"]) {
		return false, "intrusive node requires explicit approval"
	}
	return true, ""
}

func targetAllowed(domain string, policy graph.WorkflowPolicy) bool {
	domain = normalizeHost(domain)
	for _, excluded := range policy.Excluded {
		if rootMatches(domain, substitutePolicyRoot(excluded, domain)) {
			return false
		}
	}
	if len(policy.AllowedRoots) == 0 {
		return true
	}
	for _, root := range policy.AllowedRoots {
		if rootMatches(domain, substitutePolicyRoot(root, domain)) {
			return true
		}
	}
	return false
}

func artifactInScope(value, domain string, policy graph.WorkflowPolicy) bool {
	host := candidateHost(value)
	if host == "" {
		return false
	}
	for _, excluded := range policy.Excluded {
		if rootMatches(host, substitutePolicyRoot(excluded, domain)) {
			return false
		}
	}
	roots := policy.AllowedRoots
	if len(roots) == 0 {
		roots = []string{domain}
	}
	for _, root := range roots {
		if rootMatches(host, substitutePolicyRoot(root, domain)) {
			return true
		}
	}
	return false
}

func candidateHost(value string) string {
	value = strings.TrimSpace(value)
	if parsed, err := url.Parse(value); err == nil && parsed.Hostname() != "" {
		return normalizeHost(parsed.Hostname())
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		return normalizeHost(host)
	}
	fields := strings.Fields(value)
	if len(fields) > 0 {
		value = fields[0]
	}
	return normalizeHost(strings.TrimSuffix(value, "."))
}

func normalizeHost(value string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "."))
}

func rootMatches(host, root string) bool {
	host, root = normalizeHost(host), normalizeHost(root)
	return root != "" && (host == root || strings.HasSuffix(host, "."+root))
}

func substitutePolicyRoot(root, domain string) string {
	return strings.ReplaceAll(root, "{{domain}}", domain)
}

func onlyRootParents(parents []string, root string) bool {
	if len(parents) == 0 {
		return false
	}
	for _, parent := range parents {
		if parent != root {
			return false
		}
	}
	return true
}

func outputFiles(df *DataFlow, nodeID string) []string {
	if output := df.NodeOutputs[nodeID]; output != nil {
		return append([]string(nil), output.OutputFiles...)
	}
	return nil
}

func categoryForNode(node *graph.Node) string {
	if node.Subgraph != "" {
		return node.Subgraph
	}
	return string(node.EffectiveKind())
}

func hasTag(node *graph.Node, want string) bool {
	for _, tag := range node.Tags {
		if strings.EqualFold(tag, want) {
			return true
		}
	}
	return false
}

func decorateOutput(df *DataFlow, node *graph.Node, parents []string, attempts int) {
	output := df.NodeOutputs[node.ID]
	if output == nil {
		return
	}
	if output.Metadata == nil {
		output.Metadata = make(map[string]string)
	}
	output.Metadata["kind"] = string(node.EffectiveKind())
	output.Metadata["parents"] = strings.Join(parents, ",")
	output.Metadata["attempts"] = fmt.Sprintf("%d", attempts)
	output.Metadata["condition"] = node.Condition
	output.Metadata["inputs"] = artifactTypes(node.Inputs)
	output.Metadata["outputs"] = artifactTypes(node.Outputs)
}

func artifactTypes(types []graph.ArtifactType) string {
	parts := make([]string, len(types))
	for i, t := range types {
		parts[i] = string(t)
	}
	return strings.Join(parts, ",")
}

func containsID(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
