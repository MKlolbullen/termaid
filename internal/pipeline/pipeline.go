package pipeline

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/charmbracelet/log"

	"bb-runner/internal/graph"
)

/* ─────────────────────────── Config Structs ───────────────────────────── */

type Tool struct {
	Name     string   `yaml:"-"`      // here = node.ID (unique)
	Command  string   `yaml:"cmd"`    // actual binary (node.Tool)
	Args     []string `yaml:"args"`   // already split
	Output   string   `yaml:"output"` // resolved unique output file
	Parallel bool     `yaml:"parallel"`
	Stdin    bool     `yaml:"stdin"`
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

	// seed file with domain
	prevPath, prevReader := seedInput(domain)

	for _, cat := range cats {

		catDir := filepath.Join(workdir, dirSafe(cat.Name))
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
				_ = runTool(ctx, &tool, cat.Name, catDir, prevPath, prevReader, out)
			}

			if tool.Parallel {
				go launch()
			} else {
				launch()
			}
		}

		wg.Wait()

		var err error
		prevPath, prevReader, err = mergeOutputs(catDir)
		if err != nil {
			return err
		}
	}
	return nil
}

/* ─────────────────────────── Helpers ──────────────────────────────── */

func runTool(
	ctx context.Context,
	tool *Tool,          // node-derived unique tool
	catName, catDir string,
	inputPath string,
	inputReader io.Reader,
	out chan<- Status,
) error {

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
			outFile := filepath.Join(catDir, tool.Output)
			args[i] = strings.ReplaceAll(a, "{{output}}", outFile)
		}
	}

	out <- Status{Type: StatusStart, Category: catName, Tool: tool.Name}

	cmd := exec.CommandContext(ctx, tool.Command, args...)
	cmd.Dir = catDir

	if tool.Stdin {
		cmd.Stdin = inputReader
	}

	stderr, _ := cmd.StderrPipe()
	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			log.Debug("stderr", "cat", catName, "tool", tool.Name, "line", sc.Text())
		}
	}()

	if err := cmd.Run(); err != nil {
		out <- Status{Type: StatusError, Category: catName, Tool: tool.Name, Err: err}
		return err
	}

	out <- Status{Type: StatusFinish, Category: catName, Tool: tool.Name}
	return nil
}

/* mergeOutputs: dedup all *.txt into merged.txt and tee to next layer */
func mergeOutputs(dir string) (string, io.Reader, error) {
	txts, err := filepath.Glob(filepath.Join(dir, "*.txt"))
	if err != nil {
		return "", nil, err
	}
	merged := filepath.Join(dir, "merged.txt")
	outF, err := os.Create(merged)
	if err != nil {
		return "", nil, err
	}

	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		defer outF.Close()
		seen := map[string]struct{}{}
		bw := bufio.NewWriter(outF)
		for _, f := range txts {
			in, _ := os.Open(f)
			sc := bufio.NewScanner(in)
			for sc.Scan() {
				line := sc.Text()
				if _, dup := seen[line]; dup {
					continue
				}
				seen[line] = struct{}{}
				fmt.Fprintln(bw, line)
				fmt.Fprintln(pw, line)
			}
			in.Close()
		}
		bw.Flush()
	}()

	return merged, pr, nil
}

func seedInput(domain string) (string, io.Reader) {
	temp := filepath.Join(os.TempDir(), "bb-runner-domain.txt")
	_ = os.WriteFile(temp, []byte(domain+"\n"), 0644)
	return temp, strings.NewReader(domain + "\n")
}

func dirSafe(s string) string { return strings.ReplaceAll(strings.ToLower(s), " ", "_") }

func readFirstLine(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if sc.Scan() {
		return sc.Text()
	}
	return ""
}
