package tui

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"

	"github.com/MKlolbullen/termaid/internal/graph"
)

/*─────────────────────── visual styles ─────────────────────────*/

var (
	borderAct   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("10"))
	borderInact = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("8"))

	// header buttons
	btnStyle = lipgloss.NewStyle().Padding(0, 1).Bold(true)
	btnRun   = btnStyle.Background(lipgloss.Color("#005f00")).Foreground(lipgloss.Color("15"))
	btnPause = btnStyle.Background(lipgloss.Color("#5f5f00")).Foreground(lipgloss.Color("15"))
	btnStop  = btnStyle.Background(lipgloss.Color("#5f0000")).Foreground(lipgloss.Color("15"))
	btnGrey  = btnStyle.Background(lipgloss.Color("#444")).Foreground(lipgloss.Color("230"))
	btnSel   = lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Bold(true)

	// matrix cell palettes
	pickedCell  = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("13"))
	blockedCell = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("9"))

	inTypeColor = map[string]string{
		"domain":     "10",
		"subdomains": "12",
		"hosts":      "6",
		"urls":       "11",
		"js":         "208",
		"params":     "13",
		"mixed":      "8",
		"raw":        "8",
	}
)

/*─────────────────────── focus enum ───────────────────────────*/

type focusArea int

const (
	fHeader focusArea = iota
	fDomain
	fList
	fCanvas
	fArgs
)

/*─────────────────────── Builder model ────────────────────────*/

type BuilderModel struct {
	// header
	btns   []string
	btnIdx int

	// panes
	domainInp textinput.Model
	toolSel   list.Model
	argsInp   textinput.Model
	canvas    viewport.Model

	// filtering
	filterMode bool
	filterBox  textinput.Model

	// move (pick-and-drop)
	moveMode bool
	pickID   string

	// DAG
	g   *graph.DAG
	occ map[string]int

	// cursor / focus
	focus focusArea
	curY  int
	curX  int
	msg   string
}

/*─────────────────────── constructor ─────────────────────────*/

func NewBuilder(tools []string) BuilderModel {
	// header buttons
	btns := []string{
		btnRun.Render("▶ Run"),
		btnPause.Render("⏸ Pause"),
		btnStop.Render("■ Stop"),
		btnGrey.Render("💾 Save"),
		btnGrey.Render("📂 Load"),
	}

	// domain
	dom := textinput.New()
	dom.Placeholder = "example.com"

	// tool list with separators + desc
	items := buildToolItems(tools)
	lst := list.New(items, toolDelegate{}, 45, 16)
	lst.Title = "Tools ( / = filter )"

	// filter box
	filt := textinput.New()
	filt.Placeholder = "category…"

	// args
	arg := textinput.New()
	arg.Placeholder = "args"
	arg.Width = 40

	// workflow viewport
	cv := viewport.New(50, 16)
	cv.YPosition = 1

	return BuilderModel{
		btns:       btns,
		domainInp:  dom,
		toolSel:    lst,
		filterBox:  filt,
		argsInp:    arg,
		canvas:     cv,
		g:          graph.NewDAG(),
		occ:        make(map[string]int),
		focus:      fHeader,
	}
}

func (m BuilderModel) Init() tea.Cmd { return nil }

/*─────────────────────── Update loop ─────────────────────────*/

func (m BuilderModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {

	switch v := msg.(type) {

	/*──────── mouse handling ───────*/
	case tea.MouseMsg:
		if v.Button == tea.MouseButtonLeft && v.Type == tea.MouseButtonPress {
			switch {
			case hitHeader(v):
				m.focus = fHeader
				m.btnIdx = headerIndex(v)
			case hitList(v):
				m.focus = fList
				m.toolSel.Select(listRow(v))
			case hitCanvas(v):
				m.focus = fCanvas
				m.curX, m.curY = canvasCoord(v, m.canvas)
				if id := idAtCursor(m); id != "" {
					m.selNode = id
				}
			case hitArgs(v):
				m.focus = fArgs
			}
		}
		if m.focus == fCanvas {
			if v.Type == tea.MouseWheelUp {
				m.canvas.LineUp(3)
			}
			if v.Type == tea.MouseWheelDown {
				m.canvas.LineDown(3)
			}
		}

	/*──────── keyboard handling ────*/
	case tea.KeyMsg:
		m.handleKeys(v)
	}

	/* delegate subcomponents */
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

	/* refresh canvas */
	m.canvas.SetContent(renderMatrix(&m))

	return m, nil
}

/*─────────────────────── key handlers ───────────────────────*/

func (m *BuilderModel) handleKeys(k tea.KeyMsg) {
	ks := k.String()

	// move-mode keys first
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
		return
	}

	switch m.focus {

	/* header */
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
			m.msg = "clicked " + stripAnsi(m.btns[m.btnIdx])
		}

	/* domain */
	case fDomain:
		if ks == "tab" {
			m.focus = fList
		}

	/* tool list */
	case fList:
		if !m.filterMode {
			switch ks {
			case "/":
				m.filterMode = true
				m.filterBox.Reset(); m.filterBox.Focus()
			case "tab":
				m.focus = fCanvas
			}
		} else { // in filter mode
			switch ks {
			case "enter":
				m.applyFilter(m.filterBox.Value())
				m.filterMode = false
			case "esc":
				m.filterMode = false
			}
		}

	/* canvas */
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
		case "m":
			if m.selNode != "input" {
				m.moveMode, m.pickID = true, m.selNode
			}
		case "left", "right", "up", "down":
			m.arrowMove(ks)
		}

	/* args */
	case fArgs:
		if ks == "shift+tab" {
			m.focus = fCanvas
		}
	}
}

/*────────────────── DAG operations (add/rm/move) ───────────*/

func (m *BuilderModel) nodeOps(k string) {
	switch k {

	case "n": // add child
		tool := m.toolSel.SelectedItem().(entryItem).name
		if !canPipe(m.selNode, tool) {
			m.msg = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("Type mismatch!")
			return
		}
		m.occ[tool]++
		id := fmt.Sprintf("%s-%d", tool, m.occ[tool])
		args := defaultArgs(tool)
		_ = m.g.AddNode(m.selNode, id, tool, args, m.curY+1)

	case "r": // remove
		if m.selNode != "input" {
			_ = m.g.RemoveNode(m.selNode)
			m.selNode = "input"
		}

	case "c": // configure args
		if n := m.g.Nodes[m.selNode]; n != nil {
			n.Args = m.argsInp.Value()
		}
	}
}

func (m *BuilderModel) moveSubtree() {
	node := m.g.Nodes[m.pickID]
	dy := m.curY - node.Layer
	m.g.RemoveFromLayer(m.pickID)
	m.g.InsertAtLayer(m.pickID, m.curY, m.curX)
	m.shiftChildren(m.pickID, dy)
}

func (m *BuilderModel) shiftChildren(id string, dy int) {
	for _, ch := range m.g.Nodes[id].Children {
		m.g.Nodes[ch].Layer += dy
		m.shiftChildren(ch, dy)
	}
}

/*────────────────────── pan / zoom ─────────────────────*/

func (m *BuilderModel) zoomPan(key string) {
	switch key {
	case "pgup": // zoom in
		m.canvas.Width = clamp(m.canvas.Width-10, 30, 120)
		m.canvas.Height = clamp(m.canvas.Height+3, 10, 50)
	case "pgdn": // zoom out
		m.canvas.Width = clamp(m.canvas.Width+10, 30, 120)
		m.canvas.Height = clamp(m.canvas.Height-3, 10, 50)
	case "ctrl+left":
		m.canvas.SetXOffset(m.canvas.XOffset - 6)
	case "ctrl+right":
		m.canvas.SetXOffset(m.canvas.XOffset + 6)
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
	if id := idAtCursor(*m); id != "" {
		m.selNode = id
	}
}

/*────────────────────── view rendering ─────────────────*/

func styleCell(m *BuilderModel, id string, active bool) string {
	if id == "" {
		return lipgloss.NewStyle().
			Border(lipgloss.HiddenBorder()).
			Padding(0, 2).Render(" ")
	}
	inT := catalogMap[m.g.Nodes[id].Tool].In
	outT := catalogMap[m.g.Nodes[id].Tool].Out

	st := lipgloss.
		NewStyle().
		BorderLeft(true).BorderRight(true).
		BorderForeground(lipgloss.Color(inTypeColor[inT])).
		BorderRightForeground(lipgloss.Color(inTypeColor[outT])).
		Padding(0, 1)

	if active {
		st = st.Bold(true)
	}
	return st.Render(id)
}

func renderMatrix(m *BuilderModel) string {
	g := m.g
	var b strings.Builder
	max := g.MaxLayer()
	for y := 0; y <= max; y++ {
		row := g.GetLayer(y)
		fmt.Fprintf(&b, "L%-2d ", y)
		for x, id := range row {
			cell := styleCell(m, id, y == m.curY && x == m.curX)
			if m.moveMode && id == m.pickID {
				cell = pickedCell.Render(id)
			}
			if m.moveMode && y == m.curY && x == m.curX && id != "" && id != m.pickID {
				cell = blockedCell.Render(id)
			}
			b.WriteString(cell)
		}
		if m.curY == y && m.curX >= len(row) { // cursor on empty slot
			b.WriteString(styleCell(m, "", true))
		}
		b.WriteString("\n")
	}
	return b.String()
}

/*──────────────────────── View ─────────────────────────*/

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

	/* domain + list */
	domain := maybeBorder("Domain: "+m.domainInp.View(), m.focus == fDomain)
	listPane := m.toolSel.View()
	if m.filterMode {
		listPane = m.filterBox.View()
	}
	tools := maybeBorder(listPane, m.focus == fList)
	left := lipgloss.JoinVertical(lipgloss.Top, domain, tools)

	/* right column */
	right := lipgloss.JoinVertical(lipgloss.Top,
		maybeBorder(m.canvas.View(), m.focus == fCanvas),
		maybeBorder("Args: "+m.argsInp.View(), m.focus == fArgs),
	)

	help := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(
		"↑↓←→ move  n new  r rm  m pick/drop  c args  PgUp/Down zoom  Ctrl+Arrows pan  / filter  ? legend  q quit",
	)

	return hdr + "\n" +
		lipgloss.JoinHorizontal(lipgloss.Top, left, right) +
		"\n" + help + "\n" + m.msg
}

/*──────────────────────── aux utils ───────────────────*/

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

func (toolDelegate) Height() int  { return 1 }
func (toolDelegate) Spacing() int { return 0 }
func (toolDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }
func (toolDelegate) Render(w io.Writer, m list.Model, idx int, itm list.Item) {
	if sep, ok := itm.(list.Separator); ok {
		fmt.Fprintln(w, lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(sep.String()))
		return
	}
	e := itm.(entryItem)
	title := lipgloss.NewStyle().Width(14).Render(e.name)
	desc := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(e.desc)
	if idx == m.Index() {
		title = lipgloss.NewStyle().Underline(true).Foreground(lipgloss.Color("81")).Width(14).Render(e.name)
	}
	fmt.Fprintf(w, "%s  %s\n", title, desc)
}

/*──────── filter tools by category ───────*/

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

/*──────── type check ─────────────────────*/

func canPipe(parentID, childTool string) bool {
	pOut := catalogMap[parentIDTool(parentID)].Out
	cIn := catalogMap[childTool].In
	if pOut == "raw" || cIn == "raw" { return true }
	return pOut == cIn
}
func parentIDTool(id string) string { return catalogMap[id].Name }

/*──────── hit-test helpers ───────────────*/

func hitHeader(v tea.MouseMsg) bool { return v.Y == 0 }
func hitList(v tea.MouseMsg) bool   { return v.X < 45 && v.Y >= 2 }
func hitCanvas(v tea.MouseMsg) bool { return v.X >= 45 && v.Y >= 2 }
func hitArgs(v tea.MouseMsg) bool   { return v.X >= 45 && v.Y >= 21 }

func headerIndex(v tea.MouseMsg) int { return v.X / 10 }
func listRow(v tea.MouseMsg) int     { return v.Y - 3 }

func canvasCoord(v tea.MouseMsg, vp viewport.Model) (int, int) {
	x := (v.X - 46 + vp.XOffset) / 6
	y := (v.Y - 3 + vp.YOffset)
	return x, y
}

/*──────── misc ───────────────────────────*/

func maybeBorder(s string, active bool) string {
	if active {
		return borderAct.Render(s)
	}
	return borderInact.Render(s)
}

func clamp(n, min, max int) int {
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}

func idAtCursor(m BuilderModel) string {
	row := m.g.GetLayer(m.curY)
	if m.curX < len(row) {
		return row[m.curX]
	}
	return ""
}

func stripAnsi(s string) string { return lipgloss.NewStyle().Unset().
	UnsetBorder().UnsetMargin().UnsetPadding().Render(s) }
