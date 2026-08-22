package pipeline

import (
	"reflect"
	"testing"
)

func TestSplitCommandArgsPreservesQuotesAndEscapes(t *testing.T) {
	got, err := splitCommandArgs(`--header "Authorization: Bearer {{domain}}" --data 'hello world' --name escaped\ value --empty ""`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"--header",
		"Authorization: Bearer {{domain}}",
		"--data",
		"hello world",
		"--name",
		"escaped value",
		"--empty",
		"",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
}

func TestSplitCommandArgsKeepsBashCBodyTogether(t *testing.T) {
	got, err := splitCommandArgs(`bash -c "sort -u | LC_ALL=C sort"`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"bash", "-c", "sort -u | LC_ALL=C sort"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
}

func TestSplitCommandArgsRejectsMalformedQuotes(t *testing.T) {
	if _, err := splitCommandArgs(`--header "unfinished`); err == nil {
		t.Fatal("expected unterminated quote error")
	}
	if _, err := splitCommandArgs(`foo\`); err == nil {
		t.Fatal("expected unfinished escape error")
	}
}

func TestPrepareCommandStringTranslatesLegacyOutputRedirect(t *testing.T) {
	inv, err := prepareCommandString(
		`--subs-only {{domain}} > {{output}}`,
		"example.com",
		"/tmp/input.txt",
		"/tmp/output.txt",
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--subs-only", "example.com"}
	if !reflect.DeepEqual(inv.Args, want) {
		t.Fatalf("args = %#v, want %#v", inv.Args, want)
	}
	if !inv.CaptureStdout {
		t.Fatal("legacy > {{output}} must become runtime stdout capture")
	}
}

func TestPrepareCommandStringSupportsCompactRedirect(t *testing.T) {
	inv, err := prepareCommandString(`--subs-only {{domain}} 1>{{output}}`, "example.com", "in", "out")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(inv.Args, []string{"--subs-only", "example.com"}) || !inv.CaptureStdout {
		t.Fatalf("unexpected invocation: %#v", inv)
	}
}

func TestPrepareCommandStringRespectsToolOwnedOutput(t *testing.T) {
	inv, err := prepareCommandString(`-l {{input}} -o {{output}}`, "example.com", "/tmp/in", "/tmp/out")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-l", "/tmp/in", "-o", "/tmp/out"}
	if !reflect.DeepEqual(inv.Args, want) {
		t.Fatalf("args = %#v, want %#v", inv.Args, want)
	}
	if inv.CaptureStdout {
		t.Fatal("tool-owned {{output}} should not also capture stdout")
	}
}

func TestPrepareCommandStringCapturesStdoutDash(t *testing.T) {
	inv, err := prepareCommandString(`-l {{input}} -o -`, "example.com", "/tmp/in", "/tmp/out")
	if err != nil {
		t.Fatal(err)
	}
	if !inv.CaptureStdout {
		t.Fatal("-o - output must be captured by Termaid")
	}
}

func TestPrepareCommandStringRejectsArbitraryRedirectDestination(t *testing.T) {
	if _, err := prepareCommandString(`--subs-only {{domain}} > results.txt`, "example.com", "in", "out"); err == nil {
		t.Fatal("expected arbitrary redirect destination to be rejected")
	}
}
