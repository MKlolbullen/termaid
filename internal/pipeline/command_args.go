package pipeline

import (
	"fmt"
	"strings"
	"unicode"
)

// commandInvocation is the fully prepared argv plus the decision whether
// Termaid should capture stdout into the node output artifact. Commands are
// still executed directly with exec.CommandContext; this parser deliberately
// does not perform shell expansion, globbing, command substitution, or pipes.
type commandInvocation struct {
	Args          []string
	CaptureStdout bool
}

// prepareCommandString parses a workflow's human-editable argument string,
// preserves quoted/escaped values, handles the legacy `> {{output}}` spelling
// without invoking a shell, substitutes Termaid placeholders, and decides
// whether stdout needs to be captured by the runtime.
func prepareCommandString(raw, domain, inputPath, outputFile string) (commandInvocation, error) {
	args, err := splitCommandArgs(raw)
	if err != nil {
		return commandInvocation{}, err
	}
	return prepareCommandArgv(args, domain, inputPath, outputFile)
}

// prepareCommandArgv is the equivalent path for legacy callers whose arguments
// are already tokenized (pipeline.Tool). It intentionally recognizes only
// stdout redirection to Termaid's own output placeholder. Other shell syntax is
// left as a normal argument rather than being evaluated.
func prepareCommandArgv(rawArgs []string, domain, inputPath, outputFile string) (commandInvocation, error) {
	clean, redirected, err := stripOutputRedirect(rawArgs)
	if err != nil {
		return commandInvocation{}, err
	}

	captureStdout := redirected || !writesOwnFile(clean)
	return commandInvocation{
		Args:          substituteArgs(clean, domain, inputPath, outputFile),
		CaptureStdout: captureStdout,
	}, nil
}

// stripOutputRedirect translates the legacy shell-looking forms
//
//	> {{output}}
//	1> {{output}}
//	>{{output}}
//	1>{{output}}
//
// into direct stdout capture. Only the Termaid output placeholder is accepted;
// arbitrary filesystem redirection is deliberately not emulated.
func stripOutputRedirect(rawArgs []string) ([]string, bool, error) {
	clean := make([]string, 0, len(rawArgs))
	redirected := false

	for i := 0; i < len(rawArgs); i++ {
		a := rawArgs[i]
		switch a {
		case ">", "1>":
			if i+1 >= len(rawArgs) {
				return nil, false, fmt.Errorf("stdout redirection %q is missing a destination", a)
			}
			dest := rawArgs[i+1]
			if !isOutputPlaceholder(dest) {
				return nil, false, fmt.Errorf("unsupported stdout redirection to %q; use {{output}} so Termaid can manage the artifact", dest)
			}
			if redirected {
				return nil, false, fmt.Errorf("multiple stdout redirections are not supported")
			}
			redirected = true
			i++
			continue
		}

		if dest, ok := compactOutputRedirect(a); ok {
			if !isOutputPlaceholder(dest) {
				return nil, false, fmt.Errorf("unsupported stdout redirection to %q; use {{output}} so Termaid can manage the artifact", dest)
			}
			if redirected {
				return nil, false, fmt.Errorf("multiple stdout redirections are not supported")
			}
			redirected = true
			continue
		}

		clean = append(clean, a)
	}

	return clean, redirected, nil
}

func compactOutputRedirect(arg string) (string, bool) {
	for _, prefix := range []string{"1>", ">"} {
		if strings.HasPrefix(arg, prefix) && len(arg) > len(prefix) {
			return arg[len(prefix):], true
		}
	}
	return "", false
}

func isOutputPlaceholder(value string) bool {
	return value == "{{output}}" || value == "$(output)"
}

// splitCommandArgs is a small shell-like lexer, not a shell. It supports the
// quoting users expect in workflow JSON while keeping execution deterministic:
// whitespace separates arguments, single/double quotes group text, and a
// backslash escapes the next rune outside single quotes.
func splitCommandArgs(raw string) ([]string, error) {
	var args []string
	var current strings.Builder
	var quote rune
	escaped := false
	tokenStarted := false

	flush := func() {
		if tokenStarted {
			args = append(args, current.String())
			current.Reset()
			tokenStarted = false
		}
	}

	for _, r := range raw {
		if escaped {
			current.WriteRune(r)
			tokenStarted = true
			escaped = false
			continue
		}

		if quote == '\'' {
			if r == '\'' {
				quote = 0
			} else {
				current.WriteRune(r)
			}
			tokenStarted = true
			continue
		}

		if quote == '"' {
			switch r {
			case '"':
				quote = 0
			case '\\':
				escaped = true
			default:
				current.WriteRune(r)
			}
			tokenStarted = true
			continue
		}

		switch {
		case r == '\\':
			escaped = true
			tokenStarted = true
		case r == '\'' || r == '"':
			quote = r
			tokenStarted = true
		case unicode.IsSpace(r):
			flush()
		default:
			current.WriteRune(r)
			tokenStarted = true
		}
	}

	if escaped {
		return nil, fmt.Errorf("arguments end with an unfinished escape")
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated %c quote in arguments", quote)
	}
	flush()
	return args, nil
}
