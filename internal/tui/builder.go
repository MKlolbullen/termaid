package tui

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"

	"github.com/MKlolbullen/termaid/internal/graph"
)

/*──────────────────────── styles ───────────────────────*/

var (
	borderAct   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("10"))
	borderInact = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("8"))

	btnStyle = lipgloss.NewStyle().Padding(0, 1).Bold(true)
	btnRun   = btnStyle.Background(lipgloss.Color("#005f00")).Foreground(lipgloss.Color("15"))
	btnPause = btnStyle.Background(lipgloss.Color("#5f5f00")).Foreground(lipgloss.Color("15"))
	btnStop  = btnStyle.Background(lipgloss.Color("#5f0000")).Foreground(lipgloss.Color("15"))
	btnGrey  = btnStyle.Background(lipgloss.Color("#444")).Foreground(lipgloss.Color("230"))

	btnSel = lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Bold(true)

	pickedCell  = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("13"))
	blockedCell = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("9"))
	activeCell  = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("6"))
	inactiveCell = lipgloss.NewStyle().Border(lipgloss.HiddenBorder())
)

/*──────────────────────── focus enum ───────────────────*/

type focusArea int

const (
	fHeader focusArea = iota
	fDomain
	fList
	fCanvas
	fArgs
)

/*──────────────────────── Builder model ────────────────*/

type BuilderModel struct {
	/* header buttons */
	btns   []string
	btnIdx int

	domainInp textinput.Model
	toolSel   list.Model
	argsInp   textinput.Model
	canvas    viewport.Model

	filterMode bool
	filterBox  textinput.Model

	moveMode bool
	pickID   string

	g   *graph.DAG
	occ map[string]int

	focus focusArea
	curY  int // matrix cursor
	curX  int
	msg   string
}

/*──────────────────────── init ─────────────────────────*/

func NewBuilder(toolNames []string) BuilderModel {
	/* header buttons */
	btns := []string{
		btnRun.Render("▶ Run"),
		btnPause.Render("⏸ Pause"),
		btnStop.Render("■ Stop"),
		btnGrey.Render("💾 Save"),
		btnGrey.Render("📂 Load"),
	}

	/* domain */
	dom := textinput.New()
	dom.Placeholder = "example.com"

	/* tool list with separators */
	items := buildToolItems(toolNames)
	toolList := list.New(items, toolDelegate{}, 45, 16)
	toolList.Title = "Tools ( / = filter )"

	/* args */
	arg := textinput.New()
	arg.Placeholder = "args"
	arg.Width = 40

	filter := textinput.New()
	filter.Placeholder = "category…"

	cv := viewport.New(50, 16)
	cv.YPosition = 1
	cv.SetContent("")

	return BuilderModel{
		btns:       btns,
		btnIdx:     0,
		domainInp:  dom,
		toolSel:    toolList,
		argsInp:    arg,
		canvas:     cv,
		filterBox:  filter,
		g:          graph.NewDAG(),
		occ:        make(map[string]int),
		focus:      fHeader,
		curY:       0,
		curX:       0,
		moveMode:   false,
	}
}

func (m BuilderModel) Init() tea.Cmd { return nil }

/*──────────────────────── Update ───────────────────────*/

func (m BuilderModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {

	/* ───── mouse ───────────────────────*/
	case tea.MouseMsg:
		if v.Button == tea.MouseButtonLeft && v.Type == tea.MouseButtonPress {
			switch {
			case hitHeader(v):
				m.focus = fHeader
				m.btnIdx = headerIndex(v)
			case hitList(v):
				m.focus = fList
				row := listRow(v)
				m.toolSel.Select(row)
			case hitCanvas(v):
				m.focus = fCanvas
				x, y := canvasCoord(v, m.canvas)
				m.curX, m.curY = x, y
				if id := idAtCursor(m); id != "" {
					m.selNode = id
				}
			case hitArgs(v):
				m.focus = fArgs
			}
		}
		/* scroll wheel pans the canvas */
		if m.focus == fCanvas {
			if v.Type == tea.MouseWheelDown {
				m.canvas.LineDown(3)
			}
			if v.Type == tea.MouseWheelUp {
				m.canvas.LineUp(3)
			}
		}

	/* ───── keyboard ────────────────────*/
	case tea.KeyMsg:
		if m.handleKeys(v) {
			// handled
		}
	}

	/* delegate to subcomponents */
	if m.focus == fDomain {
		m.domainInp, _ = m.domainInp.Update(msg)
	}
	if m.focus == fList {
		if m.filterMode {
			m.filterBox, _ = m.filterBox.Update(msg)
		} else {
			m.toolSel, _ = m.toolSel.Update(msg)
		}
	}
	if m.focus == fArgs {
		m.argsInp, _ = m.argsInp.Update(msg)
	}

	/* keep canvas content fresh */
	m.canvas.SetContent(renderMatrix(m))

	return m, nil
}

func (m *BuilderModel) handleKeys(v tea.KeyMsg) bool {
	ks := v.String()

	/* ===== global quit ===== */
	if ks == "q" && !m.moveMode {
		return false // handled by higher-level model
	}

	/* ===== move mode pick/drop ===== */
	if ks == "m" && m.focus == fCanvas && !m.moveMode && m.selNode != "input" {
		m.moveMode, m.pickID = true, m.selNode
		m.msg = "pick " + m.pickID
		return true
	}
	if m.moveMode {
		switch ks {
		case "esc":
			m.moveMode = false
			m.msg = "move cancelled"
		case "enter":
			if idAtCursor(*m) == "" {
				m.moveSubtree()
				m.moveMode = false
				m.msg = "moved " + m.pickID
			} else {
				m.msg = "Cannot drop here!"
			}
		case "left", "right", "up", "down":
			m.arrowMove(ks)
		}
		return true
	}

	/* ===== pane-specific keys ===== */
	switch m.focus {

	case fHeader:
		switch ks {
		case "left":
			if m.btnIdx > 0 {
				m.btnIdx--
			}
		case "right":
			if m.btnIdx < len(m.btns)-1 {
				m.btnIdx++
			}
		case "tab":
			m.focus = fDomain
		case "enter":
			m.msg = fmt.Sprintf("clicked %s", stripAnsi(m.btns[m.btnIdx]))
		}

	case fDomain:
		if ks == "tab" {
			m.focus = fList
		}

	case fList:
		if !m.filterMode {
			switch ks {
			case "/":
				m.filterMode = true
				m.filterBox.Reset()
				m.filterBox.Focus()
			case "tab":
				m.focus = fCanvas
			}
		} else { // filtering
			switch ks {
			case "enter":
				m.applyFilter(m.filterBox.Value())
				m.filterMode = false
			case "esc":
				m.filterMode = false
			}
		}

	case fCanvas:
		switch ks {
		case "tab":
			m.focus = fArgs
		case "shift+tab":
			m.focus = fList
		case "pgup", "pgdn", "ctrl+left", "ctrl+right", "ctrl+up", "ctrl+down":
			m.zoomPan(ks)
		case "n", "r", "c":
			m.nodeOps(ks)
		case "left", "right", "up", "down":
			m.arrowMove(ks)
		}

	case fArgs:
		if ks == "shift+tab" {
			m.focus = fCanvas
		}
	}

	return true
}

/*──────────────────────── helpers: graph ops ──────────*/

func (m *BuilderModel) nodeOps(k string) {
	switch k {
	case "n":
		tool := m.toolSel.SelectedItem().(entryItem).name
		m.occ[tool]++
		id := fmt.Sprintf("%s-%d", tool, m.occ[tool])
		args := defaultArgs(tool)
		_ = m.g.AddNode(m.selNode, id, tool, args, m.curY+1)
	case "r":
		if m.selNode != "input" {
			_ = m.g.RemoveNode(m.selNode)
			m.selNode = "input"
		}
	case "c":
		if n := m.g.Nodes[m.selNode]; n != nil {
			n.Args = m.argsInp.Value()
		}
	}
}

func (m *BuilderModel) moveSubtree() {
	node := m.g.Nodes[m.pickID]
	dy := m.curY - node.Layer
	// remove from old layer list (slice)
	m.g.RemoveFromLayer(m.pickID)
	// insert placeholder so index exists
	m.g.InsertAtLayer(m.pickID, m.curY, m.curX)
	m.shiftChildren(m.pickID, dy)
}
func (m *BuilderModel) shiftChildren(id string, dy int) {
	for _, ch := range m.g.Nodes[id].Children {
		m.g.Nodes[ch].Layer += dy
		m.shiftChildren(ch, dy)
	}
}

/*──────── zoom & pan ─────────*/
func (m *BuilderModel) zoomPan(key string) {
	switch key {
	case "pgup": // zoom in
		m.canvas.Width = clamp(m.canvas.Width-10, 30, 120)
		m.canvas.Height = clamp(m.canvas.Height+3, 10, 50)
	case "pgdn": // zoom out
		m.canvas.Width = clamp(m.canvas.Width+10, 30, 120)
		m.canvas.Height = clamp(m.canvas.Height-3, 10, 50)
	case "ctrl+left":
		m.canvas.SetXOffset(m.canvas.XOffset - 5)
	case "ctrl+right":
		m.canvas.SetXOffset(m.canvas.XOffset + 5)
	case "ctrl+up":
		m.canvas.LineUp(2)
	case "ctrl+down":
		m.canvas.LineDown(2)
	}
}

func (m *BuilderModel) arrowMove(dir string) {
	switch dir {
	case "left":
		if m.curX > 0 {
			m.curX--
		}
	case "right":
		m.curX++
	case "up":
		if m.curY > 0 {
			m.curY--
		}
	case "down":
		m.curY++
	}
	id := idAtCursor(*m)
	if id != "" {
		m.selNode = id
	}
}

/*──────────────────────── renderers ───────────────────*/

func styleCell(id string, active bool) string {
	if id == "" {
		return inactiveCell.Render("     ")
	}
	st := inactiveCell
	if active {
		st = activeCell
	}
	return st.Render(id)
}

func renderMatrix(m *BuilderModel) string {
	g := m.g
	var out strings.Builder
	maxLayer := g.MaxLayer()
	for y := 0; y <= maxLayer; y++ {
		row := g.GetLayer(y)
		// header
		out.WriteString(fmt.Sprintf("L%-2d ", y))
		for x, id := range row {
			active := y == m.curY && x == m.curX
			if m.moveMode && id == m.pickID {
				out.WriteString(pickedCell.Render(id))
			} else if m.moveMode && active && id != "" {
				out.WriteString(blockedCell.Render(id))
			} else {
				out.WriteString(styleCell(id, active))
			}
		}
		if m.curY == y && m.curX >= len(row) { // cursor on empty slot
			out.WriteString(styleCell("", true))
		}
		out.WriteString("\n")
	}
	return out.String()
}

/*──────────────────────── view glue ───────────────────*/

func (m BuilderModel) View() string {
	/* header */
	var hdr string
	for i, b := range m.btns {
		if m.focus == fHeader && i == m.btnIdx {
			hdr += btnSel.Render(b)
		} else {
			hdr += b
		}
		if i < len(m.btns)-1 {
			hdr += " "
		}
	}
	hdr = borderAct.Render(hdr)

	/* domain + tool list left column */
	domain := maybeBorder("Domain: "+m.domainInp.View(), m.focus == fDomain)
	listView := m.toolSel.View()
	if m.filterMode {
		listView = m.filterBox.View()
	}
	tools := maybeBorder(listView, m.focus == fList)

	left := lipgloss.JoinVertical(lipgloss.Top, domain, tools)

	/* right column */
	right := lipgloss.JoinVertical(lipgloss.Top,
		maybeBorder(m.canvas.View(), m.focus == fCanvas),
		maybeBorder("Args: "+m.argsInp.View(), m.focus == fArgs),
	)

	help := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(
		"↑↓←→ move  n new  r rm  m pick/drop  c args  PgUp/PgDn zoom  Ctrl+arrows pan  / filter  tab pane  q quit",
	)

	return hdr + "\n" +
		lipgloss.JoinHorizontal(lipgloss.Top, left, right) +
		"\n" + help + "\n" + m.msg
}

/*──────────────────────── toolbox ─────────────────────*/

func buildToolItems(names []string) []list.Item {
	curCat := ""
	var items []list.Item
	for _, c := range catalog {
		if c.Cat != curCat {
			curCat = c.Cat
			items = append(items, list.Separator("── "+curCat+" ──"))
		}
		items = append(items, entryItem{c.Name, c.Desc})
	}
	return items
}

type toolDelegate struct{}

func (toolDelegate) Height() int    { return 1 }
func (toolDelegate) Spacing() int   { return 0 }
func (toolDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (toolDelegate) Render(w io.Writer, m list.Model, index int, itm list.Item) {
	if sep, ok := itm.(list.Separator); ok {
		fmt.Fprintln(w, lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(sep.String()))
		return
	}
	e := itm.(entryItem)
	title := lipgloss.NewStyle().Width(14).Render(e.name)
	desc := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(e.desc)
	if index == m.Index() {
		title = lipgloss.NewStyle().Underline(true).Foreground(lipgloss.Color("81")).Width(14).Render(e.name)
	}
	fmt.Fprintf(w, "%s  %s\n", title, desc)
}

/*──────────────────────── filter ──────────────────────*/

func (m *BuilderModel) applyFilter(cat string) {
	cat = strings.ToLower(strings.TrimSpace(cat))
	if cat == "" {
		m.toolSel.SetItems(buildToolItems(catalogueNames()))
		return
	}
	var items []list.Item
	for _, it := range buildToolItems(catalogueNames()) {
		switch v := it.(type) {
		case list.Separator:
			if strings.Contains(strings.ToLower(v.String()), cat) {
				items = append(items, v)
			}
		case entryItem:
			if strings.Contains(strings.ToLower(v.name), cat) {
				items = append(items, v)
			}
		}
	}
	m.toolSel.SetItems(items)
}

/*──────────────────────── hit-testing ─────────────────*/

func hitHeader(v tea.MouseMsg) bool { return v.Y == 0 }
func hitList(v tea.MouseMsg) bool   { return v.X < 45 && v.Y >= 2 }
func hitCanvas(v tea.MouseMsg) bool { return v.X >= 45 && v.Y >= 2 }
func hitArgs(v tea.MouseMsg) bool   { return v.X >= 45 && v.Y >= 2+17 }

func headerIndex(v tea.MouseMsg) int { return v.X / 10 }

func listRow(v tea.MouseMsg) int { return v.Y - 3 }

func canvasCoord(v tea.MouseMsg, vp viewport.Model) (int, int) {
	x := (v.X - 46 + vp.XOffset) / 6
	y := (v.Y - 3 + vp.YOffset)
	return x, y
}

/*──────────────────────── misc util ───────────────────*/

func idAtCursor(m BuilderModel) string {
	for _, id := range m.g.GetLayer(m.curY) {
		if m.g.Nodes[id].Layer == m.curY {
			if idx := indexOf(id, m.g.GetLayer(m.curY)); idx == m.curX {
				return id
			}
		}
	}
	return ""
}

func indexOf(id string, slice []string) int {
	for i, v := range slice {
		if v == id {
			return i
		}
	}
	return -1
}

func clamp(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

/* stripAnsi is used only in msg */
func stripAnsi(s string) string {
	return lipgloss.NewStyle().UnsetString(s)
}
