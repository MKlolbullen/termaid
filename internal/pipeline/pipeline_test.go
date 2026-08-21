package pipeline

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSubstituteArgsBothDialects(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "curly placeholders",
			in:   []string{"-l", "{{input}}", "-d", "{{domain}}", "-o", "{{output}}"},
			want: []string{"-l", "/in.txt", "-d", "example.com", "-o", "/out.txt"},
		},
		{
			name: "dollar placeholders",
			in:   []string{"-silent", "$(target)", "-l", "$(target_file)"},
			want: []string{"-silent", "example.com", "-l", "/in.txt"},
		},
		{
			name: "multiple placeholders in one arg",
			in:   []string{"-u", "{{input}}/FUZZ"},
			want: []string{"-u", "/in.txt/FUZZ"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := substituteArgs(tc.in, "example.com", "/in.txt", "/out.txt")
			if len(got) != len(tc.want) {
				t.Fatalf("len = %d, want %d (%v)", len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("arg %d = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestWritesOwnFile(t *testing.T) {
	if !writesOwnFile([]string{"-o", "{{output}}"}) {
		t.Fatal("expected true for {{output}}")
	}
	if !writesOwnFile([]string{"-o", "$(output)"}) {
		t.Fatal("expected true for $(output)")
	}
	if writesOwnFile([]string{"-o", "-", "-silent"}) {
		t.Fatal("expected false for stdout-only args")
	}
}

func TestMergeOutputsDeduplicates(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.txt", "a.example.com\nb.example.com\n")
	write("b.txt", "b.example.com\nc.example.com\n") // b duplicated

	merged, err := mergeOutputs(dir)
	if err != nil {
		t.Fatalf("mergeOutputs: %v", err)
	}
	data, err := os.ReadFile(merged)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for _, line := range splitNonEmpty(string(data)) {
		seen[line]++
	}
	if len(seen) != 3 {
		t.Fatalf("unique lines = %d, want 3 (%v)", len(seen), seen)
	}
	for k, v := range seen {
		if v != 1 {
			t.Fatalf("line %q appears %d times, want 1", k, v)
		}
	}
}

func TestValidateToolMissingBinary(t *testing.T) {
	err := validateTool(&Tool{Command: "definitely-not-a-real-binary-xyz", Args: []string{"-x"}})
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
}

func splitNonEmpty(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == '\n' {
			if cur != "" {
				out = append(out, cur)
			}
			cur = ""
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
