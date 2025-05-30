package tui

import (
	"os"
	"sort"

	"gopkg.in/yaml.v3"
)

/* ------ Shared UI list.Item: entryItem ------ */

type entryItem struct{ name, desc string }

func (e entryItem) Title() string       { return e.name }
func (e entryItem) Description() string { return e.desc }
func (e entryItem) FilterValue() string { return e.name }

/* ─── catalogEntry (YAML) ─────────────────────────────────────────── */

type catalogEntry struct {
	Name string `yaml:"name"`
	Cat  string `yaml:"cat"`
	Desc string `yaml:"desc"`
	Def  string `yaml:"def"`
}

/* ─── entryItem (UI list item) ────────────────────────────────────── */


/* ─── global catalog slice ───────────────────────────────────────── */

var catalog []catalogEntry

func init() {
	c, err := LoadCatalog("assets/tools.yaml")
	if err != nil {
		panic(err)
	}
	catalog = c
}

func LoadCatalog(path string) ([]catalogEntry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var list []catalogEntry
	if err := yaml.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	return list, nil
}

/* helper used by builder */
func defaultArgs(tool string) string {
	for _, c := range catalog {
		if c.Name == tool {
			return c.Def
		}
	}
	return ""
}
