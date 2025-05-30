package pipeline

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// DataFlow manages file-based data flow between workflow nodes
type DataFlow struct {
	WorkDir     string
	RunID       string
	NodeOutputs map[string]*NodeOutput
	GlobalState *GlobalState
}

// NodeOutput represents the output from a single tool execution
type NodeOutput struct {
	NodeID      string            `json:"node_id"`
	Tool        string            `json:"tool"`
	StartTime   time.Time         `json:"start_time"`
	EndTime     time.Time         `json:"end_time"`
	ExitCode    int               `json:"exit_code"`
	OutputFiles []string          `json:"output_files"`
	ErrorLog    string            `json:"error_log"`
	Metadata    map[string]string `json:"metadata"`
	LineCount   int               `json:"line_count"`
	FileSize    int64             `json:"file_size"`
	Format      string            `json:"format"` // txt, json, csv, xml
}

// GlobalState tracks the overall workflow execution state
type GlobalState struct {
	RunID        string                 `json:"run_id"`
	StartTime    time.Time              `json:"start_time"`
	Domain       string                 `json:"domain"`
	WorkflowPath string                 `json:"workflow_path"`
	NodeStates   map[string]NodeStatus  `json:"node_states"`
	DataLinks    map[string][]string    `json:"data_links"` // node_id -> input_files
	Statistics   *ExecutionStatistics   `json:"statistics"`
}

// NodeStatus tracks individual node execution status
type NodeStatus int

const (
	NodePending NodeStatus = iota
	NodeRunning
	NodeCompleted
	NodeFailed
	NodeSkipped
)

// ExecutionStatistics provides workflow execution metrics
type ExecutionStatistics struct {
	TotalNodes       int           `json:"total_nodes"`
	CompletedNodes   int           `json:"completed_nodes"`
	FailedNodes      int           `json:"failed_nodes"`
	TotalResults     int           `json:"total_results"`
	UniqueResults    int           `json:"unique_results"`
	ExecutionTime    time.Duration `json:"execution_time"`
	ParallelEfficiency float64     `json:"parallel_efficiency"`
}

// DataProcessor handles different data formats and transformations
type DataProcessor struct {
	parsers    map[string]Parser
	validators map[string]Validator
	formatters map[string]Formatter
}

// Parser interface for different file formats
type Parser interface {
	Parse(filepath string) ([]DataRecord, error)
	GetFormat() string
}

// Validator interface for data validation
type Validator interface {
	Validate(record DataRecord) bool
	GetValidationRules() []string
}

// Formatter interface for output formatting
type Formatter interface {
	Format(records []DataRecord, outputPath string) error
	GetMimeType() string
}

// DataRecord represents a single piece of data (URL, domain, IP, etc.)
type DataRecord struct {
	Value      string            `json:"value"`
	Type       string            `json:"type"`       // domain, url, ip, port, etc.
	Source     string            `json:"source"`     // which tool generated this
	Layer      int               `json:"layer"`      // workflow layer
	Timestamp  time.Time         `json:"timestamp"`
	Confidence float64           `json:"confidence"` // 0.0 to 1.0
	Metadata   map[string]string `json:"metadata"`
}

// NewDataFlow creates a new data flow manager
func NewDataFlow(workDir, domain string) (*DataFlow, error) {
	runID := fmt.Sprintf("run-%d", time.Now().Unix())
	
	df := &DataFlow{
		WorkDir:     workDir,
		RunID:       runID,
		NodeOutputs: make(map[string]*NodeOutput),
		GlobalState: &GlobalState{
			RunID:        runID,
			StartTime:    time.Now(),
			Domain:       domain,
			NodeStates:   make(map[string]NodeStatus),
			DataLinks:    make(map[string][]string),
			Statistics:   &ExecutionStatistics{},
		},
	}
	
	// Create run-specific directory
	runDir := filepath.Join(workDir, runID)
	if err := os.MkdirAll(runDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create run directory: %w", err)
	}
	
	// Create subdirectories for organization
	dirs := []string{"raw", "processed", "merged", "analysis", "logs"}
	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(runDir, dir), 0755); err != nil {
			return nil, fmt.Errorf("failed to create %s directory: %w", dir, err)
		}
	}
	
	return df, nil
}

// CreateSeedFile creates the initial input file with the target domain
func (df *DataFlow) CreateSeedFile() (string, error) {
	seedPath := filepath.Join(df.WorkDir, df.RunID, "raw", "00-seed.txt")
	content := fmt.Sprintf("%s\n", df.GlobalState.Domain)
	
	if err := os.WriteFile(seedPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to create seed file: %w", err)
	}
	
	// Create seed node output record
	df.NodeOutputs["seed"] = &NodeOutput{
		NodeID:      "seed",
		Tool:        "input",
		StartTime:   time.Now(),
		EndTime:     time.Now(),
		ExitCode:    0,
		OutputFiles: []string{seedPath},
		LineCount:   1,
		FileSize:    int64(len(content)),
		Format:      "txt",
		Metadata:    map[string]string{"type": "domain", "source": "user_input"},
	}
	
	return seedPath, nil
}

// PrepareNodeInput prepares input files for a node based on its parents
func (df *DataFlow) PrepareNodeInput(nodeID string, parentIDs []string, layer int) (string, error) {
	if len(parentIDs) == 0 {
		return "", fmt.Errorf("no parent nodes specified for %s", nodeID)
	}
	
	// For single parent, use its output directly
	if len(parentIDs) == 1 {
		parentOutput, exists := df.NodeOutputs[parentIDs[0]]
		if !exists {
			return "", fmt.Errorf("parent node %s has no output", parentIDs[0])
		}
		
		if len(parentOutput.OutputFiles) == 0 {
			return "", fmt.Errorf("parent node %s has no output files", parentIDs[0])
		}
		
		// Use the merged output if available, otherwise the first output file
		for _, file := range parentOutput.OutputFiles {
			if strings.Contains(file, "merged") {
				df.GlobalState.DataLinks[nodeID] = []string{file}
				return file, nil
			}
		}
		
		inputFile := parentOutput.OutputFiles[0]
		df.GlobalState.DataLinks[nodeID] = []string{inputFile}
		return inputFile, nil
	}
	
	// For multiple parents, merge their outputs
	return df.mergeParentOutputs(nodeID, parentIDs, layer)
}

// mergeParentOutputs combines outputs from multiple parent nodes
func (df *DataFlow) mergeParentOutputs(nodeID string, parentIDs []string, layer int) (string, error) {
	mergedPath := filepath.Join(df.WorkDir, df.RunID, "merged", 
		fmt.Sprintf("L%02d-%s-input.txt", layer, nodeID))
	
	var allRecords []DataRecord
	var inputFiles []string
	
	for _, parentID := range parentIDs {
		parentOutput, exists := df.NodeOutputs[parentID]
		if !exists {
			continue
		}
		
		for _, outputFile := range parentOutput.OutputFiles {
			inputFiles = append(inputFiles, outputFile)
			records, err := df.parseFile(outputFile, parentID)
			if err != nil {
				continue // Skip files that can't be parsed
			}
			allRecords = append(allRecords, records...)
		}
	}
	
	// Deduplicate and sort records
	uniqueRecords := df.deduplicateRecords(allRecords)
	sort.Slice(uniqueRecords, func(i, j int) bool {
		return uniqueRecords[i].Value < uniqueRecords[j].Value
	})
	
	// Write merged file
	file, err := os.Create(mergedPath)
	if err != nil {
		return "", fmt.Errorf("failed to create merged file: %w", err)
	}
	defer file.Close()
	
	writer := bufio.NewWriter(file)
	for _, record := range uniqueRecords {
		fmt.Fprintln(writer, record.Value)
	}
	writer.Flush()
	
	df.GlobalState.DataLinks[nodeID] = inputFiles
	return mergedPath, nil
}

// RecordNodeOutput records the output from a completed node
func (df *DataFlow) RecordNodeOutput(nodeID, tool string, startTime, endTime time.Time, 
	exitCode int, outputFiles []string, errorLog string) error {
	
	// Calculate file statistics
	var totalSize int64
	var totalLines int
	var format string
	
	for _, file := range outputFiles {
		if stat, err := os.Stat(file); err == nil {
			totalSize += stat.Size()
		}
		
		if lines, err := df.countLines(file); err == nil {
			totalLines += lines
		}
		
		// Detect format from first file
		if format == "" {
			format = df.detectFormat(file)
		}
	}
	
	nodeOutput := &NodeOutput{
		NodeID:      nodeID,
		Tool:        tool,
		StartTime:   startTime,
		EndTime:     endTime,
		ExitCode:    exitCode,
		OutputFiles: outputFiles,
		ErrorLog:    errorLog,
		LineCount:   totalLines,
		FileSize:    totalSize,
		Format:      format,
		Metadata:    make(map[string]string),
	}
	
	// Store node output
	df.NodeOutputs[nodeID] = nodeOutput
	
	// Update global state
	if exitCode == 0 {
		df.GlobalState.NodeStates[nodeID] = NodeCompleted
		df.GlobalState.Statistics.CompletedNodes++
	} else {
		df.GlobalState.NodeStates[nodeID] = NodeFailed
		df.GlobalState.Statistics.FailedNodes++
	}
	
	// Create analysis summary
	if err := df.createNodeAnalysis(nodeOutput); err != nil {
		return fmt.Errorf("failed to create node analysis: %w", err)
	}
	
	return nil
}

// ProcessNodeOutputs processes and validates all output files for a node
func (df *DataFlow) ProcessNodeOutputs(nodeID string) error {
	nodeOutput, exists := df.NodeOutputs[nodeID]
	if !exists {
		return fmt.Errorf("no output recorded for node %s", nodeID)
	}
	
	var processedFiles []string
	
	for _, outputFile := range nodeOutput.OutputFiles {
		// Parse and validate the file
		records, err := df.parseFile(outputFile, nodeID)
		if err != nil {
			continue // Skip invalid files
		}
		
		// Filter and clean records
		validRecords := df.filterValidRecords(records)
		
		// Create processed version
		processedPath := filepath.Join(df.WorkDir, df.RunID, "processed",
			fmt.Sprintf("%s-%s.json", nodeID, filepath.Base(outputFile)))
		
		if err := df.writeJSONRecords(validRecords, processedPath); err != nil {
			continue
		}
		
		processedFiles = append(processedFiles, processedPath)
	}
	
	// Update node output with processed files
	nodeOutput.Metadata["processed_files"] = strings.Join(processedFiles, ",")
	
	return nil
}

// GetLatestOutput returns the most recent output file for a node
func (df *DataFlow) GetLatestOutput(nodeID string) (string, error) {
	nodeOutput, exists := df.NodeOutputs[nodeID]
	if !exists {
		return "", fmt.Errorf("no output for node %s", nodeID)
	}
	
	if len(nodeOutput.OutputFiles) == 0 {
		return "", fmt.Errorf("no output files for node %s", nodeID)
	}
	
	// Return the last (most recent) output file
	return nodeOutput.OutputFiles[len(nodeOutput.OutputFiles)-1], nil
}

// CreateExecutionReport generates a comprehensive execution report
func (df *DataFlow) CreateExecutionReport() error {
	reportPath := filepath.Join(df.WorkDir, df.RunID, "execution-report.json")
	
	// Update final statistics
	df.GlobalState.Statistics.ExecutionTime = time.Since(df.GlobalState.StartTime)
	df.GlobalState.Statistics.TotalNodes = len(df.NodeOutputs)
	
	// Calculate unique results across all nodes
	allRecords := make(map[string]DataRecord)
	for _, nodeOutput := range df.NodeOutputs {
		for _, outputFile := range nodeOutput.OutputFiles {
			records, err := df.parseFile(outputFile, nodeOutput.NodeID)
			if err != nil {
				continue
			}
			for _, record := range records {
				allRecords[record.Value] = record
			}
		}
	}
	
	df.GlobalState.Statistics.TotalResults = len(allRecords)
	df.GlobalState.Statistics.UniqueResults = len(allRecords)
	
	// Write report
	reportData, err := json.MarshalIndent(df.GlobalState, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal report: %w", err)
	}
	
	return os.WriteFile(reportPath, reportData, 0644)
}

// Helper methods

func (df *DataFlow) parseFile(filePath, sourceNode string) ([]DataRecord, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	
	var records []DataRecord
	scanner := bufio.NewScanner(file)
	
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		
		record := DataRecord{
			Value:      line,
			Type:       df.inferDataType(line),
			Source:     sourceNode,
			Timestamp:  time.Now(),
			Confidence: 1.0,
			Metadata:   make(map[string]string),
		}
		
		records = append(records, record)
	}
	
	return records, scanner.Err()
}

func (df *DataFlow) deduplicateRecords(records []DataRecord) []DataRecord {
	seen := make(map[string]DataRecord)
	
	for _, record := range records {
		existing, exists := seen[record.Value]
		if !exists || record.Confidence > existing.Confidence {
			seen[record.Value] = record
		}
	}
	
	var unique []DataRecord
	for _, record := range seen {
		unique = append(unique, record)
	}
	
	return unique
}

func (df *DataFlow) filterValidRecords(records []DataRecord) []DataRecord {
	var valid []DataRecord
	
	for _, record := range records {
		if df.isValidRecord(record) {
			valid = append(valid, record)
		}
	}
	
	return valid
}

func (df *DataFlow) isValidRecord(record DataRecord) bool {
	value := strings.TrimSpace(record.Value)
	if value == "" {
		return false
	}
	
	// Basic validation based on type
	switch record.Type {
	case "domain":
		return df.isValidDomain(value)
	case "url":
		return df.isValidURL(value)
	case "ip":
		return df.isValidIP(value)
	default:
		return len(value) > 0 && len(value) < 1000
	}
}

func (df *DataFlow) inferDataType(value string) string {
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return "url"
	}
	if df.isValidIP(value) {
		return "ip"
	}
	if df.isValidDomain(value) {
		return "domain"
	}
	return "unknown"
}

func (df *DataFlow) isValidDomain(value string) bool {
	return len(value) > 0 && strings.Contains(value, ".") && !strings.Contains(value, " ")
}

func (df *DataFlow) isValidURL(value string) bool {
	return strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://")
}

func (df *DataFlow) isValidIP(value string) bool {
	parts := strings.Split(value, ".")
	return len(parts) == 4
}

func (df *DataFlow) countLines(filePath string) (int, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	
	count := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		count++
	}
	
	return count, scanner.Err()
}

func (df *DataFlow) detectFormat(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".json":
		return "json"
	case ".csv":
		return "csv"
	case ".xml":
		return "xml"
	default:
		return "txt"
	}
}

func (df *DataFlow) writeJSONRecords(records []DataRecord, outputPath string) error {
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile(outputPath, data, 0644)
}

func (df *DataFlow) createNodeAnalysis(nodeOutput *NodeOutput) error {
	analysisPath := filepath.Join(df.WorkDir, df.RunID, "analysis", 
		fmt.Sprintf("%s-analysis.json", nodeOutput.NodeID))
	
	analysis := map[string]interface{}{
		"node_id":      nodeOutput.NodeID,
		"tool":         nodeOutput.Tool,
		"runtime":      nodeOutput.EndTime.Sub(nodeOutput.StartTime).String(),
		"exit_code":    nodeOutput.ExitCode,
		"file_count":   len(nodeOutput.OutputFiles),
		"total_lines":  nodeOutput.LineCount,
		"total_size":   nodeOutput.FileSize,
		"format":       nodeOutput.Format,
		"success":      nodeOutput.ExitCode == 0,
		"generated_at": time.Now().Format(time.RFC3339),
	}
	
	data, err := json.MarshalIndent(analysis, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile(analysisPath, data, 0644)
}