package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/IngSquared99/agent-sync/i18n"
)

// A minimal working config; each test replaces the one piece it wants to verify
const baseYAML = `version: 1
sources:
  - ./src
build:
  out: .agsy
  on_conflict: {rules: rename, skills: error, workflows: rename}
  route: {field: target, default: [claude], buckets: [claude]}
mount:
  - dir: .claude
    links: {rules: rules, commands: workflows/claude}
`

func load(t *testing.T, body string) (*Config, error) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, FileName)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return Load(p)
}

// mustFail asserts the config is rejected and the error message states the cause
// (the message is the user's only clue)
func mustFail(t *testing.T, body, wantSubstr string) {
	t.Helper()
	_, err := load(t, body)
	if err == nil {
		t.Fatalf("expected validation to fail (%s), but it passed", wantSubstr)
	}
	if !strings.Contains(err.Error(), wantSubstr) {
		t.Fatalf("error message does not mention %q:\n%v", wantSubstr, err)
	}
}

func TestLoadOK(t *testing.T) {
	cfg, err := load(t, baseYAML)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Build.Categories["rules"].From != "rule" {
		t.Errorf("categories defaults not filled in: %+v", cfg.Build.Categories)
	}
	if filepath.Base(cfg.OutDir()) != ".agsy" {
		t.Errorf("OutDir = %s", cfg.OutDir())
	}
}

// build.out is wiped entirely by apply, so it may only be a dedicated directory
// inside the project
func TestOutMustBeInsideProject(t *testing.T) {
	i18n.SetLang("en")
	cases := []struct{ out, want string }{
		{"../outside", "not inside the project directory"},
		{"/tmp/somewhere-else", "not inside the project directory"},
		{".", "resolves to the project root"},
		{"..", "an ancestor of the project root"},
		// Note: a bare ~ in yaml is null; quoting is needed to test the home
		// directory — a pitfall users may hit too
		{"'~'", "contains the home directory"},
		{"./src", "contains source"},
	}
	for _, c := range cases {
		t.Run(c.out, func(t *testing.T) {
			mustFail(t, strings.Replace(baseYAML, "out: .agsy", "out: "+c.out, 1), c.want)
		})
	}
}

// Two categories outputting to the same level → files and directories mix, and
// cross-category name clashes go undetected
func TestCategoriesToMustDiffer(t *testing.T) {
	i18n.SetLang("en")
	body := strings.Replace(baseYAML, "  out: .agsy\n",
		"  out: .agsy\n  categories: {rules: {to: docs}, skills: {to: docs}}\n", 1)
	mustFail(t, body, "must output to different subdirectories")
}

func TestOnConflictRequired(t *testing.T) {
	i18n.SetLang("en")
	body := strings.Replace(baseYAML,
		"on_conflict: {rules: rename, skills: error, workflows: rename}",
		"on_conflict: {rules: rename, workflows: rename}", 1)
	mustFail(t, body, "build.on_conflict.skills is not set")

	body = strings.Replace(baseYAML, "skills: error", "skills: whatever", 1)
	mustFail(t, body, "invalid")
}

func TestMountCrossValidation(t *testing.T) {
	i18n.SetLang("en")
	cases := []struct{ links, want string }{
		{"{rules: rules, wat: nosuch}", "no such top level"},
		{"{rules: rules, commands: workflows/nobucket}", "not in build.route.buckets"},
		{"{rules: rules/deep}", "only workflows has a second bucket level"},
		{"{commands: workflows/claude/deep}", "nested too deep"},
		{"{rules: ''}", "has no target"},
	}
	for _, c := range cases {
		t.Run(c.want, func(t *testing.T) {
			mustFail(t, strings.Replace(baseYAML, "{rules: rules, commands: workflows/claude}", c.links, 1), c.want)
		})
	}
}

func TestRouteDefaultMustBeKnownBucket(t *testing.T) {
	i18n.SetLang("en")
	mustFail(t, strings.Replace(baseYAML, "default: [claude]", "default: [agents]", 1), "nonexistent bucket")
}

func TestVersionTooNew(t *testing.T) {
	i18n.SetLang("en")
	mustFail(t, strings.Replace(baseYAML, "version: 1", "version: 99", 1), "please upgrade agsy")
}

func TestExpandPath(t *testing.T) {
	cfg := &Config{BaseDir: "/proj"}
	home, _ := os.UserHomeDir()
	cases := []struct{ in, want string }{
		{"./.flow", filepath.Join("/proj", ".flow")},
		{".flow", filepath.Join("/proj", ".flow")},
		{"~/ai-lib", filepath.Join(home, "ai-lib")},
		{"/opt/team", "/opt/team"},
	}
	for _, c := range cases {
		got, err := cfg.ExpandPath(c.in)
		if err != nil {
			t.Fatal(err)
		}
		if got != c.want {
			t.Errorf("ExpandPath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIsAncestor(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"/a", "/a/b", true},
		{"/a", "/a", false},
		{"/a/b", "/a", false},
		{"/a", "/ab", false}, // a bare string prefix is not an ancestor
	}
	for _, c := range cases {
		if got := IsAncestor(c.a, c.b); got != c.want {
			t.Errorf("IsAncestor(%q,%q) = %v", c.a, c.b, got)
		}
	}
}

func TestSourceTagAndRootOf(t *testing.T) {
	if got := SourceTag("/x/y/.flow"); got != "flow" {
		t.Errorf("SourceTag = %q, want the leading dot stripped", got)
	}
	cfg := &Config{BaseDir: "/proj", Sources: []string{"/lib", "/lib/inner"}}
	// longest prefix wins: nested sources must be attributed to the deeper one
	if root, ok := cfg.SourceRootOf("/lib/inner/rule/a.md"); !ok || root != "/lib/inner" {
		t.Errorf("SourceRootOf = %q,%v", root, ok)
	}
	if _, ok := cfg.SourceRootOf("/elsewhere/a.md"); ok {
		t.Error("a path belonging to no source should have no root")
	}
}
