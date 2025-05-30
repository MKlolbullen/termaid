package tui

import (
	"path/filepath"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

/*─────────────────────────────────────────────
 *  TmplPicker lets you pick a workflow template.
 * ─────────────────────────────────────────────*/

type tmplPicker struct {
	files []string
	list  list.Model
}

func newTmplPicker(files []string) tmplPicker {
	items := make([]list.Item, len(files))
	for i, f := range files {
		items[i] = entryItem{filepath.Base(f), f}
	}
	l := list.New(items, list.NewDefaultDelegate(), 32, 12)
	l.Title = "Select Workflow Template"
	return tmplPicker{files, l}
}

func (m tmplPicker) Init() tea.Cmd { return nil }

func (m tmplPicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.KeyMsg:
		if v.String() == "q" || v.String() == "ctrl+c" {
			return NewMenu(), nil
		}
		if v.String() == "enter" {
			selected := m.list.SelectedItem().(entryItem).desc // file path
			domInput := textinput.New()
			domInput.Placeholder = "target.com"
			domInput.Focus()
			return domainPrompt{input: domInput, template: selected}, nil
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m tmplPicker) View() string {
	return m.list.View()
}
