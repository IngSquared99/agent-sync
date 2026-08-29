package config

import (
	"strings"
	"testing"

	"github.com/IngSquared99/agent-sync/i18n"
)

// Duplicate sources would be scanned twice; two spellings of the same path
// must be caught, not only literal repeats.
func TestDuplicateSourcesRejected(t *testing.T) {
	i18n.SetLang("en")
	dup := strings.Replace(baseYAML, "sources:\n  - ./src\n", "sources:\n  - ./src\n  - src\n", 1)
	mustFail(t, dup, "same directory")
}

// A source inside another source gets its files collected once per root.
func TestNestedSourcesRejected(t *testing.T) {
	i18n.SetLang("en")
	nested := strings.Replace(baseYAML, "sources:\n  - ./src\n", "sources:\n  - ./src\n  - ./src/sub\n", 1)
	mustFail(t, nested, "nested")
}

// ~user is shell syntax for another user's home; silently expanding it to
// $HOME/user would point somewhere else entirely.
func TestTildeUserRejected(t *testing.T) {
	i18n.SetLang("en")
	tilde := strings.Replace(baseYAML, "sources:\n  - ./src\n", "sources:\n  - ~nobody/lib\n", 1)
	mustFail(t, tilde, "~user")
}

// A mount dir outside the project is the hijack vector for an untrusted
// agsy.yaml (point ~/.claude at repo content); it needs an explicit opt-in.
func TestMountOutsideProjectNeedsOptIn(t *testing.T) {
	i18n.SetLang("en")
	outside := strings.Replace(baseYAML, `mount:
  - dir: .claude
    links: {rules: rules, skills: skills}
`, `mount:
  - dir: ../elsewhere
    links: {rules: rules, skills: skills}
`, 1)
	mustFail(t, outside, "outside the project")

	optIn := strings.Replace(baseYAML, `mount:
  - dir: .claude
    links: {rules: rules, skills: skills}
`, `mount:
  - dir: ../elsewhere
    outside_project: true
    links: {rules: rules, skills: skills}
`, 1)
	if _, err := load(t, optIn); err != nil {
		t.Fatalf("outside_project: true must allow the mount: %v", err)
	}
}

// build.tools and the workflows mount must stay consistent: only the stub
// tool reads the workflows directory.
func TestWorkflowsMountCrossCheck(t *testing.T) {
	i18n.SetLang("en")
	// workflows mounted, stub tool absent → the mounted dir stays empty
	mounted := strings.Replace(baseYAML, "links: {rules: rules, skills: skills}",
		"links: {rules: rules, skills: skills, workflows: workflows}", 1)
	mustFail(t, mounted, "does not list")

	// stub tool listed, workflows never mounted → stubs no tool reads
	unread := strings.Replace(baseYAML, "tools: [claude, codex]", "tools: [claude, "+StubTool+"]", 1)
	mustFail(t, unread, "no mount links to the workflows output")

	// both present → fine
	both := strings.Replace(mounted, "tools: [claude, codex]", "tools: [claude, "+StubTool+"]", 1)
	if _, err := load(t, both); err != nil {
		t.Fatalf("consistent workflows mount + tools must validate: %v", err)
	}
}
