package tui

import (
	"fmt"
	"github.com/charmbracelet/lipgloss"
)

// ScreenSize represents different terminal size categories
type ScreenSize int

const (
	ScreenTiny ScreenSize = iota // < 80x24 (minimum)
	ScreenSmall                  // 80x24 to 120x30
	ScreenMedium                 // 120x30 to 160x40
	ScreenLarge                  // 160x40 to 200x50
	ScreenXLarge                 // > 200x50
)

// Breakpoints define responsive design thresholds
type Breakpoints struct {
	TinyWidth    int
	SmallWidth   int
	MediumWidth  int
	LargeWidth   int
	XLargeWidth  int
	TinyHeight   int
	SmallHeight  int
	MediumHeight int
	LargeHeight  int
	XLargeHeight int
}

// DefaultBreakpoints provides standard terminal size breakpoints
var DefaultBreakpoints = Breakpoints{
	TinyWidth:    80,
	SmallWidth:   120,
	MediumWidth:  160,
	LargeWidth:   200,
	XLargeWidth:  240,
	TinyHeight:   24,
	SmallHeight:  30,
	MediumHeight: 40,
	LargeHeight:  50,
	XLargeHeight: 60,
}

// LayoutConfig defines responsive layout parameters
type LayoutConfig struct {
	ToolsPanelWidth      float64 // Percentage of screen width
	ToolsPanelHeight     float64 // Percentage of screen height
	HelpPanelHeight      float64 // Percentage of screen height
	InputPanelHeight     float64 // Percentage of screen height
	VisualPanelHeight    float64 // Percentage of screen height
	MinToolsWidth        int     // Minimum absolute width
	MinHelpHeight        int     // Minimum absolute height
	MinInputHeight       int     // Minimum absolute height
	MinVisualHeight      int     // Minimum absolute height
	MaxToolsEntries      int     // Maximum visible tool entries
	MaxMermaidLines      int     // Maximum Mermaid preview lines
	UseVerticalScroll    bool    // Enable vertical scrolling
	UseHorizontalScroll  bool    // Enable horizontal scrolling
	CompactMode          bool    // Use compact rendering
	ShowDetailedHelp     bool    // Show detailed help text
	ShowMatrixGrid       bool    // Show full matrix grid
	ShowSubgraphDetails  bool    // Show subgraph information
}

// ResponsiveManager handles adaptive layout calculations
type ResponsiveManager struct {
	breakpoints Breakpoints
	configs     map[ScreenSize]LayoutConfig
}

// NewResponsiveManager creates a new responsive design manager
func NewResponsiveManager() *ResponsiveManager {
	rm := &ResponsiveManager{
		breakpoints: DefaultBreakpoints,
		configs:     make(map[ScreenSize]LayoutConfig),
	}
	rm.initializeConfigs()
	return rm
}

// initializeConfigs sets up responsive configurations for each screen size
func (rm *ResponsiveManager) initializeConfigs() {
	// Tiny screens (< 80x24) - Minimal mode
	rm.configs[ScreenTiny] = LayoutConfig{
		ToolsPanelWidth:     0.25,
		ToolsPanelHeight:    0.70,
		HelpPanelHeight:     0.30,
		InputPanelHeight:    0.25,
		VisualPanelHeight:   0.75,
		MinToolsWidth:       15,
		MinHelpHeight:       6,
		MinInputHeight:      5,
		MinVisualHeight:     10,
		MaxToolsEntries:     8,
		MaxMermaidLines:     5,
		UseVerticalScroll:   true,
		UseHorizontalScroll: false,
		CompactMode:         true,
		ShowDetailedHelp:    false,
		ShowMatrixGrid:      false,
		ShowSubgraphDetails: false,
	}

	// Small screens (80x24 to 120x30) - Compact mode
	rm.configs[ScreenSmall] = LayoutConfig{
		ToolsPanelWidth:     0.22,
		ToolsPanelHeight:    0.75,
		HelpPanelHeight:     0.25,
		InputPanelHeight:    0.20,
		VisualPanelHeight:   0.80,
		MinToolsWidth:       18,
		MinHelpHeight:       6,
		MinInputHeight:      5,
		MinVisualHeight:     15,
		MaxToolsEntries:     12,
		MaxMermaidLines:     8,
		UseVerticalScroll:   true,
		UseHorizontalScroll: false,
		CompactMode:         true,
		ShowDetailedHelp:    false,
		ShowMatrixGrid:      true,
		ShowSubgraphDetails: false,
	}

	// Medium screens (120x30 to 160x40) - Standard mode
	rm.configs[ScreenMedium] = LayoutConfig{
		ToolsPanelWidth:     0.20,
		ToolsPanelHeight:    0.80,
		HelpPanelHeight:     0.20,
		InputPanelHeight:    0.18,
		VisualPanelHeight:   0.82,
		MinToolsWidth:       20,
		MinHelpHeight:       7,
		MinInputHeight:      6,
		MinVisualHeight:     20,
		MaxToolsEntries:     15,
		MaxMermaidLines:     12,
		UseVerticalScroll:   true,
		UseHorizontalScroll: true,
		CompactMode:         false,
		ShowDetailedHelp:    true,
		ShowMatrixGrid:      true,
		ShowSubgraphDetails: true,
	}

	// Large screens (160x40 to 200x50) - Enhanced mode
	rm.configs[ScreenLarge] = LayoutConfig{
		ToolsPanelWidth:     0.18,
		ToolsPanelHeight:    0.82,
		HelpPanelHeight:     0.18,
		InputPanelHeight:    0.16,
		VisualPanelHeight:   0.84,
		MinToolsWidth:       25,
		MinHelpHeight:       8,
		MinInputHeight:      7,
		MinVisualHeight:     25,
		MaxToolsEntries:     20,
		MaxMermaidLines:     15,
		UseVerticalScroll:   true,
		UseHorizontalScroll: true,
		CompactMode:         false,
		ShowDetailedHelp:    true,
		ShowMatrixGrid:      true,
		ShowSubgraphDetails: true,
	}

	// XLarge screens (> 200x50) - Full-featured mode
	rm.configs[ScreenXLarge] = LayoutConfig{
		ToolsPanelWidth:     0.15,
		ToolsPanelHeight:    0.85,
		HelpPanelHeight:     0.15,
		InputPanelHeight:    0.15,
		VisualPanelHeight:   0.85,
		MinToolsWidth:       30,
		MinHelpHeight:       10,
		MinInputHeight:      8,
		MinVisualHeight:     30,
		MaxToolsEntries:     25,
		MaxMermaidLines:     20,
		UseVerticalScroll:   true,
		UseHorizontalScroll: true,
		CompactMode:         false,
		ShowDetailedHelp:    true,
		ShowMatrixGrid:      true,
		ShowSubgraphDetails: true,
	}
}

// DetectScreenSize determines the current screen size category
func (rm *ResponsiveManager) DetectScreenSize(width, height int) ScreenSize {
	if width < rm.breakpoints.TinyWidth || height < rm.breakpoints.TinyHeight {
		return ScreenTiny
	}
	if width < rm.breakpoints.SmallWidth || height < rm.breakpoints.SmallHeight {
		return ScreenSmall
	}
	if width < rm.breakpoints.MediumWidth || height < rm.breakpoints.MediumHeight {
		return ScreenMedium
	}
	if width < rm.breakpoints.LargeWidth || height < rm.breakpoints.LargeHeight {
		return ScreenLarge
	}
	return ScreenXLarge
}

// GetLayoutConfig returns the appropriate layout configuration
func (rm *ResponsiveManager) GetLayoutConfig(width, height int) LayoutConfig {
	screenSize := rm.DetectScreenSize(width, height)
	return rm.configs[screenSize]
}

// CalculateLayout computes actual pixel dimensions for layout
func (rm *ResponsiveManager) CalculateLayout(width, height int) LayoutDimensions {
	config := rm.GetLayoutConfig(width, height)
	
	// Calculate panel dimensions
	toolsWidth := max(int(float64(width)*config.ToolsPanelWidth), config.MinToolsWidth)
	inputWidth := width - toolsWidth
	
	helpHeight := max(int(float64(height)*config.HelpPanelHeight), config.MinHelpHeight)
	inputHeight := max(int(float64(height)*config.InputPanelHeight), config.MinInputHeight)
	visualHeight := height - inputHeight
	toolsHeight := height - helpHeight
	
	return LayoutDimensions{
		ToolsWidth:    toolsWidth,
		ToolsHeight:   toolsHeight,
		HelpWidth:     toolsWidth,
		HelpHeight:    helpHeight,
		InputWidth:    inputWidth,
		InputHeight:   inputHeight,
		VisualWidth:   inputWidth,
		VisualHeight:  visualHeight,
		Config:        config,
		ScreenSize:    rm.DetectScreenSize(width, height),
	}
}

// LayoutDimensions contains calculated layout dimensions
type LayoutDimensions struct {
	ToolsWidth   int
	ToolsHeight  int
	HelpWidth    int
	HelpHeight   int
	InputWidth   int
	InputHeight  int
	VisualWidth  int
	VisualHeight int
	Config       LayoutConfig
	ScreenSize   ScreenSize
}

// ScrollState manages scrolling state for panels
type ScrollState struct {
	ToolsOffset      int
	VisualOffsetX    int
	VisualOffsetY    int
	MermaidOffset    int
	MatrixOffsetX    int
	MatrixOffsetY    int
	MaxToolsOffset   int
	MaxVisualOffsetX int
	MaxVisualOffsetY int
}

// ScrollManager handles scrolling operations
type ScrollManager struct {
	state ScrollState
}

// NewScrollManager creates a new scroll manager
func NewScrollManager() *ScrollManager {
	return &ScrollManager{
		state: ScrollState{},
	}
}

// UpdateBounds updates scrolling boundaries based on content size
func (sm *ScrollManager) UpdateBounds(toolsCount, matrixWidth, matrixHeight, mermaidLines int, layout LayoutDimensions) {
	sm.state.MaxToolsOffset = max(0, toolsCount-layout.Config.MaxToolsEntries)
	sm.state.MaxVisualOffsetX = max(0, matrixWidth-layout.VisualWidth/8) // Rough character width
	sm.state.MaxVisualOffsetY = max(0, matrixHeight-layout.VisualHeight/2) // Rough line height
}

// ScrollUp moves content up (decrease offset)
func (sm *ScrollManager) ScrollUp(area string, amount int) {
	switch area {
	case "tools":
		sm.state.ToolsOffset = max(0, sm.state.ToolsOffset-amount)
	case "visual_y":
		sm.state.VisualOffsetY = max(0, sm.state.VisualOffsetY-amount)
	case "mermaid":
		sm.state.MermaidOffset = max(0, sm.state.MermaidOffset-amount)
	}
}

// ScrollDown moves content down (increase offset)
func (sm *ScrollManager) ScrollDown(area string, amount int) {
	switch area {
	case "tools":
		sm.state.ToolsOffset = min(sm.state.MaxToolsOffset, sm.state.ToolsOffset+amount)
	case "visual_y":
		sm.state.VisualOffsetY = min(sm.state.MaxVisualOffsetY, sm.state.VisualOffsetY+amount)
	case "mermaid":
		// Mermaid scrolling handled separately
	}
}

// ScrollLeft moves content left (decrease X offset)
func (sm *ScrollManager) ScrollLeft(area string, amount int) {
	switch area {
	case "visual_x":
		sm.state.VisualOffsetX = max(0, sm.state.VisualOffsetX-amount)
	case "matrix_x":
		sm.state.MatrixOffsetX = max(0, sm.state.MatrixOffsetX-amount)
	}
}

// ScrollRight moves content right (increase X offset)
func (sm *ScrollManager) ScrollRight(area string, amount int) {
	switch area {
	case "visual_x":
		sm.state.VisualOffsetX = min(sm.state.MaxVisualOffsetX, sm.state.VisualOffsetX+amount)
	case "matrix_x":
		// Matrix X scrolling
	}
}

// GetState returns current scroll state
func (sm *ScrollManager) GetState() ScrollState {
	return sm.state
}

// StyleAdaptive creates adaptive styles based on screen size
func StyleAdaptive(screenSize ScreenSize) AdaptiveStyles {
	base := lipgloss.NewStyle()
	
	switch screenSize {
	case ScreenTiny:
		return AdaptiveStyles{
			Border:     base.Border(lipgloss.NormalBorder()),
			Title:      base.Bold(false),
			Highlight:  base.Foreground(lipgloss.Color("12")),
			Muted:      base.Foreground(lipgloss.Color("8")),
			Error:      base.Foreground(lipgloss.Color("9")),
			Success:    base.Foreground(lipgloss.Color("10")),
			Padding:    0,
			Margin:     0,
		}
	case ScreenSmall:
		return AdaptiveStyles{
			Border:     base.Border(lipgloss.RoundedBorder()),
			Title:      base.Bold(true),
			Highlight:  base.Foreground(lipgloss.Color("14")),
			Muted:      base.Foreground(lipgloss.Color("8")),
			Error:      base.Foreground(lipgloss.Color("9")),
			Success:    base.Foreground(lipgloss.Color("10")),
			Padding:    1,
			Margin:     0,
		}
	default:
		return AdaptiveStyles{
			Border:     base.Border(lipgloss.RoundedBorder()),
			Title:      base.Bold(true).Underline(true),
			Highlight:  base.Foreground(lipgloss.Color("14")).Bold(true),
			Muted:      base.Foreground(lipgloss.Color("8")),
			Error:      base.Foreground(lipgloss.Color("9")).Bold(true),
			Success:    base.Foreground(lipgloss.Color("10")).Bold(true),
			Padding:    1,
			Margin:     1,
		}
	}
}

// AdaptiveStyles contains responsive styling options
type AdaptiveStyles struct {
	Border    lipgloss.Style
	Title     lipgloss.Style
	Highlight lipgloss.Style
	Muted     lipgloss.Style
	Error     lipgloss.Style
	Success   lipgloss.Style
	Padding   int
	Margin    int
}

// Utility functions are defined in builder.go to avoid redeclaration

// ViewportManager handles content that exceeds available space
type ViewportManager struct {
	width       int
	height      int
	contentW    int
	contentH    int
	offsetX     int
	offsetY     int
	showScrollX bool
	showScrollY bool
}

// NewViewportManager creates a new viewport manager
func NewViewportManager(width, height int) *ViewportManager {
	return &ViewportManager{
		width:  width,
		height: height,
	}
}

// SetContent updates the content dimensions
func (vm *ViewportManager) SetContent(contentWidth, contentHeight int) {
	vm.contentW = contentWidth
	vm.contentH = contentHeight
	vm.showScrollX = contentWidth > vm.width
	vm.showScrollY = contentHeight > vm.height
}

// GetVisibleBounds returns the visible content bounds
func (vm *ViewportManager) GetVisibleBounds() (startX, endX, startY, endY int) {
	startX = vm.offsetX
	endX = min(vm.contentW, vm.offsetX+vm.width)
	startY = vm.offsetY
	endY = min(vm.contentH, vm.offsetY+vm.height)
	return
}

// Scroll adjusts the viewport offset
func (vm *ViewportManager) Scroll(deltaX, deltaY int) {
	vm.offsetX = max(0, min(vm.contentW-vm.width, vm.offsetX+deltaX))
	vm.offsetY = max(0, min(vm.contentH-vm.height, vm.offsetY+deltaY))
}

// GetScrollIndicators returns scroll indicator strings
func (vm *ViewportManager) GetScrollIndicators() (horizontal, vertical string) {
	if vm.showScrollX {
		progress := float64(vm.offsetX) / float64(vm.contentW-vm.width)
		horizontal = fmt.Sprintf("◄─%.0f%%─►", progress*100)
	}
	if vm.showScrollY {
		progress := float64(vm.offsetY) / float64(vm.contentH-vm.height)
		vertical = fmt.Sprintf("▲\n%.0f%%\n▼", progress*100)
	}
	return
}