package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/MKlolbullen/termaid/internal/graph"
	"github.com/MKlolbullen/termaid/internal/tui"
)

func main() {
	fmt.Println("🔧 Termaid Matrix System Demo")
	fmt.Println("============================")
	fmt.Println()

	// Create a sample matrix-based workflow
	dag := createSampleWorkflow()

	// Display matrix structure
	fmt.Println("📊 Matrix Structure:")
	fmt.Printf("   Dimensions: %dx%d (layers x positions)\n", dag.MaxX+1, dag.MaxY+1)
	fmt.Println()

	// Show layer-by-layer breakdown
	fmt.Println("🗂️  Layer Breakdown:")
	for layer := 0; layer <= dag.MaxX; layer++ {
		layerMatrix := dag.GetLayerMatrix(layer)
		if len(layerMatrix) == 0 {
			continue
		}

		fmt.Printf("   L%d: ", layer)
		hasNodes := false
		for pos := 0; pos <= dag.MaxY; pos++ {
			if nodes, exists := layerMatrix[pos]; exists {
				if hasNodes {
					fmt.Print(" | ")
				}
				fmt.Printf("P%d[", pos)
				for i, node := range nodes {
					fmt.Print(node.ID)
					if node.Parallel {
						fmt.Print("∥")
					}
					if i < len(nodes)-1 {
						fmt.Print(",")
					}
				}
				fmt.Print("]")
				hasNodes = true
			}
		}
		if !hasNodes {
			fmt.Print("(empty)")
		}
		fmt.Println()
	}
	fmt.Println()

	// Show execution order
	fmt.Println("⚡ Execution Order:")
	executionOrder := dag.GetExecutionOrder()
	for stepNum, group := range executionOrder {
		fmt.Printf("   Step %d: ", stepNum+1)
		if len(group) == 1 {
			fmt.Printf("[%s] (sequential)\n", group[0])
		} else {
			fmt.Printf("[%s] (parallel)\n", strings.Join(group, ", "))
		}
	}
	fmt.Println()

	// Show subgraphs
	if len(dag.Subgraphs) > 0 {
		fmt.Println("🔗 Subgraphs:")
		for _, sg := range dag.Subgraphs {
			parallelStatus := "sequential"
			if sg.Parallel {
				parallelStatus = "parallel"
			}
			fmt.Printf("   %s (%s): [%s]\n", sg.Name, parallelStatus, strings.Join(sg.Nodes, ", "))
		}
		fmt.Println()
	}

	// Show compact Mermaid
	fmt.Println("📈 Mermaid Diagram (Left-to-Right):")
	mermaid := dag.ToCompactMermaid()
	for _, line := range strings.Split(mermaid, "\n") {
		if strings.TrimSpace(line) != "" {
			fmt.Printf("   %s\n", line)
		}
	}
	fmt.Println()

	// Test workflow loading
	fmt.Println("📁 Testing Workflow Loading:")
	if _, err := os.Stat("workflow.json"); err == nil {
		loadedDAG, err := tui.LoadWorkflow("workflow.json")
		if err != nil {
			fmt.Printf("   ❌ Error loading workflow.json: %v\n", err)
		} else {
			fmt.Printf("   ✅ Successfully loaded workflow.json\n")
			fmt.Printf("   📏 Matrix: %dx%d\n", loadedDAG.MaxX+1, loadedDAG.MaxY+1)
			fmt.Printf("   🔧 Tools: %d\n", len(loadedDAG.Nodes)-1) // -1 for input node
			if len(loadedDAG.Subgraphs) > 0 {
				fmt.Printf("   🔗 Subgraphs: %d\n", len(loadedDAG.Subgraphs))
			}
		}
	} else {
		fmt.Println("   ⚠️  No workflow.json found")
	}
	fmt.Println()

	// Show validation
	fmt.Println("✅ Matrix Validation:")
	if err := dag.ValidateMatrix(); err != nil {
		fmt.Printf("   ❌ Validation failed: %v\n", err)
	} else {
		fmt.Println("   ✅ Matrix is valid")
	}
	fmt.Println()

	fmt.Println("🚀 Demo Complete! Matrix system is working correctly.")
	fmt.Println()
	fmt.Println("💡 Key Features Demonstrated:")
	fmt.Println("   • 2D Matrix positioning [X=layer, Y=position]")
	fmt.Println("   • Parallel execution groups with ∥ indicator")
	fmt.Println("   • Subgraph organization for logical grouping")
	fmt.Println("   • Left-to-right Mermaid visualization")
	fmt.Println("   • Execution order optimization")
	fmt.Println("   • Matrix validation and consistency checks")
}

func createSampleWorkflow() *graph.DAG {
	dag := graph.NewDAG()

	// Layer 1: Parallel subdomain enumeration
	dag.AddNodeAtPosition("input", "subfinder-1", "subfinder", "-d {{domain}} -silent -o {{output}}", 1, 0, "enum", true)
	dag.AddNodeAtPosition("input", "assetfinder-1", "assetfinder", "--subs-only {{domain}} > {{output}}", 1, 1, "enum", true)
	dag.AddNodeAtPosition("input", "amass-1", "amass", "enum -passive -d {{domain}} -o {{output}}", 1, 2, "enum", true)

	// Layer 2: DNS resolution (sequential)
	dag.AddNodeAtPosition("subfinder-1", "dnsx-1", "dnsx", "-l {{input}} -resp -a -silent -o {{output}}", 2, 0, "", false)
	dag.Nodes["assetfinder-1"].Children = append(dag.Nodes["assetfinder-1"].Children, "dnsx-1")
	dag.Nodes["amass-1"].Children = append(dag.Nodes["amass-1"].Children, "dnsx-1")

	// Layer 3: Web probing
	dag.AddNodeAtPosition("dnsx-1", "httpx-1", "httpx", "-l {{input}} -title -tech-detect -silent -o {{output}}", 3, 0, "", false)

	// Layer 4: Parallel vulnerability scanning
	dag.AddNodeAtPosition("httpx-1", "nuclei-1", "nuclei", "-l {{input}} -severity high,critical -silent -o {{output}}", 4, 0, "scan", true)
	dag.AddNodeAtPosition("httpx-1", "dalfox-1", "dalfox", "file {{input}} --skip-bav -o {{output}}", 4, 1, "scan", true)

	// Create subgraphs
	dag.Subgraphs["enum"] = &graph.SubgraphInfo{
		ID:       "enum",
		Name:     "Parallel Subdomain Enumeration",
		Parallel: true,
		Nodes:    []string{"subfinder-1", "assetfinder-1", "amass-1"},
		Matrix:   make(map[string]graph.Coordinate),
	}

	dag.Subgraphs["scan"] = &graph.SubgraphInfo{
		ID:       "scan",
		Name:     "Parallel Vulnerability Scanning",
		Parallel: true,
		Nodes:    []string{"nuclei-1", "dalfox-1"},
		Matrix:   make(map[string]graph.Coordinate),
	}

	return dag
}