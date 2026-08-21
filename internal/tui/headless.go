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

// ToolSummary is a catalog entry exposed for non-interactive consumers.
type ToolSummary struct {
	Name string
	Cat  string
	In   string
	Out  string
	Desc string
	Def  string
}

// HeadlessRunOptions exposes the v3 execution-controller features without
// coupling the CLI directly to pipeline internals.
type HeadlessRunOptions struct {
	ResumeRunID      string
	ApproveIntrusive bool
	Approvals        map[string]bool
}

func CatalogInfo() []ToolSummary {
	out := make([]ToolSummary, 0, len(catalog))
	for _, e := range catalog {
		out = append(out, ToolSummary{Name: e.Name, Cat: e.Cat, In: e.In, Out: e.Out, Desc: e.Desc, Def: e.Def})
	}
	return out
}

// MermaidForWorkflow returns semantic Mermaid for a JSON workflow or a .mmd
// chart. A .mmd file is parsed and re-rendered so the preview reflects the graph
// termaid would actually run (and surfaces structural problems); if it cannot be
// parsed, its raw text is returned as a best-effort preview.
func MermaidForWorkflow(path string) (string, error) {
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, ".mmd") || strings.HasSuffix(lower, ".mermaid") {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		g, perr := graph.ParseMermaid(string(data))
		if perr != nil {
			return string(data), nil
		}
		return g.ToMermaid(), nil
	}
	dag, err := LoadWorkflowV3(path)
	if err != nil {
		return "", err
	}
	return dag.ToMermaid(), nil
}

// ValidateWorkflow validates matrix layout, dependency acyclicity, node kinds,
// and typed artifact contracts.
func ValidateWorkflow(path string) (*graph.DAG, error) {
	dag, err := LoadWorkflowAny(path)
	if err != nil {
		return nil, err
	}
	return dag, dag.Validate()
}

// RunHeadless preserves the original API with conservative v3 defaults.
func RunHeadless(ctx context.Context, path, domain, workdir string, concurrency int, w io.Writer) error {
	return RunHeadlessWithOptions(ctx, path, domain, workdir, concurrency, HeadlessRunOptions{}, w)
}

// RunHeadlessWithOptions executes the dependency DAG and streams status lines.
func RunHeadlessWithOptions(ctx context.Context, path, domain, workdir string, concurrency int, opts HeadlessRunOptions, w io.Writer) error {
	if strings.TrimSpace(domain) == "" {
		return fmt.Errorf("domain must not be empty")
	}
	if concurrency < 1 {
		concurrency = 1
	}
	dag, err := LoadWorkflowAny(path)
	if err != nil {
		return fmt.Errorf("load workflow %q: %w", path, err)
	}
	if err := dag.Validate(); err != nil {
		return fmt.Errorf("validate workflow %q: %w", path, err)
	}

	runnable := 0
	for id := range dag.Nodes {
		if id != dag.Root {
			runnable++
		}
	}
	if runnable == 0 {
		return fmt.Errorf("workflow %q contains no runnable nodes", path)
	}

	fmt.Fprintf(w, "▶ running %q against %s (%d nodes, concurrency %d, DAG scheduler)\n", path, domain, runnable, concurrency)
	if opts.ResumeRunID != "" {
		fmt.Fprintf(w, "  resuming checkpoint %s\n", opts.ResumeRunID)
	}

	ch := make(chan pipeline.Status, 128)
	errCh := make(chan error, 1)
	go func() {
		errCh <- pipeline.RunDAG(ctx, domain, workdir, dag, pipeline.RunConfig{
			Concurrency:    concurrency,
			ResumeRunID:    opts.ResumeRunID,
			Approvals:      opts.Approvals,
			AllowIntrusive: opts.ApproveIntrusive,
		}, ch)
		close(ch)
	}()

	var failures, skipped int
	for st := range ch {
		if st.Type == pipeline.StatusError {
			failures++
		}
		if st.Type == pipeline.StatusSkip {
			skipped++
		}
		fmt.Fprintf(w, "  [%s] %-24s %s\n", st.Category, st.Tool, headlessStatus(st))
	}
	if err := <-errCh; err != nil {
		return err
	}
	fmt.Fprintf(w, "✔ finished: %d node(s), %d tool error(s), %d skipped. Results in %s/\n", runnable, failures, skipped, workdir)
	return nil
}

func headlessStatus(s pipeline.Status) string {
	switch s.Type {
	case pipeline.StatusStart:
		return "started"
	case pipeline.StatusFinish:
		return "done"
	case pipeline.StatusSkip:
		if s.Err != nil {
			return "skipped: " + s.Err.Error()
		}
		return "skipped"
	case pipeline.StatusError:
		if s.Err != nil {
			return "error: " + s.Err.Error()
		}
		return "error"
	default:
		return "?"
	}
}
