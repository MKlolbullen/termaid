package tui

import (
	"context"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/list"

	"bb-runner/internal/graph"
	"bb-runner/internal/pipeline"
)

type tmplPicker struct {
	paths []string
	list  list.Model
}

func newTmplPicker(paths []string) tmplPicker {
	items := make([]list.Item, len(paths))
	for i, p := range paths {
		items[i] = entryItem{filepath.Base(p), p} // desc holds full path
	}
	l := list.New(items, list.NewDefaultDelegate(), 40, 15)
	l.Title = "Templates (enter to run)"
	return tmplPicker{paths, l}
}

func (t tmplPicker) Init() tea.Cmd { return nil }

func (t tmplPicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok && k.String() == "enter" {
		path := t.list.SelectedItem().(entryItem).desc
		dag, err := loadWorkflow(path)
		if err != nil {
			return errView(err), nil
		}
		cats := dagToCategories(dag)
		ch := make(chan pipeline.Status, 128)
		go func() {
			_ = pipeline.Run(context.Background(), "input", "workdir", cats, 6, ch)
			close(ch)
		}()
		return New(cats, ch), nil
	}
	var cmd tea.Cmd
	t.list, cmd = t.list.Update(msg)
	return t, cmd
}

func (t tmplPicker) View() string { return t.list.View() }
