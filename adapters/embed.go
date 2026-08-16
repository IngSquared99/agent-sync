// Package adapters embeds the built-in mount presets for each vendor
// (factory templates for init, not a runtime dependency).
// To add a new tool: drop a <name>.yaml into this directory; see the
// existing files for the format.
package adapters

import "embed"

//go:embed *.yaml
var FS embed.FS
