package main

import (
	"fmt"
	"strings"

	"github.com/MKlolbullen/termaid/internal/tui"
)

func main() {
	fmt.Println("🔧 Termaid Responsive Design Demo")
	fmt.Println("=================================")
	fmt.Println()

	rm := tui.NewResponsiveManager()

	// Test different screen sizes
	testSizes := []struct {
		name   string
		width  int
		height int
		desc   string
	}{
		{"Tiny", 70, 20, "Small terminal/mobile"},
		{"Small", 100, 28, "Standard 80x24+ terminal"},
		{"Medium", 140, 35, "Modern terminal"},
		{"Large", 180, 45, "Wide monitor terminal"},
		{"XLarge", 220, 55, "Ultra-wide/4K terminal"},
	}

	for _, size := range testSizes {
		fmt.Printf("📏 %s Screen (%dx%d) - %s\n", size.name, size.width, size.height, size.desc)
		fmt.Println(strings.Repeat("─", 60))

		layout := rm.CalculateLayout(size.width, size.height)

		// Show layout breakdown
		fmt.Printf("Screen Size Category: %v\n", getScreenSizeName(layout.ScreenSize))
		fmt.Printf("Tools Panel:   %dx%d (%.1f%% width, %.1f%% height)\n",
			layout.ToolsWidth, layout.ToolsHeight,
			float64(layout.ToolsWidth)/float64(size.width)*100,
			float64(layout.ToolsHeight)/float64(size.height)*100)
		fmt.Printf("Help Panel:    %dx%d\n", layout.HelpWidth, layout.HelpHeight)
		fmt.Printf("Input Panel:   %dx%d\n", layout.InputWidth, layout.InputHeight)
		fmt.Printf("Visual Panel:  %dx%d (%.1f%% width, %.1f%% height)\n",
			layout.VisualWidth, layout.VisualHeight,
			float64(layout.VisualWidth)/float64(size.width)*100,
			float64(layout.VisualHeight)/float64(size.height)*100)

		// Show configuration details
		config := layout.Config
		fmt.Printf("\nConfiguration:\n")
		fmt.Printf("  Compact Mode:         %t\n", config.CompactMode)
		fmt.Printf("  Show Detailed Help:   %t\n", config.ShowDetailedHelp)
		fmt.Printf("  Show Matrix Grid:     %t\n", config.ShowMatrixGrid)
		fmt.Printf("  Show Subgraph Info:   %t\n", config.ShowSubgraphDetails)
		fmt.Printf("  Max Tools Visible:    %d\n", config.MaxToolsEntries)
		fmt.Printf("  Max Mermaid Lines:    %d\n", config.MaxMermaidLines)
		fmt.Printf("  Vertical Scrolling:   %t\n", config.UseVerticalScroll)
		fmt.Printf("  Horizontal Scrolling: %t\n", config.UseHorizontalScroll)

		// Visual layout representation
		fmt.Printf("\nLayout Visualization:\n")
		renderLayoutPreview(layout, size.width, size.height)

		fmt.Println()
		fmt.Println()
	}

	// Show responsive breakpoints
	fmt.Println("📊 Responsive Breakpoints")
	fmt.Println("=========================")
	fmt.Printf("Tiny:    < 80x24   (Minimal UI, compact mode)\n")
	fmt.Printf("Small:   80x24 to 120x30   (Basic UI, limited features)\n")
	fmt.Printf("Medium:  120x30 to 160x40  (Standard UI, full features)\n")
	fmt.Printf("Large:   160x40 to 200x50  (Enhanced UI, more space)\n")
	fmt.Printf("XLarge:  > 200x50  (Full UI, maximum features)\n")
	fmt.Println()

	// Show adaptive features
	fmt.Println("🎯 Adaptive Features")
	fmt.Println("===================")
	fmt.Println("✓ Panel size percentages adjust based on screen size")
	fmt.Println("✓ Minimum panel sizes prevent UI from becoming unusable")
	fmt.Println("✓ Tool list and content scrolling for large datasets")
	fmt.Println("✓ Compact vs detailed help text based on available space")
	fmt.Println("✓ Matrix grid complexity adapts to screen real estate")
	fmt.Println("✓ Mermaid preview line count scales with panel height")
	fmt.Println("✓ Input field widths adjust to prevent text overflow")
	fmt.Println("✓ Styling adapts (borders, colors, emphasis) per screen size")
	fmt.Println()

	fmt.Println("🚀 Benefits of Responsive Design")
	fmt.Println("================================")
	fmt.Println("• Works on any terminal size from 70x20 to 240x60+")
	fmt.Println("• Optimal UX on standard 1920x1080 displays")
	fmt.Println("• Efficient use of space on ultra-wide monitors")
	fmt.Println("• Graceful degradation on small terminals")
	fmt.Println("• Scrolling prevents content from being cut off")
	fmt.Println("• Professional appearance at any resolution")
}

func getScreenSizeName(screenSize tui.ScreenSize) string {
	switch screenSize {
	case 0:
		return "Tiny"
	case 1:
		return "Small"
	case 2:
		return "Medium"
	case 3:
		return "Large"
	case 4:
		return "XLarge"
	default:
		return "Unknown"
	}
}

func renderLayoutPreview(layout tui.LayoutDimensions, totalW, totalH int) {
	// Create a simple ASCII representation of the layout
	fmt.Printf("┌%s┬%s┐\n",
		strings.Repeat("─", layout.ToolsWidth/4),
		strings.Repeat("─", layout.InputWidth/4))

	fmt.Printf("│%s│%s│ ← Input (%dx%d)\n",
		centerText("Tools", layout.ToolsWidth/4),
		centerText("Input/Args", layout.InputWidth/4),
		layout.InputWidth, layout.InputHeight)

	fmt.Printf("│%s├%s┤\n",
		centerText(fmt.Sprintf("%dx%d", layout.ToolsWidth, layout.ToolsHeight), layout.ToolsWidth/4),
		strings.Repeat("─", layout.VisualWidth/4))

	fmt.Printf("├%s┤%s│\n",
		strings.Repeat("─", layout.HelpWidth/4),
		centerText("Matrix Visual", layout.VisualWidth/4))

	fmt.Printf("│%s│%s│ ← Visual (%dx%d)\n",
		centerText("Help", layout.HelpWidth/4),
		centerText(fmt.Sprintf("%dx%d", layout.VisualWidth, layout.VisualHeight), layout.VisualWidth/4),
		layout.VisualWidth, layout.VisualHeight)

	fmt.Printf("└%s┴%s┘\n",
		strings.Repeat("─", layout.HelpWidth/4),
		strings.Repeat("─", layout.VisualWidth/4))
}

func centerText(text string, width int) string {
	if len(text) >= width {
		if width > 3 {
			return text[:width-3] + "..."
		}
		return text[:width]
	}
	padding := width - len(text)
	leftPad := padding / 2
	rightPad := padding - leftPad
	return strings.Repeat(" ", leftPad) + text + strings.Repeat(" ", rightPad)
}
