package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/MKlolbullen/termaid/internal/graph"
	"github.com/MKlolbullen/termaid/internal/pipeline"
)

// ToolSummary is a catalog entry exposed for non-interactive consumers (the
// CLI, tests). It mirrors the internal catalogEntry without leaking the
// unexported type.
type ToolSummary struct {
	Name string
	Cat  string
	In   string
	Out  string
	Desc string
	Def  string
}

// CatalogInfo returns the active tool catalog as a slice of summaries, sorted
// the same way the interactive picker groups them (category, then name).
func CatalogInfo() []ToolSummary {
	out := make([]ToolSummary, 0, len(catalog))
	for _, e := range catalog {
		out = append(out, ToolSummary{
			Name: e.Name,
			Cat:  e.Cat,
			In:   e.In,
			Out:  e.Out,
			Desc: e.Desc,
			Def:  e.Def,
		})
	}
	return out
}

// MermaidForWorkflow returns the Mermaid representation of a workflow. A .mmd
// file is returned verbatim; a workflow JSON file is loaded and rendered.
func MermaidForWorkflow(path string) (string, error) {
	if strings.HasSuffix(path, ".mmd") {
		b, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	dag, err := LoadWorkflow(path)
	if err != nil {
		return "", err
	}
	return dag.ToMermaid(), nil
}

// ValidateWorkflow loads a workflow and checks its matrix for consistency.
// The DAG is returned even when validation fails, so callers can still report
// structural details.
func ValidateWorkflow(path string) (*graph.DAG, error) {
	dag, err := LoadWorkflow(path)
	if err != nil {
		return nil, err
	}
	return dag, dag.ValidateMatrix()
}

// RunHeadless executes a workflow without the TUI, streaming human-readable
// status lines to w. It blocks until the run completes and returns the first
// fatal error, if any.
func RunHeadless(ctx context.Context, path, domain, workdir string, concurrency int, w io.Writer) error {
	if strings.TrimSpace(domain) == "" {
		return fmt.Errorf("domain must not be empty")
	}
	if concurrency < 1 {
		concurrency = 1
	}

	dag, err := LoadWorkflow(path)
	if err != nil {
		return fmt.Errorf("load workflow %q: %w", path, err)
	}

	cats := dagToCategories(dag)
	if len(cats) == 0 {
		return fmt.Errorf("workflow %q contains no runnable tools", path)
	}

	fmt.Fprintf(w, "▶ running %q against %s (%d steps, concurrency %d)\n",
		path, domain, len(cats), concurrency)

	ch := make(chan pipeline.Status, 128)
	errCh := make(chan error, 1)
	go func() {
		errCh <- pipeline.Run(ctx, domain, workdir, cats, concurrency, ch)
		close(ch)
	}()

	var failures int
	for st := range ch {
		if st.Type == pipeline.StatusError {
			failures++
		}
		fmt.Fprintf(w, "  [%s] %-20s %s\n", st.Category, st.Tool, headlessStatus(st))
	}

	if err := <-errCh; err != nil {
		return err
	}
	fmt.Fprintf(w, "✔ finished: %d step(s), %d tool error(s). Results in %s/\n",
		len(cats), failures, workdir)
	return nil
}

func headlessStatus(s pipeline.Status) string {
	switch s.Type {
	case pipeline.StatusStart:
		return "started"
	case pipeline.StatusFinish:
		return "done"
	case pipeline.StatusError:
		if s.Err != nil {
			return "error: " + s.Err.Error()
		}
		return "error"
	default:
		return "?"
	}
}
