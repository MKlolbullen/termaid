package tui

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/MKlolbullen/termaid/assets"
)

/* ------ Shared UI list.Item: entryItem ------ */

type entryItem struct{ name, desc string }

func (e entryItem) Title() string       { return e.name }
func (e entryItem) Description() string { return e.desc }
func (e entryItem) FilterValue() string { return e.name }

/* ─── rawTool mirrors the YAML catalog schema ─────────────────────────────
 *
 * assets/tools.yaml is a mapping keyed by tool name, e.g.
 *
 *   subfinder:
 *     cat: discovery
 *     in:  domain
 *     out: hosts
 *     def: ["-silent","-json","-o","-","$(target)"]
 *     params:
 *       threads: {type: int, default: 25, doc: "..."}
 */

type rawTool struct {
	Cat    string              `yaml:"cat"`
	In     string              `yaml:"in"`
	Out    string              `yaml:"out"`
	Desc   string              `yaml:"desc"`
	Def    []string            `yaml:"def"`
	Params map[string]rawParam `yaml:"params"`
}

type rawParam struct {
	Type    string   `yaml:"type"`
	Default any      `yaml:"default"`
	Doc     string   `yaml:"doc"`
	Values  []string `yaml:"values"`
}

/* ─── catalogEntry (resolved, UI-friendly) ────────────────────────────────── */

type catalogEntry struct {
	Name   string
	Cat    string
	In     string // input data type (domain, hosts, urls, ...)
	Out    string // output data type
	Desc   string
	Def    string   // default args, space-joined for display/editing
	DefArr []string // default args, pre-split
	Params map[string]rawParam
}

/* ─── global catalog ──────────────────────────────────────────────────────
 *
 * catalog    - sorted slice, drives the tool picker list.
 * catalogMap - name -> entry, used for type-aware piping in the builder.
 */

var (
	catalog    []catalogEntry
	catalogMap map[string]catalogEntry
)

func init() {
	entries, err := parseCatalog(assets.ToolsYAML)
	if err != nil {
		panic(fmt.Errorf("termaid: failed to parse embedded tool catalog: %w", err))
	}
	setCatalog(entries)
}

// LoadCatalog reads and parses a catalog file, replacing the active catalog.
// It lets users point termaid at a customized assets/tools.yaml.
func LoadCatalog(path string) ([]catalogEntry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	entries, err := parseCatalog(raw)
	if err != nil {
		return nil, err
	}
	setCatalog(entries)
	return entries, nil
}

// parseCatalog decodes the YAML catalog into resolved entries.
func parseCatalog(data []byte) ([]catalogEntry, error) {
	var raw map[string]rawTool
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	entries := make([]catalogEntry, 0, len(raw))
	for name, t := range raw {
		def := t.Def
		// Some catalog entries prefix the binary name in def (e.g. bbot,
		// masscan). The binary is invoked separately, so drop a redundant
		// leading token to avoid running "bbot bbot ...".
		if len(def) > 0 && def[0] == name {
			def = def[1:]
		}

		desc := t.Desc
		if desc == "" {
			desc = describeTool(t)
		}

		entries = append(entries, catalogEntry{
			Name:   name,
			Cat:    t.Cat,
			In:     t.In,
			Out:    t.Out,
			Desc:   desc,
			Def:    strings.Join(def, " "),
			DefArr: def,
			Params: t.Params,
		})
	}
	return entries, nil
}

// describeTool builds a short human-readable description from the schema when
// the catalog entry does not supply one.
func describeTool(t rawTool) string {
	flow := ""
	if t.In != "" || t.Out != "" {
		flow = fmt.Sprintf("%s → %s", orDash(t.In), orDash(t.Out))
	}
	if t.Cat == "" {
		return flow
	}
	if flow == "" {
		return t.Cat
	}
	return fmt.Sprintf("%s (%s)", t.Cat, flow)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// setCatalog installs entries as the active catalog, sorted by category then
// name so the picker groups tools sensibly.
func setCatalog(entries []catalogEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Cat != entries[j].Cat {
			return entries[i].Cat < entries[j].Cat
		}
		return entries[i].Name < entries[j].Name
	})

	catalog = entries
	catalogMap = make(map[string]catalogEntry, len(entries))
	for _, e := range entries {
		catalogMap[e.Name] = e
	}
}

/* helper used by builder */
func defaultArgs(tool string) string {
	if e, ok := catalogMap[tool]; ok {
		return normalizePlaceholders(e.Def)
	}
	return ""
}

// normalizePlaceholders converts catalog-style placeholders ($(target),
// $(target_file)) into the pipeline's canonical {{...}} form so a freshly
// built workflow runs the same way as a hand-written preset.
func normalizePlaceholders(args string) string {
	r := strings.NewReplacer(
		"$(target_file)", "{{input}}",
		"$(target)", "{{domain}}",
		"$(output)", "{{output}}",
	)
	return r.Replace(args)
}
