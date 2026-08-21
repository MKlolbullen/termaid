package tui

import "testing"

func TestEmbeddedCatalogLoaded(t *testing.T) {
	if len(catalog) == 0 {
		t.Fatal("embedded catalog is empty")
	}
	sf, ok := catalogMap["subfinder"]
	if !ok {
		t.Fatal("subfinder missing from catalog")
	}
	if sf.In != "domain" || sf.Out != "hosts" {
		t.Fatalf("subfinder types = %s -> %s, want domain -> hosts", sf.In, sf.Out)
	}
}

func TestParseCatalogStripsLeadingBinaryName(t *testing.T) {
	data := []byte("bbot:\n  cat: discovery\n  in: domain\n  out: urls\n  def: [\"bbot\", \"-t\", \"$(target)\"]\n")
	entries, err := parseCatalog(data)
	if err != nil {
		t.Fatalf("parseCatalog: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	e := entries[0]
	if len(e.DefArr) != 2 || e.DefArr[0] != "-t" {
		t.Fatalf("leading binary name not stripped: %v", e.DefArr)
	}
	if e.Desc == "" {
		t.Fatal("expected a derived description")
	}
}

func TestNormalizePlaceholders(t *testing.T) {
	got := normalizePlaceholders("-l $(target_file) -d $(target) -o $(output)")
	want := "-l {{input}} -d {{domain}} -o {{output}}"
	if got != want {
		t.Fatalf("normalizePlaceholders = %q, want %q", got, want)
	}
}

func TestCanPipeTypeChecks(t *testing.T) {
	m := NewBuilder(catalogueNames())
	if err := m.g.AddNode("input", "subfinder-1", "subfinder", "", 1); err != nil {
		t.Fatalf("AddNode: %v", err)
	}

	// Root emits the seed domain: subfinder (in=domain) accepts it.
	if !m.canPipe("input", "subfinder") {
		t.Fatal("input -> subfinder should be allowed")
	}
	// httpx wants hosts, not a raw domain.
	if m.canPipe("input", "httpx") {
		t.Fatal("input -> httpx should be rejected (domain != hosts)")
	}
	// subfinder (out=hosts) -> httpx (in=hosts) is valid.
	if !m.canPipe("subfinder-1", "httpx") {
		t.Fatal("subfinder -> httpx should be allowed (hosts -> hosts)")
	}
	// subfinder (out=hosts) -> nuclei (in=urls) is a mismatch.
	if m.canPipe("subfinder-1", "nuclei") {
		t.Fatal("subfinder -> nuclei should be rejected (hosts != urls)")
	}
}
