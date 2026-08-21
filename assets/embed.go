// Package assets embeds static resources (the tool catalog) so the compiled
// termaid binary is self-contained and can run from any working directory.
package assets

import _ "embed"

// ToolsYAML is the built-in tool catalog, embedded at build time from
// assets/tools.yaml. Callers may still override it with a user-supplied file.
//
//go:embed tools.yaml
var ToolsYAML []byte
