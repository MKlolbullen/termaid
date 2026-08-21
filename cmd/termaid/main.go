// Command termaid is a terminal-native automation framework for recon and bug
// bounty workflows. Run it with no arguments to launch the interactive TUI, or
// use a subcommand (run, preview, tools, validate) to drive it from scripts
// and CI.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MKlolbullen/termaid/internal/tui"
)

const version = "1.2.0"

func main() {
	if len(os.Args) < 2 {
		runTUI()
		return
	}

	switch os.Args[1] {
	case "run":
		cmdRun(os.Args[2:])
	case "preview":
		cmdPreview(os.Args[2:])
	case "tools":
		cmdTools(os.Args[2:])
	case "validate":
		cmdValidate(os.Args[2:])
	case "tui":
		runTUI()
	case "version", "-v", "--version":
		fmt.Println("termaid " + version)
	case "help", "-h", "--help":
		usage(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "termaid: unknown command %q\n\n", os.Args[1])
		usage(os.Stderr)
		os.Exit(2)
	}
}

func runTUI() {
	prog := tea.NewProgram(
		tui.NewMenu(),
		tea.WithAltScreen(),
		tea.WithMouseAllMotion(),
	)
	if _, err := prog.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "termaid:", err)
		os.Exit(1)
	}
}

func cmdRun(argv []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	wf := fs.String("w", "workflow.json", "workflow JSON or Mermaid .mmd file to execute")
	domain := fs.String("d", "", "target domain (required)")
	workdir := fs.String("o", "workdir", "output/working directory")
	conc := fs.Int("c", 6, "maximum concurrent workflow nodes")
	resume := fs.String("resume", "", "resume a prior run ID from its checkpoint")
	approveIntrusive := fs.Bool("approve-intrusive", false, "explicitly authorize nodes marked intrusive")
	approve := fs.String("approve", "", "comma-separated approval gate/node IDs")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: termaid run -d <domain> [-w workflow.json] [-o workdir] [-c 6] [--resume run-id] [--approve gate]")
		fs.PrintDefaults()
	}
	_ = fs.Parse(argv)

	if *domain == "" {
		fmt.Fprintln(os.Stderr, "termaid run: -d <domain> is required")
		fs.Usage()
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := tui.RunHeadlessWithOptions(ctx, *wf, *domain, *workdir, *conc, tui.HeadlessRunOptions{
		ResumeRunID:      *resume,
		ApproveIntrusive: *approveIntrusive,
		Approvals:        parseApprovalList(*approve),
	}, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "termaid run:", err)
		os.Exit(1)
	}
}

func cmdPreview(argv []string) {
	fs := flag.NewFlagSet("preview", flag.ExitOnError)
	wf := fs.String("w", "workflow.json", "workflow JSON or .mmd file")
	_ = fs.Parse(argv)

	mmd, err := tui.MermaidForWorkflow(*wf)
	if err != nil {
		fmt.Fprintln(os.Stderr, "termaid preview:", err)
		os.Exit(1)
	}
	fmt.Print(mmd)
	if len(mmd) > 0 && mmd[len(mmd)-1] != '\n' {
		fmt.Println()
	}
}

func cmdTools(argv []string) {
	fs := flag.NewFlagSet("tools", flag.ExitOnError)
	cat := fs.String("cat", "", "filter by category")
	_ = fs.Parse(argv)

	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tCATEGORY\tIN\tOUT\tDESCRIPTION")
	count := 0
	for _, t := range tui.CatalogInfo() {
		if *cat != "" && t.Cat != *cat {
			continue
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", t.Name, t.Cat, dash(t.In), dash(t.Out), t.Desc)
		count++
	}
	tw.Flush()
	fmt.Printf("\n%d tool(s)\n", count)
}

func cmdValidate(argv []string) {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	wf := fs.String("w", "workflow.json", "workflow JSON or Mermaid .mmd file to validate")
	_ = fs.Parse(argv)

	dag, err := tui.ValidateWorkflow(*wf)
	if err != nil {
		fmt.Fprintln(os.Stderr, "termaid validate:", err)
		os.Exit(1)
	}
	fmt.Printf("✔ %s is valid: %d node(s), %d layer(s), %d subgraph(s), %d semantic edge(s)\n",
		*wf, len(dag.Nodes)-1, dag.MaxX, len(dag.Subgraphs), len(dag.Edges))
}

func parseApprovalList(raw string) map[string]bool {
	out := make(map[string]bool)
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			out[item] = true
		}
	}
	return out
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func usage(w *os.File) {
	fmt.Fprintln(w, `termaid `+version+` — terminal-native recon automation

Usage:
  termaid                                  launch the interactive TUI
  termaid run -d <domain> [-w f]           execute a dependency DAG
  termaid preview [-w f]                   print semantic Mermaid
  termaid tools [-cat category]            list the tool catalog
  termaid validate [-w f]                  validate graph + artifact contracts
  termaid version                          print the version
  termaid help                             show this help

DAG execution controls:
  --resume <run-id>                        resume from workdir/<run-id>/checkpoint.json
  --approve <gate,node,...>                approve named workflow gates/nodes
  --approve-intrusive                      authorize nodes marked intrusive

Run "termaid <command> -h" for command-specific flags.`)
}
