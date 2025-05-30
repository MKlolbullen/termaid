package pipeline

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/log"
)

/* ─────────────────────────── Config Structs ───────────────────────────── */

type Tool struct {
	Name       string   `yaml:"-"`      // here = node.ID (unique)
	Command    string   `yaml:"cmd"`    // actual binary (node.Tool)
	Args       []string `yaml:"args"`   // already split
	Output     string   `yaml:"output"` // resolved unique output file
	Parallel   bool     `yaml:"parallel"`
	Stdin      bool     `yaml:"stdin"`
	OutputType string   `yaml:"output_type"` // txt, json, xml, etc.
	Timeout    int      `yaml:"timeout"`     // execution timeout in seconds
}

type Category struct {
	Name  string `yaml:"name"`
	Tools []Tool `yaml:"tools"`
}

/* ─────────────────────────── Status Bus ─────────────────────────────── */

type StatusUpdateType int

const (
	StatusStart StatusUpdateType = iota
	StatusFinish
	StatusError
)

type Status struct {
	Type     StatusUpdateType
	Category string
	Tool     string // node.ID
	Err      error
}

/* ─────────────────────────── Run Engine ─────────────────────────────── */

func Run(
	ctx context.Context,
	domain string,
	workdir string,
	cats []Category,
	concurrency int,
	out chan<- Status,
) error {

	if err := os.MkdirAll(workdir, 0o755); err != nil {
		return err
	}

	// Initialize data flow manager
	dataFlow, err := NewDataFlow(workdir, domain)
	if err != nil {
		return fmt.Errorf("failed to initialize data flow: %w", err)
	}

	// Create seed file
	prevPath, err := dataFlow.CreateSeedFile()
	if err != nil {
		return fmt.Errorf("failed to create seed file: %w", err)
	}

	for _, cat := range cats {

		catDir := filepath.Join(workdir, dataFlow.RunID, "raw", dirSafe(cat.Name))
		if err := os.MkdirAll(catDir, 0o755); err != nil {
			return err
		}

		sem := make(chan struct{}, concurrency)
		wg := sync.WaitGroup{}

		for _, t := range cat.Tools {
			tool := t
			wg.Add(1)

			launch := func() {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				_ = runTool(ctx, &tool, cat.Name, catDir, prevPath, dataFlow, out)
			}

			if tool.Parallel {
				go launch()
			} else {
				launch()
			}
		}

		wg.Wait()

		// Process outputs and prepare for next layer
		for _, t := range cat.Tools {
			if err := dataFlow.ProcessNodeOutputs(t.Name); err != nil {
				log.Debug("Failed to process node outputs", "node", t.Name, "error", err)
			}
		}

		// Get merged output for next layer
		if len(cat.Tools) > 0 {
			prevPath, err = dataFlow.GetLatestOutput(cat.Tools[0].Name)
			if err != nil {
				return fmt.Errorf("failed to get latest output: %w", err)
			}
		}
	}

	// Create final execution report
	if err := dataFlow.CreateExecutionReport(); err != nil {
		log.Debug("Failed to create execution report", "error", err)
	}

	return nil
}

/* ─────────────────────────── Helpers ──────────────────────────────── */

func runTool(
	ctx context.Context,
	tool *Tool, // node-derived unique tool
	catName, catDir string,
	inputPath string,
	dataFlow *DataFlow,
	out chan<- Status,
) error {

	startTime := time.Now()
	var outputFiles []string
	var errorLog strings.Builder

	// Create unique output file for this tool
	outputFile := filepath.Join(catDir, fmt.Sprintf("%s-%d.txt", tool.Name, startTime.Unix()))
	outputFiles = append(outputFiles, outputFile)

	// prepare args with placeholder substitution
	args := make([]string, len(tool.Args))
	copy(args, tool.Args)

	for i, a := range args {
		if strings.Contains(a, "{{input}}") {
			args[i] = strings.ReplaceAll(a, "{{input}}", inputPath)
		}
		if strings.Contains(a, "{{domain}}") {
			domain := strings.TrimSpace(readFirstLine(inputPath))
			args[i] = strings.ReplaceAll(a, "{{domain}}", domain)
		}
		if strings.Contains(a, "{{output}}") {
			args[i] = strings.ReplaceAll(a, "{{output}}", outputFile)
		}
	}

	// Validate tool before execution
	if err := validateTool(tool); err != nil {
		out <- Status{Type: StatusError, Category: catName, Tool: tool.Name, Err: err}
		dataFlow.RecordNodeOutput(tool.Name, tool.Command, startTime, time.Now(), 1, outputFiles, err.Error())
		return err
	}

	out <- Status{Type: StatusStart, Category: catName, Tool: tool.Name}

	cmd := exec.CommandContext(ctx, tool.Command, args...)
	cmd.Dir = catDir

	// Set environment variables for better tool compatibility
	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"PYTHONUNBUFFERED=1",
		"FORCE_COLOR=0",
	)

	// For tools that read from stdin, setup input redirection
	if tool.Stdin {
		inputFile, err := os.Open(inputPath)
		if err != nil {
			out <- Status{Type: StatusError, Category: catName, Tool: tool.Name, Err: err}
			dataFlow.RecordNodeOutput(tool.Name, tool.Command, startTime, time.Now(), 1, outputFiles, err.Error())
			return err
		}
		defer inputFile.Close()
		cmd.Stdin = inputFile
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		out <- Status{Type: StatusError, Category: catName, Tool: tool.Name, Err: err}
		dataFlow.RecordNodeOutput(tool.Name, tool.Command, startTime, time.Now(), 1, outputFiles, err.Error())
		return err
	}

	go func() {
		defer stderr.Close()
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			line := sc.Text()
			errorLog.WriteString(line + "\n")
			log.Debug("stderr", "cat", catName, "tool", tool.Name, "line", line)
		}
	}()

	err = cmd.Run()
	endTime := time.Now()
	exitCode := 0

	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		} else {
			exitCode = 1
		}
		out <- Status{Type: StatusError, Category: catName, Tool: tool.Name, Err: err}
	} else {
		out <- Status{Type: StatusFinish, Category: catName, Tool: tool.Name}
	}

	// Record the node output regardless of success/failure
	dataFlow.RecordNodeOutput(tool.Name, tool.Command, startTime, endTime, exitCode, outputFiles, errorLog.String())

	return err
}

/* mergeOutputs: process and merge all tool outputs with format detection - deprecated in favor of DataFlow */
func mergeOutputs(dir string) (string, error) {
	// Find all potential output files
	patterns := []string{"*.txt", "*.json", "*.xml", "*.csv"}
	var allFiles []string

	for _, pattern := range patterns {
		if files, err := filepath.Glob(filepath.Join(dir, pattern)); err == nil {
			allFiles = append(allFiles, files...)
		}
	}

	merged := filepath.Join(dir, "merged.txt")
	outF, err := os.Create(merged)
	if err != nil {
		return "", err
	}
	defer outF.Close()

	seen := map[string]struct{}{}
	bw := bufio.NewWriter(outF)
	defer bw.Flush()

	if len(allFiles) == 0 {
		log.Debug("No output files found in directory", "dir", dir)
		return merged, nil
	}

	for _, f := range allFiles {
		lines, err := parseOutputFile(f)
		if err != nil {
			log.Debug("Failed to parse output file", "file", f, "error", err)
			continue
		}

		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}

			// Normalize URLs and domains
			line = normalizeOutput(line)

			if _, dup := seen[line]; dup {
				continue
			}
			seen[line] = struct{}{}
			fmt.Fprintln(bw, line)
		}
	}

	log.Debug("Merged outputs", "dir", dir, "unique_lines", len(seen), "total_files", len(allFiles))
	return merged, nil
}

func seedInput(domain string) (string, error) {
	temp := filepath.Join(os.TempDir(), "termaid-domain.txt")
	if err := os.WriteFile(temp, []byte(domain+"\n"), 0644); err != nil {
		return "", fmt.Errorf("failed to write seed file: %w", err)
	}
	return temp, nil
}

func dirSafe(s string) string { return strings.ReplaceAll(strings.ToLower(s), " ", "_") }

// validateTool checks if a tool exists and is executable
func validateTool(tool *Tool) error {
	// Check if command exists in PATH
	if _, err := exec.LookPath(tool.Command); err != nil {
		return fmt.Errorf("command not found: %s (install it or check PATH)", tool.Command)
	}

	// Tool-specific validations
	switch tool.Command {
	case "nuclei":
		if !strings.Contains(tool.Args[0], "-t") && !strings.Contains(tool.Args[0], "-w") {
			return fmt.Errorf("nuclei requires templates (-t) or workflows (-w)")
		}
	case "ffuf":
		if !strings.Contains(tool.Args[0], "-w") {
			return fmt.Errorf("ffuf requires a wordlist (-w)")
		}
	case "gobuster":
		if !strings.Contains(tool.Args[0], "-w") {
			return fmt.Errorf("gobuster requires a wordlist (-w)")
		}
	}

	return nil
}

// parseOutputFile parses different output formats and returns normalized lines
func parseOutputFile(filepath string) ([]string, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	ext := strings.ToLower(filepath[strings.LastIndex(filepath, "."):])

	switch ext {
	case ".json":
		return parseJSONOutput(file)
	case ".xml":
		return parseXMLOutput(file)
	case ".csv":
		return parseCSVOutput(file)
	default:
		return parseTextOutput(file)
	}
}

// parseJSONOutput extracts relevant data from JSON output
func parseJSONOutput(file *os.File) ([]string, error) {
	var lines []string
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		// Try to parse as JSON
		var jsonData map[string]interface{}
		if err := json.Unmarshal([]byte(line), &jsonData); err == nil {
			// Extract common fields
			if url, ok := jsonData["url"].(string); ok {
				lines = append(lines, url)
			}
			if host, ok := jsonData["host"].(string); ok {
				lines = append(lines, host)
			}
			if input, ok := jsonData["input"].(string); ok {
				lines = append(lines, input)
			}
		} else {
			// Fallback to treating as text
			lines = append(lines, line)
		}
	}

	return lines, scanner.Err()
}

// parseXMLOutput extracts data from XML output (basic implementation)
func parseXMLOutput(file *os.File) ([]string, error) {
	// For now, treat XML as text and extract URL-like patterns
	return parseTextOutput(file)
}

// parseCSVOutput extracts data from CSV output
func parseCSVOutput(file *os.File) ([]string, error) {
	var lines []string
	scanner := bufio.NewScanner(file)

	// Skip header if present
	if scanner.Scan() {
		header := scanner.Text()
		if !strings.Contains(header, "http") && !strings.Contains(header, ".") {
			// Looks like a header, skip it
		} else {
			lines = append(lines, header)
		}
	}

	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), ",")
		for _, field := range fields {
			field = strings.Trim(field, `"`)
			if isValidTarget(field) {
				lines = append(lines, field)
			}
		}
	}

	return lines, scanner.Err()
}

// parseTextOutput handles standard text output
func parseTextOutput(file *os.File) ([]string, error) {
	var lines []string
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}

	return lines, scanner.Err()
}

// normalizeOutput standardizes output format
func normalizeOutput(line string) string {
	// Remove common prefixes and suffixes
	line = strings.TrimPrefix(line, "http://")
	line = strings.TrimPrefix(line, "https://")
	line = strings.TrimSuffix(line, "/")

	// Remove port numbers for standard ports
	line = regexp.MustCompile(`:80$|:443$`).ReplaceAllString(line, "")

	// Clean up any remaining whitespace
	return strings.TrimSpace(line)
}

// isValidTarget checks if a string looks like a valid target (URL, domain, IP)
func isValidTarget(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}

	// Check for URL patterns
	if strings.Contains(s, "http://") || strings.Contains(s, "https://") {
		return true
	}

	// Check for domain patterns
	if matched, _ := regexp.MatchString(`^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?)*$`, s); matched {
		return true
	}

	// Check for IP addresses
	if matched, _ := regexp.MatchString(`^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}(:\d+)?$`, s); matched {
		return true
	}

	return false
}

func readFirstLine(path string) string {
	f, err := os.Open(path)
	if err != nil {
		log.Debug("Failed to open file for reading first line", "path", path, "error", err)
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if sc.Scan() {
		return sc.Text()
	}
	return ""
}
