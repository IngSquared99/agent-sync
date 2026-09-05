package config

import (
	"strings"
	"testing"
)

// Config validation for the hooks category: one case per rule.

func TestHooksCategoryDefaults(t *testing.T) {
	cfg, err := load(t, baseYAML)
	if err != nil {
		t.Fatal(err)
	}
	h := cfg.Build.Categories["hooks"]
	if h.From != "hooks" || h.To != "hooks" {
		t.Fatalf("hooks category default = %+v", h)
	}
	if cfg.Version != 1 {
		t.Fatalf("version: 1 must still load as written, got %d", cfg.Version)
	}
}

func TestOnConflictHooksRequiredWithUpgradeHint(t *testing.T) {
	body := strings.Replace(baseYAML,
		"on_conflict: {rules: rename, skills: error, workflows: rename, hooks: error}",
		"on_conflict: {rules: rename, skills: error, workflows: rename}", 1)
	mustFail(t, body, "build.on_conflict.hooks is not set (new in v0.2.0)")
}

func TestRegistryNamesReserved(t *testing.T) {
	body := strings.Replace(baseYAML, "  out: .agsy\n",
		"  out: .agsy\n  categories: {hooks: {to: hooks.codex.json}}\n", 1)
	mustFail(t, body, "reserved for a derived hook registry file")
}

func TestRegistryLinkable(t *testing.T) {
	body := baseYAML + "  - dir: .codex\n    links: {hooks.json: hooks.codex.json}\n"
	if _, err := load(t, body); err != nil {
		t.Fatalf("link to registry must be accepted: %v", err)
	}
}

func TestRegistryLinkRequiresTool(t *testing.T) {
	body := baseYAML + "  - dir: .cursor\n    links: {hooks.json: hooks.cursor.json}\n"
	mustFail(t, body, `build.tools does not list "cursor"`)
}

func TestMergeAccepted(t *testing.T) {
	body := strings.Replace(baseYAML,
		"    links: {rules: rules, skills: skills}\n",
		"    links: {rules: rules, skills: skills}\n    merge: {settings.json: hooks.claude.json}\n", 1)
	cfg, err := load(t, body)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mount[0].Merge["settings.json"] != "hooks.claude.json" {
		t.Fatalf("merge not loaded: %+v", cfg.Mount[0])
	}
	if !cfg.HookRegistryMounted("claude") || cfg.HookRegistryMounted("codex") {
		t.Fatal("HookRegistryMounted wrong")
	}
}

func TestMergeOnlyEntryIsEnough(t *testing.T) {
	body := strings.Replace(baseYAML,
		"    links: {rules: rules, skills: skills}\n",
		"    merge: {settings.json: hooks.claude.json}\n", 1)
	if _, err := load(t, body); err != nil {
		t.Fatalf("a mount with only merge must be accepted: %v", err)
	}
}

func TestMergeTargetMustBeRegistry(t *testing.T) {
	body := strings.Replace(baseYAML,
		"    links: {rules: rules, skills: skills}\n",
		"    links: {rules: rules}\n    merge: {settings.json: skills}\n", 1)
	mustFail(t, body, "only a hook registry file can be merged")
}

func TestMergeToolMustBeListed(t *testing.T) {
	body := strings.Replace(baseYAML,
		"    links: {rules: rules, skills: skills}\n",
		"    links: {rules: rules}\n    merge: {settings.json: hooks.cursor.json}\n", 1)
	mustFail(t, body, `build.tools does not list "cursor"`)
}

func TestMergeAndLinkSameName(t *testing.T) {
	body := strings.Replace(baseYAML,
		"    links: {rules: rules, skills: skills}\n",
		"    links: {settings.json: hooks.claude.json}\n    merge: {settings.json: hooks.claude.json}\n", 1)
	mustFail(t, body, "both as a link and as a merge target")
}

func TestMergeNameNoSeparators(t *testing.T) {
	body := strings.Replace(baseYAML,
		"    links: {rules: rules, skills: skills}\n",
		"    links: {rules: rules}\n    merge: {\"a/b.json\": hooks.claude.json}\n", 1)
	mustFail(t, body, "merge name")
}

func TestMergeMergedAcrossSameDir(t *testing.T) {
	body := baseYAML + "  - dir: ./.claude\n    merge: {settings.json: hooks.claude.json}\n" +
		"  - dir: .claude\n    merge: {settings.json: hooks.codex.json}\n"
	mustFail(t, body, "merge.settings.json more than once")
}
