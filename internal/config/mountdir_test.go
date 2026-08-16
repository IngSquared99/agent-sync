package config

import (
	"strings"
	"testing"

	"github.com/IngSquared99/agent-sync/i18n"
)

// mount.dir gets the same basic guards as build.out: duplicate dirs overwrite
// each other's links, a dir inside the output is wiped by apply, and a dir
// inside a source contaminates scanning.
func TestMountDirGuards(t *testing.T) {
	i18n.SetLang("en")

	dup := baseYAML + `  - dir: .claude
    links: {skills: skills}
`
	mustFail(t, dup, "appears more than once")

	mustFail(t, strings.Replace(baseYAML, "dir: .claude", "dir: .agsy/claude", 1),
		"inside build.out")

	mustFail(t, strings.Replace(baseYAML, "dir: .claude", "dir: ./src/tool", 1),
		"scanned as source content")
}
